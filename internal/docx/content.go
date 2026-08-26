package docx

// This file is deliberately not a DOCX importer. It reads only the visible
// text needed to compare a document produced by docc with an edited copy.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

const maxContentPart = 32 << 20

// Content is the canonical textual content of a DOCX, split into Word stories.
type Content struct {
	Stories []Story `json:"stories"`
}

// Story is the main document, a header, or a footer.
type Story struct {
	Name    string   `json:"name"`
	Records []string `json:"records"`
}

// ReadContent reads canonical visible text from a DOCX file.
func ReadContent(name string) (Content, error) {
	f, err := os.Open(name) //nolint:gosec // the caller supplied the document
	if err != nil {
		return Content{}, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return Content{}, err
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return Content{}, fmt.Errorf("open DOCX: %w", err)
	}
	return readContent(zr)
}

// ReadContentBytes is ReadContent for an in-memory build.
func ReadContentBytes(data []byte) (Content, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Content{}, fmt.Errorf("open DOCX: %w", err)
	}
	return readContent(zr)
}

func readContent(zr *zip.Reader) (Content, error) {
	parts := map[string]*zip.File{}
	var names []string
	for _, f := range zr.File {
		name := path.Clean(f.Name)
		if name != f.Name || strings.HasPrefix(name, "../") {
			continue
		}
		base := path.Base(name)
		if name == "word/document.xml" ||
			(strings.HasPrefix(base, "header") || strings.HasPrefix(base, "footer")) &&
				strings.HasSuffix(base, ".xml") && path.Dir(name) == "word" {
			parts[name] = f
			names = append(names, name)
		}
	}
	if parts["word/document.xml"] == nil {
		return Content{}, fmt.Errorf("DOCX has no word/document.xml")
	}
	sort.Strings(names)
	// The body is the useful first story even though lexical order puts it
	// before headers only by accident today.
	for i, name := range names {
		if name == "word/document.xml" {
			names[0], names[i] = names[i], names[0]
			break
		}
	}

	var body []string
	groups := map[string]map[string][]string{"headers": {}, "footers": {}}
	for _, name := range names {
		f := parts[name]
		if f.UncompressedSize64 > maxContentPart {
			return Content{}, fmt.Errorf("%s is too large to compare", name)
		}
		r, err := f.Open()
		if err != nil {
			return Content{}, fmt.Errorf("open %s: %w", name, err)
		}
		records, parseErr := extractText(io.LimitReader(r, maxContentPart+1))
		closeErr := r.Close()
		if parseErr != nil {
			return Content{}, fmt.Errorf("read %s: %w", name, parseErr)
		}
		if closeErr != nil {
			return Content{}, fmt.Errorf("close %s: %w", name, closeErr)
		}
		switch {
		case name == "word/document.xml":
			body = records
		case strings.HasPrefix(path.Base(name), "header"):
			groups["headers"][strings.Join(records, "\x00")] = records
		default:
			groups["footers"][strings.Join(records, "\x00")] = records
		}
	}

	out := Content{Stories: []Story{{Name: "body", Records: body}}}
	// Applications freely rename and duplicate header/footer parts on save.
	// Compare their distinct textual contents, not unstable package filenames.
	for _, group := range []string{"headers", "footers"} {
		delete(groups[group], "")
		keys := make([]string, 0, len(groups[group]))
		for key := range groups[group] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var records []string
		for _, key := range keys {
			records = append(records, groups[group][key]...)
		}
		if len(records) > 0 {
			out.Stories = append(out.Stories, Story{Name: group, Records: records})
		}
	}
	return out, nil
}

type fieldText struct {
	instruction strings.Builder
	written     bool
}

type paragraphText struct {
	text   strings.Builder
	fields []fieldText
}

// extractText intentionally ignores formatting and revision metadata. It reads
// tracked changes as their final view: insertions remain visible; deletions and
// move sources do not.
func extractText(r io.Reader) ([]string, error) {
	dec := xml.NewDecoder(r)
	var (
		paragraphs []*paragraphText
		records    []string
		hidden     int
		simple     int
	)
	current := func() *paragraphText {
		if len(paragraphs) == 0 {
			return nil
		}
		return paragraphs[len(paragraphs)-1]
	}
	appendText := func(s string) {
		if p := current(); p != nil && hidden == 0 && simple == 0 && len(p.fields) == 0 {
			p.text.WriteString(s)
		}
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "del", "moveFrom", "Fallback":
				hidden++
			case "p":
				paragraphs = append(paragraphs, &paragraphText{})
			case "fldSimple":
				if hidden == 0 {
					for _, a := range t.Attr {
						if a.Name.Local == "instr" {
							appendText(fieldMarker(a.Value))
							break
						}
					}
				}
				simple++
			case "fldChar":
				if p := current(); p != nil && hidden == 0 && simple == 0 {
					kind := attrValue(t.Attr, "fldCharType")
					switch kind {
					case "begin":
						p.fields = append(p.fields, fieldText{})
					case "separate":
						if len(p.fields) > 0 && !p.fields[len(p.fields)-1].written {
							p.text.WriteString(fieldMarker(p.fields[len(p.fields)-1].instruction.String()))
							p.fields[len(p.fields)-1].written = true
						}
					case "end":
						if len(p.fields) > 0 {
							f := p.fields[len(p.fields)-1]
							if !f.written {
								p.text.WriteString(fieldMarker(f.instruction.String()))
							}
							p.fields = p.fields[:len(p.fields)-1]
						}
					}
				}
			case "instrText":
				if p := current(); p != nil && hidden == 0 && simple == 0 && len(p.fields) > 0 {
					var s string
					if err := dec.DecodeElement(&s, &t); err != nil {
						return nil, err
					}
					p.fields[len(p.fields)-1].instruction.WriteString(s)
				}
			case "t":
				if hidden == 0 && simple == 0 {
					var s string
					if err := dec.DecodeElement(&s, &t); err != nil {
						return nil, err
					}
					appendText(s)
				}
			case "tab":
				appendText("\t")
			case "br", "cr":
				appendText("\n")
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "del", "moveFrom", "Fallback":
				if hidden > 0 {
					hidden--
				}
			case "fldSimple":
				if simple > 0 {
					simple--
				}
			case "p":
				if len(paragraphs) == 0 {
					continue
				}
				p := paragraphs[len(paragraphs)-1]
				paragraphs = paragraphs[:len(paragraphs)-1]
				// Word and LibreOffice may materialize a paragraph's final
				// layout break differently. Internal hard breaks remain content;
				// trailing ones do not change the paragraph's words.
				if text := strings.TrimRight(p.text.String(), "\n"); text != "" {
					records = append(records, text)
				}
			}
		}
	}
	if len(paragraphs) != 0 {
		return nil, fmt.Errorf("unclosed paragraph")
	}
	return records, nil
}

func attrValue(attrs []xml.Attr, local string) string {
	for _, a := range attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func fieldMarker(instruction string) string {
	instruction = strings.Join(strings.Fields(instruction), " ")
	if instruction == "" {
		return ""
	}
	return "{" + instruction + "}"
}
