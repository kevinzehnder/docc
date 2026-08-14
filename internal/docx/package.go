package docx

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// mediaFile is an image embedded in the document.
type mediaFile struct {
	// name is the file name inside word/media.
	name string
	ext  string
	data []byte
}

// relationship is one entry in a .rels part.
type relationship struct {
	id     string
	typ    string
	target string
}

const (
	relTypeStyles    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"
	relTypeNumbering = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering"
	relTypeHeader    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header"
	relTypeFooter    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer"
	relTypeImage     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
	relTypeSettings  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings"
	relTypeOfficeDoc = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relTypeCore      = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"
	relTypeApp       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties"
)

// AddImage stores an image and returns a Drawing that renders it at the given
// size. ext is the file extension without a dot: "png", "jpeg", "gif".
//
// The same bytes added twice are stored once: a logo repeated in a header and
// a footer should not double the file size.
func (d *Document) AddImage(name string, data []byte, ext string, width, height EMU) *Drawing {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if ext == "jpg" {
		ext = "jpeg"
	}

	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:8])

	fileName := "image_" + digest + "." + ext
	found := false
	for _, m := range d.media {
		if m.name == fileName {
			found = true
			break
		}
	}
	if !found {
		d.media = append(d.media, mediaFile{name: fileName, ext: ext, data: data})
	}

	return &Drawing{
		Name:    name,
		Width:   width,
		Height:  height,
		AltText: name,
		relID:   "rIdImg" + digest,
	}
}

// Bytes renders the document as a .docx archive.
//
// Output is deterministic: identical input produces byte-identical output, with
// fixed archive timestamps and a stable part order. That is what makes golden
// tests over the archive meaningful.
func (d *Document) Bytes() ([]byte, error) {
	parts, err := d.buildParts()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)

	// A fixed timestamp keeps rebuilds byte-identical. The zip epoch cannot
	// represent times before 1980.
	fixed := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, name := range names {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.Modified = fixed
		f, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", name, err)
		}
		if _, err := f.Write(parts[name]); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close archive: %w", err)
	}
	return buf.Bytes(), nil
}

// Write renders the document to a file.
func (d *Document) Write(dst string) error {
	data, err := d.Bytes()
	if err != nil {
		return err
	}
	if dir := path.Dir(dst); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	return os.WriteFile(dst, data, 0o600)
}

// WriteTo renders the document to w.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	data, err := d.Bytes()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	return int64(n), err
}

// buildParts assembles every archive member. Relationship ids are assigned here
// because a part cannot reference another until both are known.
func (d *Document) buildParts() (map[string][]byte, error) {
	parts := map[string][]byte{}

	var docRels []relationship
	docRels = append(docRels, relationship{"rId1", relTypeStyles, "styles.xml"})
	docRels = append(docRels, relationship{"rId2", relTypeSettings, "settings.xml"})

	next := 3
	if !d.Numbering.empty() {
		docRels = append(docRels, relationship{fmt.Sprintf("rId%d", next), relTypeNumbering, "numbering.xml"})
		next++
	}

	// Headers and footers must be named and related before document.xml is
	// rendered, since sectPr references them by id.
	for i := range d.Headers {
		id := fmt.Sprintf("rId%d", next)
		next++
		name := fmt.Sprintf("header%d.xml", i+1)
		d.Headers[i].relID = id
		d.Headers[i].partName = name
		docRels = append(docRels, relationship{id, relTypeHeader, name})
	}
	for i := range d.Footers {
		id := fmt.Sprintf("rId%d", next)
		next++
		name := fmt.Sprintf("footer%d.xml", i+1)
		d.Footers[i].relID = id
		d.Footers[i].partName = name
		docRels = append(docRels, relationship{id, relTypeFooter, name})
	}

	for _, m := range d.media {
		digest := strings.TrimSuffix(strings.TrimPrefix(m.name, "image_"), "."+m.ext)
		docRels = append(docRels, relationship{"rIdImg" + digest, relTypeImage, "media/" + m.name})
		parts["word/media/"+m.name] = m.data
	}

	d.assignDrawingIDs()

	parts["[Content_Types].xml"] = d.writeContentTypes()
	parts["_rels/.rels"] = writeRootRels()
	parts["word/document.xml"] = d.writeDocument()
	parts["word/_rels/document.xml.rels"] = writeRels(docRels)
	parts["word/styles.xml"] = d.writeStyles()
	parts["word/settings.xml"] = d.writeSettings()
	parts["docProps/core.xml"] = d.writeCoreProps()
	parts["docProps/app.xml"] = writeAppProps()

	if !d.Numbering.empty() {
		parts["word/numbering.xml"] = d.writeNumbering()
	}
	for _, h := range d.Headers {
		parts["word/"+h.partName] = d.writeHeaderFooter(h, "w:hdr")
	}
	for _, f := range d.Footers {
		parts["word/"+f.partName] = d.writeHeaderFooter(f, "w:ftr")
	}

	return parts, nil
}

// assignDrawingIDs numbers every drawing in the document. Word requires the ids
// to be unique and non-zero across the whole file, so they are assigned in one
// pass over every block rather than per part.
func (d *Document) assignDrawingIDs() {
	id := 1
	visit := func(blocks []Block) {
		var walk func([]Block)
		walk = func(bs []Block) {
			for _, b := range bs {
				switch v := b.(type) {
				case Paragraph:
					for _, r := range v.Runs {
						for _, item := range r.Items {
							if dr, ok := item.(*Drawing); ok {
								dr.docPr = id
								id++
							}
						}
					}
				case Table:
					for _, row := range v.Rows {
						for _, c := range row.Cells {
							walk(c.Blocks)
						}
					}
				}
			}
		}
		walk(blocks)
	}

	visit(d.Body)
	for _, h := range d.Headers {
		visit(h.Blocks)
	}
	for _, f := range d.Footers {
		visit(f.Blocks)
	}
}

