package ingest

import (
	"image"
	"image/color"
	"testing"
)

// fill builds a test page: white, with one black rectangle to locate.
func fill(w, h int, mark image.Rectangle) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{R: 255, G: 255, B: 255, A: 255}
			if image.Pt(x, y).In(mark) {
				c = color.RGBA{A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// The layout pass enters layout mode on the geometry, so the square is not
// negotiable and the distortion is deliberate. Measured: the same page at
// 733x1036 answers the layout prompt with a transcription instead.
func TestLayoutImageIsExactlySquare(t *testing.T) {
	got := layoutImage(fill(1240, 1754, image.Rect(0, 0, 10, 10))).Bounds()
	if got.Dx() != layoutSize || got.Dy() != layoutSize {
		t.Errorf("layout image is %dx%d, want %dx%d", got.Dx(), got.Dy(), layoutSize, layoutSize)
	}
}

func TestCropBlockCutsTheRightRegion(t *testing.T) {
	// A black band across the middle fifth of a 1000x1000 page.
	src := fill(1000, 1000, image.Rect(0, 400, 1000, 600))

	crop := cropBlock(src, BBox{X0: 0, Y0: 0.4, X1: 1, Y1: 0.6})
	if crop == nil {
		t.Fatal("cropBlock returned nil")
	}
	b := crop.Bounds()
	if b.Dx() != 1000 || b.Dy() != 200 {
		t.Fatalf("crop is %dx%d, want 1000x200", b.Dx(), b.Dy())
	}
	// Every pixel of it should be the band, not the white around it.
	r, _, _, _ := crop.At(b.Min.X+500, b.Min.Y+100).RGBA()
	if r != 0 {
		t.Errorf("crop centre is not the marked band (r=%d)", r)
	}
}

func TestCropBlockRejectsAnEmptyBox(t *testing.T) {
	src := fill(100, 100, image.Rect(0, 0, 1, 1))
	if got := cropBlock(src, BBox{X0: 0.5, Y0: 0.5, X1: 0.5, Y1: 0.5}); got != nil {
		t.Errorf("cropBlock of an empty box = %v, want nil", got)
	}
}

// A crop below the model's minimum edge is enlarged rather than sent as is:
// MinerU's own resize_by_need does the same, because a block that small does
// not survive the patch grid.
func TestCropBlockEnlargesASliver(t *testing.T) {
	src := fill(1000, 1000, image.Rect(0, 0, 1000, 1000))
	crop := cropBlock(src, BBox{X0: 0.1, Y0: 0.5, X1: 0.9, Y1: 0.51})
	if crop == nil {
		t.Fatal("cropBlock returned nil")
	}
	if got := crop.Bounds().Dy(); got < minCropEdge {
		t.Errorf("crop height %d, want at least %d", got, minCropEdge)
	}
}

// Downscaling by averaging, not by sampling: a black line one pixel high has to
// survive a 4x reduction as grey rather than vanish because the sampled row
// happened to be white.
func TestResampleAveragesRatherThanSamples(t *testing.T) {
	src := fill(400, 400, image.Rect(0, 200, 400, 201))

	got := resample(src, 100, 100)
	darkest := uint32(0xffff)
	for y := range 100 {
		r, _, _, _ := got.At(50, y).RGBA()
		if r < darkest {
			darkest = r
		}
	}
	if darkest == 0xffff {
		t.Error("the line vanished entirely — resample is sampling, not averaging")
	}
	if darkest == 0 {
		t.Error("the line stayed fully black — a 1-in-4 row should average to grey")
	}
}

func TestResampleHandlesDegenerateSizes(t *testing.T) {
	src := fill(10, 10, image.Rect(0, 0, 5, 5))
	if got := resample(src, 0, 10).Bounds(); !got.Empty() {
		t.Errorf("resample to zero width = %v, want empty", got)
	}
}

func TestPNGDataURLIsADataURL(t *testing.T) {
	url, err := pngDataURL(fill(4, 4, image.Rect(0, 0, 2, 2)))
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "data:image/png;base64,"
	if len(url) <= len(prefix) || url[:len(prefix)] != prefix {
		t.Errorf("data URL = %.40q, want the %q prefix", url, prefix)
	}
}
