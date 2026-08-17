package main

// `docc doctor` answers the two questions the CLI could not answer before:
// which configuration am I actually using, and is it coherent?
//
// The first matters because schemas are discovered by walking up from a path,
// the way git finds .git — a nested .docc, or a stale one in a parent, silently
// changes what a document is validated against, and nothing printed the
// resolved location. The second matters because emit.Validate — the check that
// a schema and its theme agree — only ran as a side effect of building a valid
// document, so a broken profile stayed invisible until someone had authored one.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kevinzehnder/docc/internal/emit"
	"github.com/kevinzehnder/docc/internal/project"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
	"github.com/kevinzehnder/docc/internal/theme"
)

// doctorReport is the JSON shape of a configuration report.
type doctorReport struct {
	// Start is the path discovery began from.
	Start string `json:"start"`
	// Root is the directory containing .docc, empty when the directories were
	// given explicitly.
	Root      string `json:"root,omitempty"`
	SchemaDir string `json:"schema_dir"`
	// SchemaSource is "discovered" or "--schema-dir".
	SchemaSource string          `json:"schema_source"`
	ThemeDir     string          `json:"theme_dir,omitempty"`
	ThemeSource  string          `json:"theme_source,omitempty"`
	ThemeError   string          `json:"theme_error,omitempty"`
	Types        []doctorType    `json:"types"`
	Themes       []doctorTheme   `json:"themes,omitempty"`
	Problems     []doctorProblem `json:"problems,omitempty"`
	// Warnings are findings that do not stop a build: a style mapping nothing
	// reads renders exactly as if it were absent. --strict promotes them.
	Warnings []doctorProblem `json:"warnings,omitempty"`
	OK       bool            `json:"ok"`
}

type doctorType struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Theme       string `json:"theme,omitempty"`
	// Status is "ok", "check-only" (no theme, so it cannot be built) or "error".
	Status string `json:"status"`
}

type doctorTheme struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Styles      int    `json:"styles"`
}

type doctorProblem struct {
	Type    string `json:"type"`
	Theme   string `json:"theme,omitempty"`
	Message string `json:"message"`
}

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if code, stop := parseFlags(fs, doctorHelp, args); stop {
		return code
	}
	if fs.NArg() > 1 {
		return failf(cf, exitUsage, "docc doctor: expects at most one path")
	}
	start := "."
	if fs.NArg() == 1 {
		start = fs.Arg(0)
	}

	rep, err := diagnose(cf, start)
	if err != nil {
		return fail(cf, exitConfig, err)
	}

	if cf.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return fail(cf, exitDiag, err)
		}
	} else {
		printDoctor(rep)
	}
	// A profile that cannot render is a configuration error, which is the whole
	// subject of this command.
	if !rep.OK {
		return exitConfig
	}
	return exitOK
}

