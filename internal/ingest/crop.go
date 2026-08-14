package ingest

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"

	_ "image/gif"  // decode the image inputs CheckInput accepts
	_ "image/jpeg" // ...
)

// layoutSize is the edge length MinerU's layout pass expects, and it is not
// negotiable: the model enters layout mode on the geometry, not on the prompt.
// Measured against a page of dense German legal prose, an aspect-preserving
// 733x1036 render answered "\nLayout Detection:" with a transcription of the
// page — the same answer it gives to "describe this image" — while the same
// page stretched to 1036x1036 answered with eighteen typed boxes. The stretch
// distorts the page, which looks wrong and is what the model was trained on.
const layoutSize = 1036

// Aspect ratio and edge limits from MinerU's own resize_by_need: a sliver of a
// block carries no recoverable signal at its natural size, and a crop below
// minCropEdge pixels does not survive the patch grid at all.
const (
	maxCropAspect = 50
	minCropEdge   = 28
)

// BBox is a block's position on the page, as fractions of the page's width and
// height. The model reports thousandths; normalizing at the parse boundary
// keeps every consumer free of the scale, and free of the layout image's
// dimensions, which are not the page's.
type BBox struct {
	X0, Y0, X1, Y1 float64
}

// Empty reports whether the box has no area, which is a box nothing can be
// cropped from.
func (b BBox) Empty() bool { return b.X1 <= b.X0 || b.Y1 <= b.Y0 }

// loadImage decodes a rasterized page or an image input.
func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path) //nolint:gosec // path is the caller's own rasterized page, or an input CheckInput accepted
	if err != nil {
		return nil, fmt.Errorf("read page image: %w", err)
	}
	defer func() { _ = f.Close() }()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode page image %s: %w", path, err)
	}
	return img, nil
}

// layoutImage produces the square, deliberately distorted image the layout pass
// wants. See layoutSize for why the distortion is the point.
func layoutImage(src image.Image) *image.RGBA {
	return resample(src, layoutSize, layoutSize)
}

// cropBlock cuts one detected block out of the page at its native resolution —
// which is the other half of why this protocol reads small print well, the
// layout pass having only ever seen the page shrunk to a thumbnail.
//
// It returns nil for a box with no pixels in it.
func cropBlock(src image.Image, b BBox) image.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	r := image.Rect(
		bounds.Min.X+int(math.Floor(b.X0*float64(w))),
		bounds.Min.Y+int(math.Floor(b.Y0*float64(h))),
		bounds.Min.X+int(math.Ceil(b.X1*float64(w))),
		bounds.Min.Y+int(math.Ceil(b.Y1*float64(h))),
	).Intersect(bounds)
	if r.Empty() {
		return nil
	}

	// SubImage shares the backing array rather than copying: a page yields
	// twenty of these, and every one of them is about to be PNG-encoded
	// anyway.
	type subImager interface {
		SubImage(image.Rectangle) image.Image
	}
	var out image.Image
	if si, ok := src.(subImager); ok {
		out = si.SubImage(r)
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
		out = dst
	}
	return fitCrop(out)
}

// fitCrop applies MinerU's two admission rules to a crop: pad a sliver out to a
// workable aspect ratio, and enlarge anything too small to survive the model's
// patch grid. Both produce a new image only when they fire, so the common case
// stays a view onto the page.
func fitCrop(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil
	}

	// Pad, not stretch: a one-line block scaled to a square would be
	// unreadable, while white space around it is what the rest of the page
	// looks like anyway.
	long, short := w, h
	if h > w {
		long, short = h, w
	}
	if short > 0 && long/short > maxCropAspect {
		want := long / maxCropAspect
		var padded *image.RGBA
		if h < w {
			padded = image.NewRGBA(image.Rect(0, 0, w, want))
		} else {
			padded = image.NewRGBA(image.Rect(0, 0, want, h))
		}
		draw.Draw(padded, padded.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		// Centred, so the block does not sit against an edge the model reads
		// as a page boundary.
		at := image.Pt((padded.Bounds().Dx()-w)/2, (padded.Bounds().Dy()-h)/2)
		draw.Draw(padded, image.Rect(at.X, at.Y, at.X+w, at.Y+h), img, b.Min, draw.Src)
		img = padded
		b = img.Bounds()
		w, h = b.Dx(), b.Dy()
	}

	if minEdge := min(w, h); minEdge < minCropEdge && minEdge > 0 {
		scale := float64(minCropEdge) / float64(minEdge)
		img = resample(img, int(math.Ceil(float64(w)*scale)), int(math.Ceil(float64(h)*scale)))
	}
	return img
}

// resample scales src to exactly w by h by averaging over the source rectangle
// each destination pixel covers.
//
// It is written out rather than pulled in because the standard library has no
// resampler and the operation here is always a reduction — a page rendered at
// 150 dpi is 1240x1754 going to 1036x1036 — where an area average is the right
// filter and a cheap one. The alternative, sampling one source pixel per
// destination pixel, drops whole rows of a 7pt footnote and the layout pass
// then reports one block where the page has three.
func resample(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	if w <= 0 || h <= 0 || sb.Empty() {
		return dst
	}

	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	for y := range h {
		y0 := sb.Min.Y + int(float64(y)*sh/float64(h))
		y1 := sb.Min.Y + int(math.Ceil(float64(y+1)*sh/float64(h)))
		y1 = min(max(y1, y0+1), sb.Max.Y)

		for x := range w {
			x0 := sb.Min.X + int(float64(x)*sw/float64(w))
			x1 := sb.Min.X + int(math.Ceil(float64(x+1)*sw/float64(w)))
			x1 = min(max(x1, x0+1), sb.Max.X)

			var sr, sg, sb64, sa uint64
			var n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					r, g, b, a := src.At(xx, yy).RGBA()
					sr += uint64(r)
					sg += uint64(g)
					sb64 += uint64(b)
					sa += uint64(a)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: avg8(sr, n),
				G: avg8(sg, n),
				B: avg8(sb64, n),
				A: avg8(sa, n),
			})
		}
	}
	return dst
}

// avg8 averages a sum of channel values and returns the top eight bits.
//
// RGBA() reports each channel as 16 bits, so sum/n cannot exceed 0xffff and the
// shift cannot exceed 0xff. The clamp states that rather than leaving it to be
// re-derived: it is one comparison per channel per destination pixel, against a
// conversion that would wrap silently if either assumption ever stopped
// holding.
func avg8(sum, n uint64) uint8 {
	v := sum / n >> 8
	if v > 0xff {
		v = 0xff
	}
	return uint8(v) //nolint:gosec // clamped to 0xff on the line above
}

// pngDataURL encodes an image for an image_url content part. Crops never reach
// the filesystem: writing twenty of them per page to a temp directory to read
// them straight back would be the only reason the backend needed one.
func pngDataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode crop: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
