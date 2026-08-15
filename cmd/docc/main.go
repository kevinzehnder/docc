// Command docc compiles structured markdown documents: it checks them against a
// schema and renders them to Word documents through a theme.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/emit"
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
  docc build [flags] <file.md>      validate, then render to .docx (or compatibility PDF)
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
  --to docx|pdf        output format; pdf is compatibility-only and needs soffice (default: docx)
  --output <path>      output path (default: input with the new extension)
  --theme <name>       theme to render with (default: the schema's own)
  --force              render despite validation errors

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

	// Schemas are resolved per file, not once from the first argument. Files
	// named on one command line may live in different projects, and loading one
	// project's contract to check another's document validates against the wrong
	// schema — a silent false pass. The cache keeps a shared project loaded once.
	cache := map[string]*schema.Set{}

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

		set, err := loadSchemasCached(cache, cf.schemaDir, path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 2
		}

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
	if cf.jsonOut {
		type item struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			Theme       string `json:"theme"`
		}
		items := make([]item, 0, len(set.Types()))
		for _, t := range set.Types() {
			sc, _ := set.Get(t)
			items = append(items, item{Type: sc.Type, Description: sc.Description, Theme: sc.Theme})
		}
		if err := json.NewEncoder(os.Stdout).Encode(struct {
			Types []item `json:"types"`
		}{Types: items}); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 2
		}
		return 0
	}
	for _, t := range set.Types() {
		sc, _ := set.Get(t)
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
	if cf.jsonOut {
		type item struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Styles      int    `json:"styles"`
		}
		items := make([]item, 0, len(set.Names()))
		for _, name := range set.Names() {
			th, _ := set.Get(name)
			items = append(items, item{Name: th.Name, Description: th.Description, Styles: len(th.Styles)})
		}
		if err := json.NewEncoder(os.Stdout).Encode(struct {
			Themes []item `json:"themes"`
		}{Themes: items}); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 2
		}
		return 0
	}
	for _, name := range set.Names() {
		th, _ := set.Get(name)
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

	// --strict is the caller asking for the warnings to bind too, so a document
	// that only warns is treated exactly like one that errors: the build stops.
	diags := res.Diagnostics
	if cf.strict {
		for i := range diags {
			diags[i].Severity = diag.Error
		}
	}

	srcText := func(string) string { return string(src) }
	// Diagnostics go to stderr so stdout carries only the output path (or the
	// result object under --json); --json keeps them machine-readable there too,
	// rather than falling back to the human caret rendering.
	emitDiags := func() {
		if len(diags) == 0 {
			return
		}
		if cf.jsonOut {
			_ = diags.RenderJSON(os.Stderr)
		} else {
			_ = diags.Render(os.Stderr, srcText, cf.color())
		}
	}

	// Validation gates the build. Reporting the diagnostics and stopping is the
	// whole point: a document that fails its schema should not reach a court.
	if diags.HasErrors() && !*force {
		emitDiags()
		if cf.jsonOut {
			fmt.Printf("{\"ok\":false,\"error\":\"validation failed\",\"type\":%q}\n", res.DocType)
		} else {
			fmt.Fprintln(os.Stderr, "\nrefusing to build — fix the errors above, or pass --force")
		}
		return 1
	}
	emitDiags()
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

	switch *to {
	case "docx":
		if err := writeAtomic(outPath, built.Write); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 1
		}
	case "pdf":
		// The intermediate .docx is built in a private temp directory, never
		// derived from outPath. Deriving it — outPath with a .docx extension —
		// collides when the caller writes `--to pdf --output x.docx`, and the
		// cleanup step then deletes the file that was just produced.
		tmpDir, err := os.MkdirTemp("", "docc-build-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 1
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		tmpDocx := filepath.Join(tmpDir, "doc.docx")
		if err := built.Write(tmpDocx); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 1
		}
		if err := writeAtomic(outPath, func(dst string) error {
			return emit.ToPDF(tmpDocx, dst, emit.PDFOptions{Retries: 1})
		}); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 1
		}
	}

	if cf.jsonOut {
		fmt.Printf("{\"ok\":true,\"type\":%q,\"theme\":%q,\"format\":%q,\"output\":%q}\n",
			res.DocType, th.Name, *to, outPath)
	} else {
		fmt.Println(outPath)
	}
	return 0
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
	"DOC026": "the `{...}` attribute block on a fenced div did not parse. Attributes are written `{#id key=value key=\"quoted value\"}`, separated by spaces; the diagnostic names the first token that did not lex.",
	"DOC027": "the `{...}` attribute block on an inline span did not parse. A span is written `[literal text]{.type key=value}` on one line; the diagnostic names the first token that did not lex.",
	"DOC030": "the document uses a `:::` block the schema does not declare. The hint lists the declared blocks; check `blocks:` in the schema.",
	"DOC031": "an inline span is missing its type class, or uses one the schema does not declare. The first `.class` in a span's attribute block is its type; the schema's `spans:` section lists the valid ones.",
	"DOC032": "a block with variants is missing its discriminator attribute, or names a variant the schema does not declare — for example a `partei` block without `kind=person` or `kind=company`.",
	"DOC033": "a block is missing a span its schema variant requires. Annotate the value inside the block with the named type: `[...]{.uid}`.",
	"DOC034": "a `#id` is used by more than one block. Ids must be unique in the document because references resolve against them; the diagnostic lists the other occurrence.",
	"DOC035": "a block carries an attribute its schema declaration does not permit. Only `#id`, the discriminator and the keys in the block's `attributes:` list are allowed.",
	"DOC036": "the schema declares `variants:` for a block but no `discriminator:`, so no document can satisfy it. This is a schema bug, not a document bug.",
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

// loadSchemasCached is loadSchemas memoised across the files of one `docc check`
// run. The key is the schema directory each file resolves to — the flag when
// given, otherwise the file's own project — so files sharing a project load its
// schemas once, and files in different projects each get their own contract.
func loadSchemasCached(cache map[string]*schema.Set, schemaDir, start string) (*schema.Set, error) {
	key := schemaDir
	if key == "" {
		if proj, err := project.Resolve(start); err == nil {
			key = proj.Dir
		}
	}
	if key != "" {
		if set, ok := cache[key]; ok {
			return set, nil
		}
	}
	set, err := loadSchemas(schemaDir, start)
	if err != nil {
		return nil, err
	}
	if key != "" {
		cache[key] = set
	}
	return set, nil
}

// writeAtomic writes to a temporary file in the destination's own directory and
// renames it over dest, so an interrupted or failed build never truncates or
// half-writes an existing output. The rename is atomic because the temp file
// shares dest's filesystem. write receives the temp path and must create it.
func writeAtomic(dest string, write func(path string) error) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dest)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_ = tmp.Close()

	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if err := write(tmpName); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	committed = true
	return nil
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