func (d *Document) writeContentTypes() []byte {
	w := &xw{}
	w.header()
	w.open("Types", a("xmlns", "http://schemas.openxmlformats.org/package/2006/content-types"))

	w.empty("Default", a("Extension", "rels"),
		a("ContentType", "application/vnd.openxmlformats-package.relationships+xml"))
	w.empty("Default", a("Extension", "xml"), a("ContentType", "application/xml"))

	// One Default per distinct image extension, sorted so output stays stable.
	seen := map[string]bool{}
	var exts []string
	for _, m := range d.media {
		if !seen[m.ext] {
			seen[m.ext] = true
			exts = append(exts, m.ext)
		}
	}
	sort.Strings(exts)
	for _, ext := range exts {
		w.empty("Default", a("Extension", ext), a("ContentType", imageContentType(ext)))
	}

	override := func(part, typ string) {
		w.empty("Override", a("PartName", part), a("ContentType", typ))
	}
	const wordml = "application/vnd.openxmlformats-officedocument.wordprocessingml"
	override("/word/document.xml", wordml+".document.main+xml")
	override("/word/styles.xml", wordml+".styles+xml")
	override("/word/settings.xml", wordml+".settings+xml")
	if !d.Numbering.empty() {
		override("/word/numbering.xml", wordml+".numbering+xml")
	}
	for _, h := range d.Headers {
		override("/word/"+h.partName, wordml+".header+xml")
	}
	for _, f := range d.Footers {
		override("/word/"+f.partName, wordml+".footer+xml")
	}
	override("/docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml")
	override("/docProps/app.xml", "application/vnd.openxmlformats-officedocument.extended-properties+xml")

	w.close("Types")
	return w.bytes()
}

func imageContentType(ext string) string {
	switch ext {
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "tiff", "tif":
		return "image/tiff"
	case "svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

func writeRootRels() []byte {
	return writeRelsWithNS([]relationship{
		{"rId1", relTypeOfficeDoc, "word/document.xml"},
		{"rId2", relTypeCore, "docProps/core.xml"},
		{"rId3", relTypeApp, "docProps/app.xml"},
	})
}

func writeRels(rels []relationship) []byte {
	return writeRelsWithNS(rels)
}

func writeRelsWithNS(rels []relationship) []byte {
	w := &xw{}
	w.header()
	w.open("Relationships", a("xmlns", "http://schemas.openxmlformats.org/package/2006/relationships"))
	for _, r := range rels {
		w.empty("Relationship", a("Id", r.id), a("Type", r.typ), a("Target", r.target))
	}
	w.close("Relationships")
	return w.bytes()
}

func (d *Document) writeSettings() []byte {
	w := &xw{}
	w.header()
	w.open("w:settings", nsAttrs()...)
	w.empty("w:defaultTabStop", ai("w:val", Twips(708)))
	// Only meaningful when an even-page header exists, but harmless otherwise.
	for _, h := range d.Headers {
		if h.hfType() == HFEven {
			w.empty("w:evenAndOddHeaders")
			break
		}
	}
	w.open("w:compat")
	// Declaring the format level keeps Word from applying legacy layout
	// quirks that would move text relative to what LibreOffice renders.
	w.empty("w:compatSetting",
		a("w:name", "compatibilityMode"),
		a("w:uri", "http://schemas.microsoft.com/office/word"),
		a("w:val", "15"),
	)
	w.close("w:compat")
	w.close("w:settings")
	return w.bytes()
}

func (d *Document) writeCoreProps() []byte {
	w := &xw{}
	w.header()
	w.open("cp:coreProperties",
		a("xmlns:cp", "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"),
		a("xmlns:dc", "http://purl.org/dc/elements/1.1/"),
		a("xmlns:dcterms", "http://purl.org/dc/terms/"),
		a("xmlns:dcmitype", "http://purl.org/dc/dcmitype/"),
		a("xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance"),
	)
	p := d.Properties
	elem := func(name, value string) {
		if value == "" {
			return
		}
		w.open(name)
		w.text(value)
		w.close(name)
	}
	elem("dc:title", p.Title)
	elem("dc:subject", p.Subject)
	elem("dc:creator", p.Creator)
	elem("dc:description", p.Description)
	elem("cp:keywords", p.Keywords)

	// A fixed default keeps output deterministic; a caller wanting real
	// timestamps sets them explicitly.
	created := p.Created
	if created == "" {
		created = "1980-01-01T00:00:00Z"
	}
	modified := p.Modified
	if modified == "" {
		modified = created
	}
	w.open("dcterms:created", a("xsi:type", "dcterms:W3CDTF"))
	w.text(created)
	w.close("dcterms:created")
	w.open("dcterms:modified", a("xsi:type", "dcterms:W3CDTF"))
	w.text(modified)
	w.close("dcterms:modified")

	w.close("cp:coreProperties")
	return w.bytes()
}

func writeAppProps() []byte {
	w := &xw{}
	w.header()
	w.open("Properties",
		a("xmlns", "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"),
		a("xmlns:vt", "http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"),
	)
	w.open("Application")
	w.text("docc")
	w.close("Application")
	w.close("Properties")
	return w.bytes()
}
