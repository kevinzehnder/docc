package docx

import (
	"strings"
	"testing"
)

func TestReadContentCanonicalizesVisibleText(t *testing.T) {
	xml := `<?xml version="1.0"?><w:document xmlns:w="urn:w"><w:body>
<w:p><w:r><w:t>Hello </w:t></w:r><w:ins><w:r><w:t>new</w:t></w:r></w:ins><w:del><w:r><w:delText>old</w:delText></w:r></w:del><w:r><w:tab/><w:t>world</w:t><w:br/><w:t>again</w:t></w:r></w:p>
<w:p><w:fldSimple w:instr=" PAGE "><w:r><w:t>7</w:t></w:r></w:fldSimple></w:p>
<w:p><w:r><w:fldChar w:fldCharType="begin"/><w:instrText> NUMPAGES </w:instrText><w:fldChar w:fldCharType="separate"/><w:t>9</w:t><w:fldChar w:fldCharType="end"/></w:r></w:p>
</w:body></w:document>`
	got, err := extractText(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Hello new\tworld\nagain", "{PAGE}", "{NUMPAGES}"}
	if len(got) != len(want) {
		t.Fatalf("records = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadContentBytesReadsBuiltStories(t *testing.T) {
	d := &Document{
		Body:    []Block{P("", "Body")},
		Headers: []HeaderFooter{{Blocks: []Block{P("", "Header")}}},
		Footers: []HeaderFooter{{Blocks: []Block{P("", "Footer")}}},
	}
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadContentBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Stories) != 3 || got.Stories[0].Name != "body" || got.Stories[0].Records[0] != "Body" {
		t.Fatalf("content = %#v", got)
	}
}
