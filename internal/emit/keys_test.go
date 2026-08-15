package emit

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/schema"
)

func memoSchema() *schema.Schema {
	return &schema.Schema{
		Type:   "memo",
		Blocks: map[string]schema.BlockSpec{"beweis": {}},
		Spans:  map[string]schema.SpanSpec{"uid": {}},
		Styles: map[string]string{},
	}
}

// The whole point of StyleKeys is that it matches what the emitter reads. This
// scans the emitter's own source for the block-style suffixes it looks up, so a
// new lookup that nobody adds to blockSuffixes fails here rather than making
// `docc doctor` call a working mapping unread — which is exactly what happened
// with `.words`.
func TestBlockSuffixesMatchTheEmitter(t *testing.T) {
	src, err := os.ReadFile("emit.go")
	if err != nil {
		t.Fatal(err)
	}

	// Matches `d.Name + ".amount"` in any spacing.
	re := regexp.MustCompile(`d\.Name\s*\+\s*"(\.[^"]+)"`)
	found := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		found[m[1]] = true
	}
	if len(found) == 0 {
		t.Fatal("no block style suffixes found in emit.go — has the lookup changed shape?")
	}

	declared := map[string]bool{}
	for _, s := range blockSuffixes {
		declared[s.suffix] = true
	}
	for suffix := range found {
		if !declared[suffix] {
			t.Errorf("emit.go looks up div.<name>%s, but blockSuffixes does not declare it", suffix)
		}
	}
	for suffix := range declared {
		if !found[suffix] {
			t.Errorf("blockSuffixes declares %s, which emit.go never looks up", suffix)
		}
	}
}

// The same guard for the constructs that are not block-scoped.
func TestFixedKeysMatchTheEmitter(t *testing.T) {
	src, err := os.ReadFile("emit.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`e\.style\("([a-z_]+)"`)
	known := map[string]bool{}
	for _, k := range StyleKeys(memoSchema()) {
		known[k.Key] = true
	}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if !known[m[1]] {
			t.Errorf("emit.go reads style key %q, which StyleKeys does not report", m[1])
		}
	}
}

func TestStyleKeysCoverBlocksAndSpans(t *testing.T) {
	var keys []string
	for _, k := range StyleKeys(memoSchema()) {
		keys = append(keys, k.Key)
	}
	for _, want := range []string{
		"paragraph", "heading", "h1", "h6", "quote", "code", "table",
		"ordered_list", "bullet_list",
		"div.beweis", "div.beweis.label", "div.beweis.amount", "div.beweis.total",
		"div.beweis.total.amount", "div.beweis.line", "div.beweis.words",
		"span.uid",
	} {
		if !slices.Contains(keys, want) {
			t.Errorf("StyleKeys is missing %q", want)
		}
	}
}

func TestUnreadStyleKeys(t *testing.T) {
	t.Run("fixed-formatting keys are named with the reason", func(t *testing.T) {
		sc := memoSchema()
		sc.Styles["code_span"] = "Mono"
		got := UnreadStyleKeys(sc)
		if len(got) != 1 || !strings.Contains(got[0], "Courier New") {
			t.Errorf("got %v, want one finding naming the fixed formatting", got)
		}
	})

	t.Run("an undeclared block is distinguished from a bad suffix", func(t *testing.T) {
		sc := memoSchema()
		sc.Styles["div.bewies"] = "X"
		sc.Styles["div.beweis.wrong"] = "Y"
		got := strings.Join(UnreadStyleKeys(sc), "\n")
		if n := len(UnreadStyleKeys(sc)); n != 2 {
			t.Fatalf("got %d findings, want 2:\n%s", n, got)
		}
		if !strings.Contains(got, `div.bewies — the schema declares no block "bewies"`) {
			t.Errorf("the unknown block is not named as such:\n%s", got)
		}
		if !strings.Contains(got, `div.beweis.wrong — "beweis" is a declared block`) {
			t.Errorf("the bad suffix is not distinguished from an unknown block:\n%s", got)
		}
	})

	t.Run("div keys are not judged until the schema declares blocks", func(t *testing.T) {
		sc := memoSchema()
		sc.Blocks = nil
		sc.Styles["div.anything"] = "X"
		if got := UnreadStyleKeys(sc); len(got) != 0 {
			t.Errorf("got %v, want nothing — an undeclared block contract permits any name", got)
		}
	})

	t.Run("span keys are left to validateSpanStyles", func(t *testing.T) {
		sc := memoSchema()
		sc.Styles["span.nosuch"] = "X"
		sc.Styles["span.docc-field"] = "Y"
		if got := UnreadStyleKeys(sc); len(got) != 0 {
			t.Errorf("got %v, want nothing — spans are validated elsewhere, as errors", got)
		}
	})

	t.Run("a correct map is silent", func(t *testing.T) {
		sc := memoSchema()
		sc.Styles["h1"] = "Heading1"
		sc.Styles["div.beweis.label"] = "Label"
		sc.Styles["span.uid"] = "UID"
		if got := UnreadStyleKeys(sc); len(got) != 0 {
			t.Errorf("got %v, want nothing", got)
		}
	})
}

// Mapping a suffix selects a rendering pattern the block's own declaration says
// nothing about, which is why describe reports it.
func TestBlockPattern(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"", "plain"},
		{"div.beweis.label", "labelled"},
		{"div.beweis.line", "ruled"},
		{"div.beweis.amount", "amount"},
	}
	for _, tc := range cases {
		sc := memoSchema()
		if tc.key != "" {
			sc.Styles[tc.key] = "S"
		}
		if got := BlockPattern(sc, "beweis"); got != tc.want {
			t.Errorf("BlockPattern with %q = %q, want %q", tc.key, got, tc.want)
		}
	}

	// Two mapped: the emitter takes the first of its dispatch, and so does this.
	sc := memoSchema()
	sc.Styles["div.beweis.label"] = "S"
	sc.Styles["div.beweis.amount"] = "S"
	if got := BlockPattern(sc, "beweis"); got != "amount" {
		t.Errorf("BlockPattern with both = %q, want amount (the emitter's dispatch order)", got)
	}
}
