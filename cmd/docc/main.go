// Command docc compiles structured markdown documents: it checks them against a
// schema and renders them to Word documents through a theme.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/emit"
	"github.com/kevinzehnder/docc/internal/ingest"
	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/lsp"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/project"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
	"github.com/kevinzehnder/docc/internal/starter"
	"github.com/kevinzehnder/docc/internal/theme"
)

// buildVersion is stamped by the build via -ldflags.
var buildVersion = "dev"

const usage = `docc — a compiler for structured documents

usage:
  docc check [flags] <file.md>...   validate documents against their schema
  docc build [flags] <file.md>      validate, then render to .docx or .pdf
  docc ingest [flags] <file>...     draft markdown from a PDF or image via a local VLM
  docc init [directory]             create a generic starter project
  docc lsp [flags]                  start a Language Server Protocol server
  docc types [flags]                list known document types
  docc themes [flags]               list available themes
  docc explain <CODE>               describe a diagnostic code
  docc version

flags:
  --schema-dir <dir>   schema directory (default: nearest .docc/schemas)
  --theme-dir <dir>    theme directory (default: nearest .docc/themes)
  --type <type>        override the frontmatter document_type
  --json               machine-readable output
  --strict             treat warnings as errors
  --no-color           disable coloured output

build flags:
  --to docx|pdf        output format (default: docx)
  --output <path>      output path (default: input with the new extension)
  --theme <name>       theme to render with (default: the schema's own)
  --force              render despite validation errors

ingest flags:
  --type <type>        document_type to write into the output frontmatter
  --dpi <n>            page rasterization DPI (default: from .docc/ingest.yaml, or 200)
  --pages <n|n-m>      page range to convert, e.g. 3 or 3-5 (default: the whole document)
  --no-anchor          disable born-digital text-layer anchoring
  --model <name>       VLM model name (default: from .docc/ingest.yaml)
  --endpoint <url>     VLM chat completions endpoint (default: from .docc/ingest.yaml)
  --output <path>      output path (single input file only; default: input with .md extension)
  --force              overwrite an output file that docc ingest did not write
  --json               machine-readable output

exit codes:
  0  no errors
  1  diagnostics reported, or the build failed
  2  usage or configuration error
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "check":
		return cmdCheck(rest)
	case "build":
		return cmdBuild(rest)
	case "ingest":
		return cmdIngest(rest)
	case "init":
		return cmdInit(rest)
	case "lsp":
		return cmdLSP(rest)
	case "types":
		return cmdTypes(rest)
	case "themes":
		return cmdThemes(rest)
	case "explain":
		return cmdExplain(rest)
	case "version":
		fmt.Println("docc", buildVersion)
		return 0
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "docc: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}

type commonFlags struct {
	schemaDir string
	themeDir  string
	docType   string
	jsonOut   bool
	strict    bool
	noColor   bool
}

func (c *commonFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&c.schemaDir, "schema-dir", "", "schema directory (default: nearest .docc/schemas)")
	fs.StringVar(&c.themeDir, "theme-dir", "", "theme directory (default: nearest .docc/themes)")
	fs.StringVar(&c.docType, "type", "", "override the frontmatter document_type")
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable output")
	fs.BoolVar(&c.strict, "strict", false, "treat warnings as errors")
	fs.BoolVar(&c.noColor, "no-color", false, "disable coloured output")
}

// color reports whether to emit ANSI colour. Redirected output and NO_COLOR
// both disable it, so piping diagnostics into a file or an agent stays clean.
func (c *commonFlags) color() bool {
	if c.noColor || c.jsonOut || os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	files := fs.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "docc check: no input files")
		return 2
	}

	set, err := loadSchemas(cf.schemaDir, files[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 2
	}

	var all diag.List
	sources := map[string][]byte{}
	for _, path := range files {
		src, err := os.ReadFile(path) //nolint:gosec // paths are the user's own arguments
		if err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 2
		}
		name := displayPath(path)
		sources[name] = src

		f, parseDiags := parse.Parse(name, src)
		res := sema.Check(f, set, parseDiags, cf.docType)
		all = append(all, res.Diagnostics...)
	}

	return report(all, sources, cf)
}

func report(ds diag.List, sources map[string][]byte, cf commonFlags) int {
	if cf.strict {
		for i := range ds {
			ds[i].Severity = diag.Error
		}
	}
	if cf.jsonOut {
		if err := ds.RenderJSON(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 2
		}
	} else {
		src := func(name string) string { return string(sources[name]) }
		if err := ds.Render(os.Stdout, src, cf.color()); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 2
		}
	}
	if ds.HasErrors() {
		return 1
	}
	return 0
}

// cmdLSP serves editor diagnostics over stdio. Protocol messages must use
// stdout exclusively, so errors are reported on stderr.
func cmdLSP(args []string) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: docc lsp [--schema-dir <dir>] [--type <type>]")
		return 2
	}
	if err := lsp.Serve(os.Stdin, os.Stdout, lsp.Options{
		SchemaDir: cf.schemaDir,
		DocType:   cf.docType,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "docc lsp:", err)
		return 1
	}
	return 0
}

func cmdInit(args []string) int {
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "usage: docc init [directory]")
		return 2
	}
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	if err := starter.Init(dir); err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 1
	}
	fmt.Printf("created docc starter in %s\n", dir)
	return 0
}

func cmdTypes(args []string) int {
	fs := flag.NewFlagSet("types", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	start := "."
	if fs.NArg() > 0 {
		start = fs.Arg(0)
	}
	set, err := loadSchemas(cf.schemaDir, start)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 2
	}
	for _, t := range set.Types() {
		sc, _ := set.Get(t)
		if cf.jsonOut {
			fmt.Printf("{\"type\":%q,\"description\":%q,\"theme\":%q}\n", sc.Type, sc.Description, sc.Theme)
			continue
		}
		fmt.Printf("%-14s %s\n", sc.Type, sc.Description)
	}
	return 0
}

func cmdThemes(args []string) int {
	fs := flag.NewFlagSet("themes", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	start := "."
	if fs.NArg() > 0 {
		start = fs.Arg(0)
	}
	set, _, err := loadThemes(cf.themeDir, start)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 2
	}
	for _, name := range set.Names() {
		th, _ := set.Get(name)
		if cf.jsonOut {
			fmt.Printf("{\"name\":%q,\"description\":%q,\"styles\":%d}\n", th.Name, th.Description, len(th.Styles))
			continue
		}
		fmt.Printf("%-14s %s\n", th.Name, th.Description)
	}
	return 0
}

func cmdBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	var (
		to        = fs.String("to", "docx", "output format: docx or pdf")
		output    = fs.String("output", "", "output path")
		themeName = fs.String("theme", "", "theme to render with")
		force     = fs.Bool("force", false, "render despite validation errors")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "docc build: expects exactly one input file")
		return 2
	}
	if *to != "docx" && *to != "pdf" {
		fmt.Fprintf(os.Stderr, "docc build: unknown format %q — use docx or pdf\n", *to)
		return 2
	}

	input := fs.Arg(0)
	src, err := os.ReadFile(input) //nolint:gosec // the user's own argument
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 2
	}
	name := displayPath(input)

	schemas, err := loadSchemas(cf.schemaDir, input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 2
	}
	themes, themeDir, err := loadThemes(cf.themeDir, input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 2
	}

	f, parseDiags := parse.Parse(name, src)
	res := sema.Check(f, schemas, parseDiags, cf.docType)

	// Validation gates the build. Reporting the diagnostics and stopping is the
	// whole point: a document that fails its schema should not reach a court.
	if res.Diagnostics.HasErrors() && !*force {
		_ = res.Diagnostics.Render(os.Stderr, func(string) string { return string(src) }, cf.color())
		fmt.Fprintln(os.Stderr, "\nrefusing to build — fix the errors above, or pass --force")
		return 1
	}
	if len(res.Diagnostics) > 0 {
		_ = res.Diagnostics.Render(os.Stderr, func(string) string { return string(src) }, cf.color())
	}
	if res.Schema == nil {
		fmt.Fprintln(os.Stderr, "docc: no schema resolved; cannot build")
		return 1
	}

	wanted := *themeName
	if wanted == "" {
		wanted = res.Schema.Theme
	}
	if wanted == "" {
		fmt.Fprintf(os.Stderr, "docc: schema %q declares no theme and none was given (--theme)\n", res.Schema.Type)
		return 2
	}
	th, err := themes.Get(wanted)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 2
	}

	doc := ir.Build(f, res.DocType, res.Meta.Values)
	built, err := emit.Build(doc, res.Schema, th, emit.Options{ThemeDir: themeDir})
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 1
	}

	outPath := *output
	if outPath == "" {
		outPath = strings.TrimSuffix(input, filepath.Ext(input)) + "." + *to
	}

	docxPath := outPath
	if *to == "pdf" {
		docxPath = strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".docx"
	}
	if err := built.Write(docxPath); err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 1
	}

	if *to == "pdf" {
		if err := emit.ToPDF(docxPath, outPath, emit.PDFOptions{Retries: 1}); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 1
		}
		// The intermediate .docx is not what was asked for.
		_ = os.Remove(docxPath)
	}

	if cf.jsonOut {
		fmt.Printf("{\"ok\":true,\"type\":%q,\"theme\":%q,\"format\":%q,\"output\":%q}\n",
			res.DocType, th.Name, *to, outPath)
	} else {
		fmt.Println(outPath)
	}
	return 0
}

// cmdIngest converts each input PDF or image into a plain markdown draft via
// a locally hosted VLM. It does no schema fitting — adapting the result to a
// specific docc document type (frontmatter fields, section structure) is a
// later editing pass, not ingest's job. Nothing it produces is trusted
// automatically; run docc check once a draft has been adapted.
func cmdIngest(args []string) int {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	var (
		docType  = fs.String("type", "", "document_type to write into the output frontmatter")
		jsonOut  = fs.Bool("json", false, "machine-readable output")
		dpi      = fs.Int("dpi", 0, "page rasterization DPI (default: from .docc/ingest.yaml, or 200)")
		noAnchor = fs.Bool("no-anchor", false, "disable born-digital text-layer anchoring")
		model    = fs.String("model", "", "VLM model name (default: from .docc/ingest.yaml)")
		endpoint = fs.String("endpoint", "", "VLM chat completions endpoint (default: from .docc/ingest.yaml)")
		output   = fs.String("output", "", "output path (single input file only; default: input with .md extension)")
		pages    = fs.String("pages", "", "page range to convert, e.g. 3 or 3-5 (default: the whole document)")
		force    = fs.Bool("force", false, "overwrite an output file that docc ingest did not write")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	files := fs.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "docc ingest: no input files")
		return 2
	}
	if *output != "" && len(files) > 1 {
		fmt.Fprintln(os.Stderr, "docc ingest: --output requires exactly one input file")
		return 2
	}
	// Every input and every destination is checked before any of them is
	// converted: a run that transcribes the first file and only then discovers
	// the second is a typo has spent minutes of VLM time to report a usage
	// error, and one that discovers the collision afterwards has already
	// written over the file it was meant to protect.
	badArgs := false
	for _, input := range files {
		if err := ingest.CheckInput(input); err != nil {
			fmt.Fprintln(os.Stderr, "docc ingest:", err)
			badArgs = true
			continue
		}
		if err := checkIngestOutput(ingestOutputPath(input, *output), *force); err != nil {
			fmt.Fprintln(os.Stderr, "docc ingest:", err)
			badArgs = true
		}
	}
	if badArgs {
		return 2
	}
	firstPage, lastPage, err := parsePageRange(*pages)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc ingest:", err)
		return 2
	}

	cfg, err := resolveIngestConfig(files[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return 2
	}
	if *dpi != 0 {
		cfg.DPI = *dpi
	}
	if *noAnchor {
		cfg.Anchor = false
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *endpoint != "" {
		cfg.Endpoint = *endpoint
	}
	if cfg.Model == "" {
		fmt.Fprintln(os.Stderr, "docc ingest: no model configured — pass --model or set model in .docc/ingest.yaml")
		return 2
	}

	// A conversion is minutes of GPU time. Ctrl-C cancels the request in
	// flight, and the pages already transcribed are still written out below.
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	exitCode := 0
	for _, input := range files {
		outPath := ingestOutputPath(input, *output)

		prog := newProgress(os.Stderr, progressModeFor(*jsonOut))
		prog.begin(input)
		attempted := 0
		md, done, err := ingest.Convert(ctx, input, cfg, ingest.ConvertOptions{
			First: firstPage, Last: lastPage, DocType: *docType,
			Progress: func(ev ingest.Event) {
				if ev.Total > 0 {
					attempted = ev.Total
				}
				prog.event(ev)
			},
		})
		prog.finish(err)

		switch {
		case err != nil && md != "":
			if werr := os.WriteFile(outPath, []byte(md), 0o644); werr != nil { //nolint:gosec // draft output, not a secret
				fmt.Fprintf(os.Stderr, "docc ingest: %s: %v\n", input, werr)
				exitCode = 1
				continue
			}
			// The partial path is announced on stderr even in text mode:
			// stdout carries the path of a finished draft only, so
			// `out=$(docc ingest x.pdf)` cannot hand a script half a document.
			fmt.Fprintf(os.Stderr, "docc ingest: %s: %v\n", input, err)
			fmt.Fprintf(os.Stderr, "docc ingest: wrote %d of %d pages to %s\n", len(done), attempted, outPath)
			if *jsonOut {
				fmt.Printf("{\"ok\":false,\"partial\":true,\"output\":%q,\"pages\":%d,\"attempted\":%d}\n",
					outPath, len(done), attempted)
			}
			exitCode = 1

		case err != nil:
			fmt.Fprintf(os.Stderr, "docc ingest: %s: %v\n", input, err)
			exitCode = 1

		default:
			if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil { //nolint:gosec // draft output, not a secret
				fmt.Fprintf(os.Stderr, "docc ingest: %s: %v\n", input, err)
				exitCode = 1
				continue
			}
			if *jsonOut {
				fmt.Printf("{\"ok\":true,\"output\":%q}\n", outPath)
			} else {
				fmt.Println(outPath)
			}
		}
	}
	return exitCode
}

// ingestOutputPath is where one input's draft goes: the explicit --output, or
// the input's own name with a .md extension.
func ingestOutputPath(input, output string) string {
	if output != "" {
		return output
	}
	return strings.TrimSuffix(input, filepath.Ext(input)) + ".md"
}

// checkIngestOutput refuses to write over work that cannot be recovered.
//
// Re-running a conversion over ingest's own finished output is the normal
// iteration loop and stays silent — the same command reproduces it. Two cases
// are not reproducible: a file whose generated-by banner is gone has been
// adapted by hand, and a partial draft holds the pages of a run that stopped,
// which a resume over a different page range will not produce again.
func checkIngestOutput(path string, force bool) error {
	if force {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	info, err := ingest.InspectDraft(path)
	if err != nil {
		return err
	}
	switch {
	case !info.Generated:
		return fmt.Errorf("%s already exists and was not written by docc ingest — it may hold edits; write elsewhere with --output, or pass --force to overwrite it", path)
	case info.Incomplete:
		return fmt.Errorf("%s holds a partial draft from a run that stopped early — overwriting it loses those pages; write elsewhere with --output, or pass --force", path)
	default:
		return nil
	}
}

// resolveIngestConfig loads .docc/ingest.yaml the same way loadSchemas
// resolves the schema directory: nearest .docc above the first input file. A
// project that has no ingest.yaml, or no .docc directory at all, still gets
// Defaults() — ingest works from flags alone.
func resolveIngestConfig(start string) (ingest.Config, error) {
	proj, err := project.Resolve(start)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return ingest.Defaults(), nil
		}
		return ingest.Config{}, err
	}
	return ingest.LoadConfig(proj.IngestConfigPath())
}

// parsePageRange parses --pages as "N" or "N-M", both 1-based and inclusive.
// An empty string means the whole document.
func parsePageRange(s string) (first, last int, err error) {
	if s == "" {
		return 0, 0, nil
	}
	before, after, hasDash := strings.Cut(s, "-")
	first, err = strconv.Atoi(strings.TrimSpace(before))
	if err != nil || first < 1 {
		return 0, 0, fmt.Errorf("invalid --pages %q: expected N or N-M", s)
	}
	if !hasDash {
		return first, first, nil
	}
	last, err = strconv.Atoi(strings.TrimSpace(after))
	if err != nil || last < first {
		return 0, 0, fmt.Errorf("invalid --pages %q: expected N or N-M", s)
	}
	return first, last, nil
}

func cmdExplain(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: docc explain <CODE>")
		return 2
	}
	code := strings.ToUpper(args[0])
	if text, ok := explanations[code]; ok {
		fmt.Printf("%s — %s\n", code, text)
		return 0
	}
	fmt.Fprintf(os.Stderr, "docc: no explanation for %q\n", code)
	return 2
}

// explanations backs `docc explain`. Codes the schema author defines for named
// rules are documented in the schema, not here.
var explanations = map[string]string{
	"REF010": "a reference document's paragraph numbers do not run unbroken. Those numbers are the source document's own citation keys, not something docc generates, so a gap, a repeat or a step backwards means the transcription lost or reordered text — re-convert the affected pages rather than renumbering.",
	"DOC001": "the file has no YAML frontmatter. Every document starts with a `---` delimited block declaring `docc: 1` and `document_type`.",
	"DOC002": "the frontmatter block was opened with `---` but never closed.",
	"DOC003": "the frontmatter is not valid YAML.",
	"DOC004": "a field the schema marks required is missing or empty.",
	"DOC005": "a field has the wrong shape — a list or object was expected.",
	"DOC006": "a field has the wrong scalar type. Numbers that must keep leading zeros, such as Swiss postal codes, have to be quoted.",
	"DOC007": "a date field is not an ISO date (YYYY-MM-DD).",
	"DOC008": "an enum field holds a value the schema does not allow.",
	"DOC009": "the schema itself is wrong: an unknown field type, an invalid pattern, or an unregistered check.",
	"DOC010": "a string field does not match the pattern its schema declares.",
	"DOC011": "the frontmatter declares a field the schema does not know. Usually a typo.",
	"DOC012": "the document type could not be determined from the frontmatter.",
	"DOC013": "the declared document type has no schema.",
	"DOC020": "a section the schema requires is missing from the body.",
	"DOC021": "a conventional but optional section is missing.",
	"DOC022": "a section appears out of the order the schema declares.",
	"DOC023": "a fenced div was opened but not closed. Put the closing `:::` on a line of its own.",
	"DOC024": "the frontmatter does not declare the `docc` marker. A file only becomes a docc document by writing `docc: 1` in its frontmatter; unrelated YAML frontmatter (Hugo, Obsidian, …) is ignored.",
	"DOC025": "the frontmatter declares a docc format version this compiler does not support. Use the version listed in the diagnostic's hint.",
}

// loadSchemas resolves the schema directory: an explicit flag, else the nearest
// .docc directory above the first input file.
func loadSchemas(schemaDir, start string) (*schema.Set, error) {
	if schemaDir != "" {
		return schema.Load(schemaDir)
	}
	proj, err := project.Resolve(start)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return nil, fmt.Errorf("%w\n  create %s/schemas/ in your project, or pass --schema-dir", err, project.DirName)
		}
		return nil, err
	}
	return schema.Load(proj.SchemaDir())
}

// loadThemes resolves the theme directory the same way loadSchemas resolves the
// schema directory, and returns it so theme-relative image paths can be found.
func loadThemes(themeDir, start string) (*theme.Set, string, error) {
	if themeDir != "" {
		set, err := theme.Load(themeDir)
		return set, themeDir, err
	}
	proj, err := project.Resolve(start)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return nil, "", fmt.Errorf("%w\n  create %s/themes/ in your project, or pass --theme-dir", err, project.DirName)
		}
		return nil, "", err
	}
	dir := proj.ThemeDir()
	set, err := theme.Load(dir)
	return set, dir, err
}

// displayPath shortens an absolute path to a working-directory-relative one so
// diagnostics stay readable and stable across machines.
func displayPath(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