// diagnose resolves the configuration and checks every schema against the theme
// it names. Only a failure to resolve schemas at all is an error: a project
// with no themes still has a describable contract, so that is reported as a
// finding rather than aborting the report.
func diagnose(cf commonFlags, start string) (*doctorReport, error) {
	rep := &doctorReport{Start: start, OK: true}

	if proj, err := project.Resolve(start); err == nil {
		rep.Root = proj.Root
	}
	schemas, err := loadSchemas(cf.schemaDir, start)
	if err != nil {
		return nil, err
	}
	rep.SchemaDir = schemas.Root
	rep.SchemaSource = "discovered"
	if cf.schemaDir != "" {
		rep.SchemaSource = "--schema-dir"
	}

	themes, themeDir, _, themeErr := loadThemes(cf.themeDir, start)
	if themeErr != nil {
		rep.ThemeError = themeErr.Error()
	} else {
		rep.ThemeDir = themeDir
		rep.ThemeSource = "discovered"
		if cf.themeDir != "" {
			rep.ThemeSource = "--theme-dir"
		}
		for _, name := range themes.Names() {
			th, _ := themes.Get(name)
			rep.Themes = append(rep.Themes, doctorTheme{
				Name: th.Name, Description: th.Description, Styles: len(th.Styles),
			})
		}
	}

	for _, t := range schemas.Types() {
		sc, err := schemas.Get(t)
		if err != nil {
			continue
		}
		entry := doctorType{Type: sc.Type, Description: sc.Description, Theme: sc.Theme, Status: "ok"}

		// A mapping the emitter never reads is silent in every other way: it
		// passes Validate, it renders, and it changes nothing.
		for _, unread := range emit.UnreadStyleKeys(sc) {
			rep.Warnings = append(rep.Warnings, doctorProblem{
				Type: sc.Type, Message: "styles: " + unread,
			})
		}

		// A rule scoped to a block the schema never requires is worse than a
		// mapping nothing reads: it reports success for a document it never
		// examined.
		for _, unguarded := range sema.UnguardedDivRules(sc) {
			rep.Warnings = append(rep.Warnings, doctorProblem{
				Type: sc.Type, Message: unguarded,
			})
		}

		switch {
		case sc.Theme == "":
			// Not a defect. A base or check-only type deliberately has no theme.
			entry.Status = "check-only"
		case themeErr != nil:
			entry.Status = "error"
			rep.Problems = append(rep.Problems, doctorProblem{
				Type: sc.Type, Theme: sc.Theme,
				Message: "no themes could be loaded, so this type cannot be built",
			})
		default:
			if err := checkPair(sc, themes); err != nil {
				entry.Status = "error"
				rep.Problems = append(rep.Problems, doctorProblem{
					Type: sc.Type, Theme: sc.Theme, Message: err.Error(),
				})
			}
		}
		rep.Types = append(rep.Types, entry)
	}

	// --strict is the caller asking for the warnings to bind, the same bargain
	// `check` and `build` offer.
	if cf.strict {
		rep.Problems = append(rep.Problems, rep.Warnings...)
		rep.Warnings = nil
	}
	rep.OK = len(rep.Problems) == 0
	return rep, nil
}

// checkPair runs the schema-and-theme agreement check that until now only ran
// inside a build.
func checkPair(sc *schema.Schema, themes *theme.Set) error {
	th, err := themes.Get(sc.Theme)
	if err != nil {
		return err
	}
	return emit.Validate(sc, th)
}

func printDoctor(rep *doctorReport) {
	fmt.Println("configuration:")
	if rep.Root != "" {
		fmt.Printf("  project root   %s\n", rep.Root)
	}
	fmt.Printf("  schemas        %s  (%s)\n", rep.SchemaDir, rep.SchemaSource)
	if rep.ThemeError != "" {
		fmt.Printf("  themes         unavailable: %s\n", rep.ThemeError)
	} else {
		fmt.Printf("  themes         %s  (%s)\n", rep.ThemeDir, rep.ThemeSource)
	}

	fmt.Println("\ndocument types:")
	w := 0
	for _, t := range rep.Types {
		w = max(w, len(t.Type))
	}
	for _, t := range rep.Types {
		switch t.Status {
		case "check-only":
			fmt.Printf("  %-*s  check-only  declares no theme, cannot be built\n", w, t.Type)
		case "error":
			fmt.Printf("  %-*s  ERROR       theme %s\n", w, t.Type, t.Theme)
		default:
			fmt.Printf("  %-*s  ok          theme %s\n", w, t.Type, t.Theme)
		}
	}

	if len(rep.Themes) > 0 {
		fmt.Println("\nthemes:")
		w := 0
		for _, th := range rep.Themes {
			w = max(w, len(th.Name))
		}
		for _, th := range rep.Themes {
			fmt.Printf("  %-*s  %2d styles  %s\n", w, th.Name, th.Styles, th.Description)
		}
	}

	if len(rep.Warnings) > 0 {
		fmt.Printf("\n%d warning(s) — a build succeeds with these; --strict binds them:\n", len(rep.Warnings))
		for _, w := range rep.Warnings {
			fmt.Printf("  %s: %s\n", w.Type, w.Message)
		}
	}

	if len(rep.Problems) == 0 {
		fmt.Println("\nno problems found")
		return
	}
	fmt.Printf("\n%d problem(s):\n", len(rep.Problems))
	for _, p := range rep.Problems {
		if p.Theme == "" {
			fmt.Printf("\n  %s\n", p.Type)
		} else {
			fmt.Printf("\n  %s → theme %s\n", p.Type, p.Theme)
		}
		// emit.Validate joins its findings with newlines; keep them under the
		// indent rather than printing one very long line.
		for line := range strings.SplitSeq(p.Message, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}
}
