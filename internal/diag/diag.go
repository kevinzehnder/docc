// Package diag defines compiler diagnostics and their rendering.
//
// Diagnostics are the product: every check docc performs reports through this
// type, and both humans and agents consume the result. A diagnostic without a
// source position and an actionable hint is a bug.
package diag

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Severity classifies a diagnostic. Only Error fails a check.
type Severity int

const (
	Warning Severity = iota
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warning"
}

// Position is a 1-indexed source location. Line 0 means the diagnostic applies
// to the file as a whole and no caret is rendered.
type Position struct {
	Line int `json:"line"`
	Col  int `json:"col"`
	// Len is the width of the offending span in bytes, used for the caret
	// underline. Zero renders a single caret.
	Len int `json:"len,omitempty"`
}

// Location is a position in a named file, used for the other occurrences a
// diagnostic involves.
type Location struct {
	File string   `json:"file"`
	Pos  Position `json:"pos"`
}

// Diagnostic is one finding against one source file.
type Diagnostic struct {
	File     string   `json:"file"`
	Pos      Position `json:"pos"`
	Severity Severity `json:"severity"`
	// Code is a stable identifier such as "DOC004". Never renumber a released
	// code: agents and docs reference them.
	Code    string `json:"code"`
	Message string `json:"message"`
	// Hint tells the author what to do. Omit only when genuinely self-evident.
	Hint string `json:"hint,omitempty"`

	// Block names the semantic block (its kind, or its `#id` when one exists)
	// the finding concerns.
	Block string `json:"block,omitempty"`
	// Key is the span consistency key the finding concerns.
	Key string `json:"key,omitempty"`
	// Expected is one concise valid-syntax example that would satisfy the
	// check, for an agent to imitate directly.
	Expected string `json:"expected,omitempty"`
	// Related lists the other source locations involved in the same finding —
	// every conflicting occurrence of an inconsistent value, the first use of
	// a duplicated id.
	Related []Location `json:"related,omitempty"`
}

func (d Diagnostic) MarshalJSON() ([]byte, error) {
	type alias Diagnostic
	return json.Marshal(struct {
		alias
		Severity string `json:"severity"`
	}{alias(d), d.Severity.String()})
}

// List is an ordered set of diagnostics for one compilation.
type List []Diagnostic

// Add appends a fully constructed diagnostic, for callers that set the
// structured fields Errorf cannot express.
func (l *List) Add(d Diagnostic) {
	*l = append(*l, d)
}

// Errorf appends an error-severity diagnostic.
func (l *List) Errorf(file string, pos Position, code, hint, format string, args ...any) {
	*l = append(*l, Diagnostic{
		File: file, Pos: pos, Severity: Error, Code: code,
		Message: fmt.Sprintf(format, args...), Hint: hint,
	})
}

// Warnf appends a warning-severity diagnostic.
func (l *List) Warnf(file string, pos Position, code, hint, format string, args ...any) {
	*l = append(*l, Diagnostic{
		File: file, Pos: pos, Severity: Warning, Code: code,
		Message: fmt.Sprintf(format, args...), Hint: hint,
	})
}

// HasErrors reports whether any diagnostic is error severity.
func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Counts returns the number of errors and warnings.
func (l List) Counts() (errs, warns int) {
	for _, d := range l {
		if d.Severity == Error {
			errs++
		} else {
			warns++
		}
	}
	return
}

