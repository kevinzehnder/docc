package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/docx"
	"github.com/kevinzehnder/docc/internal/emit"
	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/sema"
)

const diffHelp = `docc diff [flags] <file.md> <edited.docx>

Compare the visible text docc renders from the Markdown with an edited Word
file. Formatting, comments and images are ignored; this reports content for a
human or another tool to apply back to the source.

flags:
`

type contentHunk struct {
	OldLine int      `json:"old_line"`
	NewLine int      `json:"new_line"`
	Old     []string `json:"old"`
	New     []string `json:"new"`
}

type storyDiff struct {
	Name  string        `json:"name"`
	Hunks []contentHunk `json:"hunks"`
}

type diffResult struct {
	OK      bool        `json:"ok"`
	Equal   bool        `json:"equal"`
	Source  string      `json:"source"`
	Against string      `json:"against"`
	Changes int         `json:"changes"`
	Stories []storyDiff `json:"stories"`
}

func cmdDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	themeName := fs.String("theme", "", "theme used to render the baseline (default: the schema's own)")
	if code, stop := parseFlags(fs, diffHelp, args); stop {
		return code
	}
	if fs.NArg() != 2 {
		return failf(cf, exitUsage, "docc diff: requires <file.md> <edited.docx>")
	}

	input, against := fs.Arg(0), fs.Arg(1)
	src, err := os.ReadFile(input) //nolint:gosec // the user's own argument
	if err != nil {
		return fail(cf, exitUsage, err)
	}
	schemas, err := loadSchemas(cf.schemaDir, input)
	if err != nil {
		return fail(cf, exitConfig, err)
	}
	themes, themeDir, resolved, err := loadThemes(cf.themeDir, input)
	if err != nil {
		return fail(cf, exitConfig, err)
	}

	name := displayPath(input)
	f, parseDiags := parse.Parse(name, src)
	res := sema.Check(f, schemas, parseDiags, cf.docType)
	if res.Schema != nil {
		sema.CheckCompletion(f, res.Schema, &res.Diagnostics)
	}
	diags := res.Diagnostics
	if cf.strict {
		for i := range diags {
			diags[i].Severity = diag.Error
		}
	}
	if diags.HasErrors() {
		if cf.jsonOut {
			_ = diags.RenderJSON(os.Stderr)
		} else {
			_ = diags.Render(os.Stderr, func(string) string { return string(src) }, cf.color())
		}
		return exitDiag
	}
	if res.Schema == nil {
		return failf(cf, exitDiag, "no schema resolved; cannot render comparison baseline")
	}

	wanted := *themeName
	if wanted == "" {
		wanted = res.Schema.Theme
	}
	if wanted == "" {
		return failf(cf, exitConfig, "schema %q declares no theme and none was given (--theme)", res.Schema.Type)
	}
	th, err := themes.Get(wanted)
	if err != nil {
		return fail(cf, exitConfig, err)
	}
	if err := emit.Validate(res.Schema, th); err != nil {
		return fail(cf, exitConfig, err)
	}
	built, err := emit.Build(ir.Build(f, res.DocType, res.Meta.Values), res.Schema, th, emit.Options{
		ThemeDir: themeDir, Provenance: resolved.Provenance(),
	})
	if err != nil {
		return fail(cf, exitDiag, err)
	}
	data, err := built.Bytes()
	if err != nil {
		return fail(cf, exitDiag, err)
	}
	expected, err := docx.ReadContentBytes(data)
	if err != nil {
		return fail(cf, exitDiag, err)
	}
	actual, err := docx.ReadContent(against)
	if err != nil {
		return fail(cf, exitDiag, err)
	}

	result := compareContent(name, against, expected, actual)
	if cf.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return fail(cf, exitDiag, err)
		}
	} else {
		renderContentDiff(result)
	}
	if result.Equal {
		return exitOK
	}
	return exitDiag
}

func compareContent(source, against string, expected, actual docx.Content) diffResult {
	oldStories := make(map[string][]string, len(expected.Stories))
	newStories := make(map[string][]string, len(actual.Stories))
	var names []string
	for _, story := range expected.Stories {
		oldStories[story.Name] = story.Records
		names = append(names, story.Name)
	}
	for _, story := range actual.Stories {
		newStories[story.Name] = story.Records
		if _, exists := oldStories[story.Name]; !exists {
			names = append(names, story.Name)
		}
	}
	if len(names) > 1 {
		sort.Strings(names[1:]) // body remains first
	}

	result := diffResult{OK: true, Equal: true, Source: source, Against: against, Stories: []storyDiff{}}
	for _, name := range names {
		hunks := contentHunks(oldStories[name], newStories[name])
		if len(hunks) == 0 {
			continue
		}
		result.Equal = false
		result.Changes += len(hunks)
		result.Stories = append(result.Stories, storyDiff{Name: name, Hunks: hunks})
	}
	return result
}

type lineEdit struct {
	kind byte
	text string
}

// contentHunks uses a small LCS table. Legal documents normally have hundreds,
// not tens of thousands, of paragraphs.
func contentHunks(old, new []string) []contentHunk {
	// ponytail: avoid an unbounded quadratic allocation; replace the unmatched
	// middle wholesale if giant documents ever make a linear-space diff matter.
	if len(old) != 0 && len(new) > 4_000_000/len(old) {
		if strings.Join(old, "\x00") == strings.Join(new, "\x00") {
			return nil
		}
		return []contentHunk{{OldLine: 1, NewLine: 1, Old: old, New: new}}
	}

	dp := make([][]int, len(old)+1)
	for i := range dp {
		dp[i] = make([]int, len(new)+1)
	}
	for i := len(old) - 1; i >= 0; i-- {
		for j := len(new) - 1; j >= 0; j-- {
			if old[i] == new[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var edits []lineEdit
	for i, j := 0, 0; i < len(old) || j < len(new); {
		switch {
		case i < len(old) && j < len(new) && old[i] == new[j]:
			edits = append(edits, lineEdit{' ', old[i]})
			i++
			j++
		case j == len(new) || i < len(old) && dp[i+1][j] >= dp[i][j+1]:
			edits = append(edits, lineEdit{'-', old[i]})
			i++
		default:
			edits = append(edits, lineEdit{'+', new[j]})
			j++
		}
	}

	oldLine, newLine := 1, 1
	var hunks []contentHunk
	for i := 0; i < len(edits); {
		if edits[i].kind == ' ' {
			oldLine++
			newLine++
			i++
			continue
		}
		h := contentHunk{OldLine: oldLine, NewLine: newLine, Old: []string{}, New: []string{}}
		for i < len(edits) && edits[i].kind != ' ' {
			if edits[i].kind == '-' {
				h.Old = append(h.Old, edits[i].text)
				oldLine++
			} else {
				h.New = append(h.New, edits[i].text)
				newLine++
			}
			i++
		}
		hunks = append(hunks, h)
	}
	return hunks
}

func renderContentDiff(result diffResult) {
	if result.Equal {
		return
	}
	fmt.Printf("--- %s (rendered)\n+++ %s\n", result.Source, result.Against)
	for _, story := range result.Stories {
		for _, h := range story.Hunks {
			fmt.Printf("@@ %s -%d +%d @@\n", story.Name, h.OldLine, h.NewLine)
			for _, line := range h.Old {
				printDiffLine('-', line)
			}
			for _, line := range h.New {
				printDiffLine('+', line)
			}
		}
	}
}

func printDiffLine(prefix byte, text string) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		fmt.Printf("%c%s\n", prefix, line)
	}
}
