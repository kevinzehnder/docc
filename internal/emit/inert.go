package emit

// Configuration a theme can set that provably cannot do anything.
//
// This is the theme-side counterpart to UnreadStyleKeys: a mapping the emitter
// never reads, a flag no line can act on. Both pass every loader, render
// without complaint, and change nothing — which is the worst kind of
// configuration, because the author concludes the compiler is at fault and
// goes looking somewhere else.
//
// It was a real defect before it was a check. A letter's enclosures heading
// carried `omit_if_empty: false` under a comment claiming that suppressed it
// for a letter with no enclosures; the flag was unreachable, the heading
// printed over an empty list, and the pattern had been copied onto 66 further
// lines. `docc doctor --strict` said "no problems found".

import (
	"fmt"
	"strings"

	"github.com/kevinzehnder/docc/internal/theme"
)

// InertFurnitureFlags reports `omit_if_empty` settings that cannot affect the
// output, with the region and line they sit on.
//
// The drop only fires when every placeholder a line filled in came out empty
// (theme.Interp.AllEmpty, which requires at least one placeholder), so a line
// of fixed text can never be dropped by it whatever the flag says — not even
// `true`. The knob for that case is `if_nonempty:`, which asks about a named
// field instead of about placeholders.
//
// Findings are warnings, not errors: a theme carrying one renders correct
// documents, and refusing to build a filed letter over a keyword that does
// nothing would be the worse trade. `--strict` binds them.
func InertFurnitureFlags(th *theme.Theme) []string {
	var out []string
	report := func(region string, i int, line theme.Line, why, fix string) {
		style := line.Style
		if style == "" {
			style = "no style"
		}
		out = append(out, fmt.Sprintf("%s line %d (%s): %s — %s", region, i+1, style, why, fix))
	}

	inspect := func(region string, lines []theme.Line) {
		for i, line := range lines {
			if line.OmitIfEmpty != nil {
				if why := inertLineReason(line); why != "" {
					report(region, i, line, why,
						"remove it, or use `if_nonempty: <field>` to drop the line when a list or field is empty")
				}
			}
			for _, r := range line.Runs {
				if r.OmitIfEmpty == nil || strings.Contains(r.Text, "{{") {
					continue
				}
				report(region, i, line,
					fmt.Sprintf("`omit_if_empty` on the run %q has no placeholder to be empty", r.Text),
					"remove it; a literal run is dropped only with the line around it")
			}
		}
	}

	inspect("prologue", th.Prologue)
	inspect("epilogue", th.Epilogue)
	for _, key := range sortedKeys(th.Header) {
		inspect("header:"+key, th.Header[key])
	}
	for _, key := range sortedKeys(th.Footer) {
		inspect("footer:"+key, th.Footer[key])
	}
	return out
}

// inertLineReason says why a line's `omit_if_empty` cannot fire, or "" when it
// can. It mirrors the drop conditions in furnitureLine and furnitureRunLine
// exactly; keep the two in step.
func inertLineReason(line theme.Line) string {
	// An image line is never dropped — the picture is the content, so there is
	// nothing for an empty placeholder to take with it.
	if line.Image != nil {
		return "`omit_if_empty` never applies to a line carrying an image"
	}
	if len(line.Runs) == 0 {
		if theme.HasMetaPlaceholder(line.Text) {
			return ""
		}
		return "`omit_if_empty` has no placeholder to be empty on a line of fixed text"
	}
	// A runs line is dropped when its interpolating runs all came up empty, or
	// when it produced no text at all. Literal runs that do produce text make
	// both impossible.
	for _, r := range line.Runs {
		if theme.HasMetaPlaceholder(r.Text) {
			return ""
		}
	}
	for _, r := range line.Runs {
		if strings.TrimSpace(r.Text) != "" {
			return "`omit_if_empty` has no placeholder to be empty on a line of fixed runs"
		}
	}
	return ""
}