// Sort orders diagnostics by file, then line, then column. File-level
// diagnostics (line 0) sort last within their file so positioned findings read
// first.
func (l List) Sort() {
	sort.SliceStable(l, func(i, j int) bool {
		a, b := l[i], l[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if (a.Pos.Line == 0) != (b.Pos.Line == 0) {
			return b.Pos.Line == 0
		}
		if a.Pos.Line != b.Pos.Line {
			return a.Pos.Line < b.Pos.Line
		}
		return a.Pos.Col < b.Pos.Col
	})
}

// SourceFn returns the raw text of a file so the renderer can quote the
// offending line. Returning "" suppresses the source excerpt.
type SourceFn func(file string) string

// Render writes human-readable diagnostics with source excerpts and carets.
func (l List) Render(w io.Writer, src SourceFn, color bool) error {
	l.Sort()
	for _, d := range l {
		if err := renderOne(w, d, src, color); err != nil {
			return err
		}
	}
	errs, warns := l.Counts()
	if errs == 0 && warns == 0 {
		_, err := fmt.Fprintln(w, tint(color, green, "✅ ok"))
		return err
	}
	_, err := fmt.Fprintf(w, "\n%s, %s\n", plural(errs, "error"), plural(warns, "warning"))
	return err
}

// RenderJSON writes diagnostics as a JSON object for programmatic consumers.
func (l List) RenderJSON(w io.Writer) error {
	l.Sort()
	errs, warns := l.Counts()
	if l == nil {
		l = List{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		OK          bool `json:"ok"`
		Errors      int  `json:"errors"`
		Warnings    int  `json:"warnings"`
		Diagnostics List `json:"diagnostics"`
	}{OK: errs == 0, Errors: errs, Warnings: warns, Diagnostics: l})
}

const (
	red    = "\x1b[31m"
	yellow = "\x1b[33m"
	green  = "\x1b[32m"
	blue   = "\x1b[34m"
	dim    = "\x1b[2m"
	reset  = "\x1b[0m"
)

func tint(on bool, c, s string) string {
	if !on {
		return s
	}
	return c + s + reset
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func renderOne(w io.Writer, d Diagnostic, src SourceFn, color bool) error {
	sevColor := yellow
	if d.Severity == Error {
		sevColor = red
	}
	loc := d.File
	if d.Pos.Line > 0 {
		loc = fmt.Sprintf("%s:%d:%d", d.File, d.Pos.Line, d.Pos.Col)
	}
	head := fmt.Sprintf("%s: %s: %s\n",
		loc,
		tint(color, sevColor, fmt.Sprintf("%s[%s]", d.Severity, d.Code)),
		d.Message,
	)
	if _, err := io.WriteString(w, head); err != nil {
		return err
	}

	if d.Pos.Line == 0 || src == nil {
		return writeBareHint(w, d, color)
	}
	line, ok := nthLine(src(d.File), d.Pos.Line)
	if !ok {
		return writeBareHint(w, d, color)
	}

	gutter := fmt.Sprintf("%d", d.Pos.Line)
	pad := strings.Repeat(" ", len(gutter))
	if _, err := fmt.Fprintf(w, "%s %s\n", tint(color, blue, pad+" |"), ""); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s %s\n", tint(color, blue, gutter+" |"), line); err != nil {
		return err
	}

	caret := caretLine(line, d.Pos)
	trailer := ""
	if d.Hint != "" {
		trailer = " " + d.Hint
	}
	if _, err := fmt.Fprintf(w, "%s %s\n",
		tint(color, blue, pad+" |"),
		tint(color, sevColor, caret+trailer),
	); err != nil {
		return err
	}
	return writeExtras(w, d, color)
}

func writeBareHint(w io.Writer, d Diagnostic, color bool) error {
	if d.Hint != "" {
		if _, err := fmt.Fprintf(w, "  %s %s\n", tint(color, blue, "="), d.Hint); err != nil {
			return err
		}
	}
	return writeExtras(w, d, color)
}

// writeExtras renders the structured fields that exist mainly for agents but
// help humans too: the syntax to imitate and the other locations involved.
func writeExtras(w io.Writer, d Diagnostic, color bool) error {
	if d.Expected != "" {
		if _, err := fmt.Fprintf(w, "  %s expected: %s\n", tint(color, blue, "="), d.Expected); err != nil {
			return err
		}
	}
	for _, r := range d.Related {
		if _, err := fmt.Fprintf(w, "  %s related: %s:%d:%d\n",
			tint(color, blue, "="), r.File, r.Pos.Line, r.Pos.Col); err != nil {
			return err
		}
	}
	return nil
}

// caretLine builds the "   ^^^^" underline beneath the quoted source line.
//
// Position.Col and Position.Len are byte offsets, because that is what the
// parsers report. The terminal aligns by character, so both are converted to
// rune counts here — otherwise every caret under a line containing an umlaut
// drifts right by one column per non-ASCII byte.
func caretLine(line string, pos Position) string {
	col := pos.Col
	if col < 1 {
		col = 1
	}
	startByte := min(col-1, len(line))
	endByte := len(line)
	if pos.Len > 0 {
		endByte = min(startByte+pos.Len, len(line))
	}

	var b strings.Builder
	for _, r := range line[:startByte] {
		if r == '\t' {
			b.WriteRune('\t')
		} else {
			b.WriteRune(' ')
		}
	}

	n := len([]rune(line[startByte:endByte]))
	if n < 1 {
		n = 1
	}
	b.WriteString(strings.Repeat("^", n))
	return b.String()
}

func nthLine(text string, n int) (string, bool) {
	if text == "" || n < 1 {
		return "", false
	}
	lines := strings.Split(text, "\n")
	if n > len(lines) {
		return "", false
	}
	return strings.TrimRight(lines[n-1], "\r"), true
}
