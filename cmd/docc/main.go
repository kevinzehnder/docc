// Command docc compiles structured markdown documents: it checks them against a
// schema and renders them to Word documents through a theme.
package main

import (
	"bytes"
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
	"github.com/kevinzehnder/docc/internal/profile"
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
  docc init [flags] [directory]     create an editable starter profile pack
  docc profile <command>            install, select, inspect, or update profile packs
  docc doctor [flags] [path]        report the resolved configuration and check it
  docc lsp [flags]                  start a Language Server Protocol server
  docc types [flags]                list known document types
  docc describe [flags] <type>      report a document type's full contract
  docc example [flags] <type>       print a compact valid document of a type
  docc themes [flags]               list available themes
  docc explain [flags] [CODE]       describe a diagnostic code, or list them all
  docc version                      print the version (also --version)

Flags may appear before or after the positional arguments. Use "--" to end flag
parsing when a file name begins with a dash.

flags:
  --schema-dir <dir>   schema directory (default: the resolved profile's)
  --theme-dir <dir>    theme directory (default: the resolved profile's)
  --type <type>        override the frontmatter document_type
  --json               machine-readable output
  --strict             treat warnings as errors
  --no-color           disable coloured output

describe, example and explain also take:
  --from <path>        resolve the project from this path, not the working directory

build flags:
  --to docx|pdf        output format; pdf is compatibility-only and needs soffice (default: docx)
  --output <path>      output path (default: input with the new extension)
  --theme <name>       theme to render with (default: the schema's own)
  --force              render despite validation errors

exit codes:
  0  no errors
  1  diagnostics reported, or the build failed
  2  usage error — the command line is wrong
  3  configuration error — the project's schemas or themes are missing or unusable
`

// The per-subcommand help pages. They live beside the top-level usage string so
// a new command or flag is documented in one place, and `parseFlags` appends the
// flag defaults from the set itself, which cannot drift.
const (
	checkHelp = `docc check [flags] <file.md>...

Validate documents against their schema. Each file resolves its own project, so
files from different projects may be named in one invocation.

flags:
`
	buildHelp = `docc build [flags] <file.md>

Validate one document and render it through its theme. Validation gates the
build; --force renders anyway.

flags:
`
	initHelp = `docc init [flags] [directory]

Create an editable profile-pack checkout: docc-profile.yaml, schemas/ and
themes/ copied from the starter pack built into docc, plus sample documents in
examples/. Refuses to overwrite an existing pack. The result is yours to edit —
it is a starting point, not a managed install. For a Git-managed profile, use
"docc profile use" instead.

flags:
`
	profileHelp = `docc profile <command> [flags]

Manage Git-backed profile packs. A profile pack supplies schemas, themes and
assets without copying them into every document project.

commands:
  install [--ref REF] [--default] REPOSITORY
  use [--ref REF] [--project DIR] REPOSITORY
  update [--project DIR]
  status [--project DIR]

Run "docc profile <command> --help" for command-specific help.
`
	doctorHelp = `docc doctor [flags] [path]

Report which profile, schemas and themes are in effect for path (default
the working directory), then check every schema against the theme it names.

flags:
`
	lspHelp = `docc lsp [flags]

Serve diagnostics to an editor over stdio.

flags:
`
	typesHelp = `docc types [flags] [path]

List the document types the project supplies. path selects which project.

flags:
`
	themesHelp = `docc themes [flags] [path]

List the themes the project supplies. path selects which project.

flags:
`
	describeHelp = `docc describe [flags] <type>

Report a document type's whole contract: frontmatter fields with their
constraints, the body outline, blocks, spans, blanks and rules. Use --json for
the machine-readable form.

flags:
`
	exampleHelp = `docc example [flags] <type>

Print a compact, complete document of the type, ready to edit.

With --blank, every field marker is emptied and the rest is left alone: the
result is a skeleton whose blanks are the decisions the type requires. It still
passes "docc check", because a blank is content rather than missing content,
and "docc build" refuses it while naming each position left to decide.

flags:
`
	explainHelp = `docc explain [flags] [CODE]

Describe a diagnostic code. Without a code, list every code docc emits. With
--type, add the constraints that type's schema actually declares.

flags:
`
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return exitUsage
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "check":
		return cmdCheck(rest)
	case "build":
		return cmdBuild(rest)
	case "init":
		return cmdInit(rest)
	case "profile":
		return cmdProfile(rest)
	case "doctor":
		return cmdDoctor(rest)
	case "lsp":
		return cmdLSP(rest)
	case "types":
		return cmdTypes(rest)
	case "describe":
		return cmdDescribe(rest)
	case "example":
		return cmdExample(rest)
	case "themes":
		return cmdThemes(rest)
	case "explain":
		return cmdExplain(rest)
	// `--version` is what a CI step or a Makefile reaches for first, and every
	// other tool on the PATH answers it. `-v` is deliberately absent: it reads
	// as "verbose" often enough that answering it with a version is a trap.
	case "version", "--version":
		fmt.Println("docc", buildVersion)
		return 0
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "docc: unknown command %q\n\n%s", cmd, usage)
		return exitUsage
	}
}

type commonFlags struct {
	schemaDir string
	themeDir  string
	docType   string
	// from is the path project discovery starts at, for commands that name a
	// document type rather than a file. Bound by bindFrom, not bind.
	from    string
	jsonOut bool
	strict  bool
	noColor bool
}

func (c *commonFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&c.schemaDir, "schema-dir", "", "schema directory (default: the resolved profile's)")
	fs.StringVar(&c.themeDir, "theme-dir", "", "theme directory (default: the resolved profile's)")
	fs.StringVar(&c.docType, "type", "", "override the frontmatter document_type")
	fs.BoolVar(&c.jsonOut, "json", false, "machine-readable output")
	fs.BoolVar(&c.strict, "strict", false, "treat warnings as errors")
	fs.BoolVar(&c.noColor, "no-color", false, "disable coloured output")
}

// bindFrom adds --from for the commands whose positional argument is a document
// type, not a file. Without it they resolve the project from the working
// directory only, so a type could not be described from outside its own tree
// without naming both directories by hand.
func (c *commonFlags) bindFrom(fs *flag.FlagSet) {
	fs.StringVar(&c.from, "from", "", "resolve the project from this path (default: the working directory)")
}

// start is the path discovery begins at.
func (c *commonFlags) start() string {
	if c.from == "" {
		return "."
	}
	return c.from
}

// Exit codes. Usage and configuration are separated because a caller can do
// something about the difference: a usage error means the command line is wrong
// and retrying it differently may work, while a configuration error means the
// project is wrong and no invocation will help. They shared code 2 for a long
// time, which left an agent guessing.
const (
	exitOK     = 0 // no errors
	exitDiag   = 1 // the command ran and reported diagnostics, or failed part-way
	exitUsage  = 2 // the command line is wrong
	exitConfig = 3 // the project's schemas or themes are missing or unusable
)

// fail reports a terminal error and returns the exit code to use. Under --json
// it writes a failure object to stdout, so a consumer parsing that stream sees
// the failure there rather than having to notice an empty result and go looking
// on stderr.
func fail(cf commonFlags, code int, err error) int {
	if cf.jsonOut {
		out := struct {
			OK    bool   `json:"ok"`
			Kind  string `json:"kind"`
			Error string `json:"error"`
		}{OK: false, Kind: failureKind(code), Error: err.Error()}
		if err := json.NewEncoder(os.Stdout).Encode(out); err == nil {
			return code
		}
		// Fall through to the human form rather than exiting silently.
	}
	fmt.Fprintln(os.Stderr, "docc:", err)
	return code
}

// failf is fail with a formatted message.
func failf(cf commonFlags, code int, format string, args ...any) int {
	return fail(cf, code, fmt.Errorf(format, args...))
}

func failureKind(code int) string {
	switch code {
	case exitUsage:
		return "usage"
	case exitConfig:
		return "config"
	default:
		return "error"
	}
}

// permute reorders args so flags may follow the positional arguments. Go's flag
// package stops at the first non-flag, which turns the natural
// `docc build file.md --output x.docx` into "expects exactly one input file" —
// an error about the wrong thing entirely.
//
// A `--` terminates flag scanning: everything after it is positional, so a file
// whose name begins with a dash stays reachable. A flag name the set does not
// know is left where it is, so fs.Parse still reports it.
func permute(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			positional = append(positional, arg)
			continue
		}

		name := strings.TrimLeft(arg, "-")
		hasValue := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, hasValue = name[:eq], true
		}
		f := fs.Lookup(name)
		if f == nil {
			// Unknown: leave it in place and let fs.Parse produce the error it
			// would have produced anyway.
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		if hasValue || isBoolFlag(f) {
			continue
		}
		// A value flag consumes the next token, wherever it sits.
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

// isBoolFlag reports whether a flag is satisfied without a following value.
// The stdlib marks these with an unexported interface, which is the only way to
// tell `--json file.md` from `--output file.md`.
func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// parseFlags parses one subcommand's arguments, permuting flags that follow the
// positionals, and turns a help request into a complete usage page on stdout
// with a successful exit. stop reports whether the caller should return code.
func parseFlags(fs *flag.FlagSet, help string, args []string) (code int, stop bool) {
	// The flag package prints the error and the usage itself, on the way to
	// returning. Buffering that keeps the choice of stream ours: a help request
	// belongs on stdout, a malformed one on stderr.
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	fs.Usage = func() {
		fmt.Fprint(&buf, help)
		fs.PrintDefaults()
	}

	err := fs.Parse(permute(fs, args))
	switch {
	case err == nil:
		return 0, false
	case errors.Is(err, flag.ErrHelp):
		// A help request is a successful request. Exiting 2 surprises scripts
		// and readers alike.
		fmt.Print(help)
		fs.SetOutput(os.Stdout)
		fs.PrintDefaults()
		return 0, true
	default:
		_, _ = os.Stderr.Write(buf.Bytes())
		return 2, true
	}
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
	if code, stop := parseFlags(fs, checkHelp, args); stop {
		return code
	}
	files := fs.Args()
	if len(files) == 0 {
		return failf(cf, exitUsage, "docc check: no input files")
	}

	// Schemas are resolved per file, not once from the first argument. Files
	// named on one command line may live in different projects, and loading one
	// project's contract to check another's document validates against the wrong
	// schema — a silent false pass. The cache keeps a shared project loaded once.
	cache := map[string]*schema.Set{}

	var all diag.List
	// Occurrences of the span types the schemas ask to be consistent, gathered
	// across every file in this invocation. The dossier check needs no dossier
	// format: `docc check *.md` already has the whole set open.
	var occurrences []sema.SpanOccurrence
	sources := map[string][]byte{}
	for _, path := range files {
		src, err := os.ReadFile(path) //nolint:gosec // paths are the user's own arguments
		if err != nil {
			return fail(cf, exitUsage, err)
		}
		name := displayPath(path)
		sources[name] = src

		set, err := loadSchemasCached(cache, cf.schemaDir, path)
		if err != nil {
			return fail(cf, exitConfig, err)
		}

		f, parseDiags := parse.Parse(name, src)
		res := sema.Check(f, set, parseDiags, cf.docType)
		all = append(all, res.Diagnostics...)
		if res.Schema != nil {
			occurrences = append(occurrences, sema.WatchedSpanValues(f, res.Schema)...)
		}
	}

	// Only with more than one file: a document cannot disagree with itself
	// across files, and `spans_agree` already covers it within one.
	if len(files) > 1 {
		all = append(all, sema.CrossFileDisagreements(occurrences)...)
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
			return fail(cf, exitDiag, err)
		}
	} else {
		src := func(name string) string { return string(sources[name]) }
		if err := ds.Render(os.Stdout, src, cf.color()); err != nil {
			return fail(cf, exitDiag, err)
		}
	}
	if ds.HasErrors() {
		return exitDiag
	}
	return exitOK
}

// cmdLSP serves editor diagnostics over stdio. Protocol messages must use
// stdout exclusively, so errors are reported on stderr.
func cmdLSP(args []string) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if code, stop := parseFlags(fs, lspHelp, args); stop {
		return code
	}
	if fs.NArg() != 0 {
		// The lsp failure paths stay human: stdout carries the protocol, so a
		// JSON failure object there would corrupt the stream.
		fmt.Fprintln(os.Stderr, "docc lsp: takes no positional arguments")
		return exitUsage
	}
	if err := lsp.Serve(os.Stdin, os.Stdout, lsp.Options{
		SchemaDir: cf.schemaDir,
		DocType:   cf.docType,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "docc lsp:", err)
		return exitDiag
	}
	return 0
}

func cmdInit(args []string) int {
	// init has a flag set of its own for one reason above all: without one,
	// `docc init --help` took --help for the target directory and wrote a
	// starter project into a folder named "--help".
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "list the files that would be created, without writing any")
	if code, stop := parseFlags(fs, initHelp, args); stop {
		return code
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "docc init: expects at most one directory")
		return exitUsage
	}
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}

	if *dryRun {
		planned, err := starter.Plan(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return exitConfig
		}
		for _, path := range planned {
			fmt.Println(path)
		}
		fmt.Printf("\n%d files would be created in %s\n", len(planned), dir)
		return 0
	}

	if err := starter.Init(dir); err != nil {
		// Refusing to overwrite an existing starter is a fact about the project,
		// not about the command line.
		fmt.Fprintln(os.Stderr, "docc:", err)
		return exitConfig
	}
	fmt.Printf("created starter profile pack in %s\n", dir)
	return 0
}

func cmdTypes(args []string) int {
	fs := flag.NewFlagSet("types", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if code, stop := parseFlags(fs, typesHelp, args); stop {
		return code
	}

	start := "."
	if fs.NArg() > 0 {
		start = fs.Arg(0)
	}
	set, err := loadSchemas(cf.schemaDir, start)
	if err != nil {
		return fail(cf, exitConfig, err)
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
			return fail(cf, exitDiag, err)
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
	if code, stop := parseFlags(fs, themesHelp, args); stop {
		return code
	}
	start := "."
	if fs.NArg() > 0 {
		start = fs.Arg(0)
	}
	set, _, _, err := loadThemes(cf.themeDir, start)
	if err != nil {
		return fail(cf, exitConfig, err)
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
			return fail(cf, exitDiag, err)
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
	if code, stop := parseFlags(fs, buildHelp, args); stop {
		return code
	}
	if fs.NArg() != 1 {
		if fs.NArg() == 0 {
			return failf(cf, exitUsage, "docc build: no input file")
		}
		return failf(cf, exitUsage, "docc build: expects exactly one input file, got %d: %s",
			fs.NArg(), strings.Join(fs.Args(), " "))
	}
	if *to != "docx" && *to != "pdf" {
		return failf(cf, exitUsage, "docc build: unknown format %q — use docx or pdf", *to)
	}

	input := fs.Arg(0)
	src, err := os.ReadFile(input) //nolint:gosec // the user's own argument
	if err != nil {
		return fail(cf, exitUsage, err)
	}
	name := displayPath(input)

	schemas, err := loadSchemas(cf.schemaDir, input)
	if err != nil {
		return fail(cf, exitConfig, err)
	}
	themes, themeDir, resolvedProfile, err := loadThemes(cf.themeDir, input)
	if err != nil {
		return fail(cf, exitConfig, err)
	}

	f, parseDiags := parse.Parse(name, src)
	res := sema.Check(f, schemas, parseDiags, cf.docType)
	// Build-stage checks bind only here: `check` accepts a draft with blank
	// fields, but a blank that is not completed by hand must not reach a
	// rendered document.
	if res.Schema != nil {
		sema.CheckCompletion(f, res.Schema, &res.Diagnostics)
	}

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
			fmt.Printf("{\"ok\":false,\"kind\":\"diagnostics\",\"error\":\"validation failed\",\"type\":%q}\n", res.DocType)
		} else {
			fmt.Fprintln(os.Stderr, "\nrefusing to build — fix the errors above, or pass --force")
		}
		return exitDiag
	}
	emitDiags()
	if res.Schema == nil {
		return failf(cf, exitDiag, "no schema resolved; cannot build")
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

	// The schema-and-theme agreement check runs here rather than inside
	// emit.Build so its failure is reported as the configuration error it is,
	// instead of sharing an exit code with a genuine render failure.
	if err := emit.Validate(res.Schema, th); err != nil {
		return fail(cf, exitConfig, err)
	}

	doc := ir.Build(f, res.DocType, res.Meta.Values)
	built, err := emit.Build(doc, res.Schema, th, emit.Options{ThemeDir: themeDir, Provenance: resolvedProfile.Provenance()})
	if err != nil {
		return fail(cf, exitDiag, err)
	}

	outPath := *output
	if outPath == "" {
		outPath = strings.TrimSuffix(input, filepath.Ext(input)) + "." + *to
	}

	switch *to {
	case "docx":
		if err := writeAtomic(outPath, built.Write); err != nil {
			return fail(cf, exitDiag, err)
		}
	case "pdf":
		// The intermediate .docx is built in a private temp directory, never
		// derived from outPath. Deriving it — outPath with a .docx extension —
		// collides when the caller writes `--to pdf --output x.docx`, and the
		// cleanup step then deletes the file that was just produced.
		tmpDir, err := os.MkdirTemp("", "docc-build-")
		if err != nil {
			return fail(cf, exitDiag, err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		tmpDocx := filepath.Join(tmpDir, "doc.docx")
		if err := built.Write(tmpDocx); err != nil {
			return fail(cf, exitDiag, err)
		}
		if err := writeAtomic(outPath, func(dst string) error {
			return emit.ToPDF(tmpDocx, dst, emit.PDFOptions{Retries: 1})
		}); err != nil {
			return fail(cf, exitDiag, err)
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
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	cf.bindFrom(fs)
	if code, stop := parseFlags(fs, explainHelp, args); stop {
		return code
	}
	if fs.NArg() > 1 {
		return failf(cf, exitUsage, "docc explain: expects at most one code")
	}

	// No code lists the catalogue. It used to be a usage error, which left the
	// codes discoverable only by provoking them.
	if fs.NArg() == 0 {
		return explainAll(cf)
	}

	code := strings.ToUpper(fs.Arg(0))
	text, ok := explanations[code]
	if !ok {
		// Not an engine code. It may still be one a schema declares for a rule
		// it selects — `docc describe` prints those next to the DOC0xx ones and
		// they look identical, so landing here is the expected way to arrive at
		// a schema code, not a mistake worth a bare refusal.
		if explained, exit := explainSchemaCode(cf, code); explained {
			return exit
		}
		return failf(cf, exitUsage,
			"no explanation for %q\n  run `docc explain` for the full list, "+
				"or `docc explain %s --type <type>` if a schema declares it", code, code)
	}

	// --type turns the generic prose into the concrete contract: which fields
	// carry the pattern that failed, which values the enum permits. Without it
	// the explanation cannot say more than the schema-independent half.
	var detail []string
	if cf.docType != "" {
		sc, exit := schemaForType(cf, cf.docType)
		if sc == nil {
			return exit
		}
		detail = explainForSchema(code, sc)
	}

	if cf.jsonOut {
		out := struct {
			Code        string   `json:"code"`
			Explanation string   `json:"explanation"`
			Type        string   `json:"type,omitempty"`
			Detail      []string `json:"detail,omitempty"`
		}{Code: code, Explanation: text, Type: cf.docType, Detail: detail}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return fail(cf, exitDiag, err)
		}
		return 0
	}

	fmt.Printf("%s — %s\n", code, text)
	if cf.docType == "" {
		fmt.Printf("\nrun `docc explain %s --type <type>` for the constraints a schema declares\n", code)
		return 0
	}
	fmt.Printf("\nin %s:\n", cf.docType)
	if len(detail) == 0 {
		fmt.Printf("  nothing specific to report — run `docc describe %s` for the whole contract\n", cf.docType)
		return 0
	}
	for _, line := range detail {
		fmt.Printf("  %s\n", line)
	}
	return 0
}

// explainSchemaCode explains a code a schema declared for a rule it selects.
// Those codes never reach `explanations`: the engine does not own them, and the
// point of schema-owned codes is that a document type names its own diagnostics.
//
// Without this the trail went cold exactly where an author picks it up.
// `docc describe` prints "STA031  no_placeholder_text (error)" beside codes that
// look identical to the engine's, so `docc explain STA031` is the obvious next
// move, and it used to answer "no explanation" — leaving the check's meaning
// discoverable only by deliberately breaking a document to read the message.
//
// With --type it reads that type; without one it searches the project, so an
// author holding only a code from a diagnostic still gets an answer.
func explainSchemaCode(cf commonFlags, code string) (bool, int) {
	set, err := loadSchemas(cf.schemaDir, cf.start())
	if err != nil {
		return false, 0
	}

	types := set.Types()
	if cf.docType != "" {
		types = []string{cf.docType}
	}
	for _, name := range types {
		sc, err := set.Get(name)
		if err != nil {
			continue
		}
		for _, rule := range sc.Rules {
			if !strings.EqualFold(rule.ID, code) {
				continue
			}
			severity := rule.Severity
			if severity == "" {
				severity = "error"
			}
			if cf.jsonOut {
				out := struct {
					Code        string `json:"code"`
					Type        string `json:"type"`
					Check       string `json:"check"`
					Severity    string `json:"severity"`
					Explanation string `json:"explanation,omitempty"`
					Message     string `json:"message,omitempty"`
					Hint        string `json:"hint,omitempty"`
				}{code, name, rule.Check, severity, sema.DescribeCheck(rule.Check), rule.Message, rule.Hint}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					return true, fail(cf, exitDiag, err)
				}
				return true, 0
			}

			fmt.Printf("%s — declared by the document type %q, not by docc itself\n", code, name)
			fmt.Printf("\n  check:    %s (%s)\n", rule.Check, severity)
			if d := sema.DescribeCheck(rule.Check); d != "" {
				fmt.Printf("  reports:  %s\n", d)
			}
			if rule.Message != "" {
				fmt.Printf("  message:  %s\n", rule.Message)
			}
			if rule.Hint != "" {
				fmt.Printf("  hint:     %s\n", rule.Hint)
			}
			fmt.Printf("\nrun `docc describe %s` for the whole contract\n", name)
			return true, 0
		}
	}
	return false, 0
}

// explainAll lists every code docc emits.
func explainAll(cf commonFlags) int {
	codes := sortedKeys(explanations)
	if cf.jsonOut {
		type item struct {
			Code        string `json:"code"`
			Explanation string `json:"explanation"`
		}
		items := make([]item, 0, len(codes))
		for _, c := range codes {
			items = append(items, item{Code: c, Explanation: explanations[c]})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(struct {
			Codes []item `json:"codes"`
		}{Codes: items}); err != nil {
			return fail(cf, exitDiag, err)
		}
		return 0
	}
	for _, c := range codes {
		fmt.Printf("%-8s %s\n", c, firstSentence(explanations[c]))
	}
	fmt.Printf("\n%d codes. Run `docc explain <CODE>` for one in full.\n", len(codes))
	return 0
}

// firstSentence shortens an explanation for the catalogue listing.
func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
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
	"DOC009": "the schema itself is wrong: an unknown field type, an invalid pattern, an unregistered check, or a rule argument the check does not read.",
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
	"DOC029": "two documents checked together disagree about a value both of them state. The span types compared are the ones a schema's `spans_agree` rule watches, so this fires only where a type has said the occurrences must match — a Firma spelled two ways across a dossier, not two different parties. It is a warning because files named on one command line are not necessarily one transaction; `--strict` makes it bind, which is what a dossier being filed should run.",
	"DOC028": "an attribute block was found with no `[` on the same line. Spans are parsed a line at a time, so one wrapped across a line break is not a span at all — it becomes prose containing braces, and any check that looks for the annotation reports it as missing. Rewrap the line so the whole span, brackets and attributes together, sits on one of them.",
	"DOC030": "the document uses a `:::` block the schema does not declare. The hint lists the declared blocks; check `blocks:` in the schema.",
	"DOC031": "an inline span is missing its type class, or uses one the schema does not declare. The first `.class` in a span's attribute block is its type; the schema's `spans:` section lists the valid ones.",
	"DOC032": "a block with variants is missing its discriminator attribute, or names a variant the schema does not declare — for example a `partei` block without `kind=person` or `kind=company`.",
	"DOC033": "a block is missing a span its schema variant requires. Annotate the value inside the block with the named type: `[...]{.uid}`.",
	"DOC034": "a `#id` is used by more than one block. Ids must be unique in the document because references resolve against them; the diagnostic lists the other occurrence.",
	"DOC035": "a block carries an attribute its schema declaration does not permit. Only `#id`, the discriminator and the keys in the block's `attributes:` list are allowed.",
	"DOC036": "the schema declares `variants:` for a block but no `discriminator:`, so no document can satisfy it. This is a schema bug, not a document bug.",
	"DOC037": "a span's `ref=` names a block id that does not exist in the document. References resolve against `{#id}` attributes on blocks; the hint lists the ids that do exist.",
	"DOC038": "a field the schema requires does not appear in the document at all. A blank is content: write it visibly — `[____________]{.docc-field key=<name>}` — even when the value is not yet known.",
	"DOC039": "a field is still blank at build time but its completion is not `handwritten`. Fill in the value before building, or declare `completion: handwritten` in the schema if a human completes it on paper.",
	"DOC040": "a `.docc-field` span is missing its `key=`, or names a field the schema does not declare. The key ties the blank to its `fields:` entry.",
	"DOC041": "the schema declares a field with a completion stage the compiler does not know. Use `handwritten` or `before-execution`.",
	"DOC099": "a check the schema selected reported a problem but the schema gave the rule no `id:`. Add one to `rules:` so the diagnostic has a stable code.",
}

// explainForSchema answers the second half of an explanation: not what the code
// means, but what this document type actually requires. A pattern diagnostic is
// only useful next to the pattern.
//
// Codes with nothing type-specific to say return nil, and the caller points at
// `docc describe` instead of inventing detail.
func explainForSchema(code string, sc *schema.Schema) []string {
	var out []string
	switch code {
	case "DOC004", "DOC005", "DOC006", "DOC007":
		for _, name := range sortedFieldNames(sc.Frontmatter) {
			f := sc.Frontmatter[name]
			if !f.Required {
				continue
			}
			line := fmt.Sprintf("%s is a required %s", name, f.Type)
			if f.Nullable {
				line += ", nullable (an explicit ~ satisfies it)"
			}
			out = append(out, line)
		}
	case "DOC008":
		for _, name := range sortedFieldNames(sc.Frontmatter) {
			if f := sc.Frontmatter[name]; f.Type == "enum" {
				out = append(out, fmt.Sprintf("%s permits: %s", name, strings.Join(f.Values, ", ")))
			}
		}
	case "DOC010":
		for _, name := range sortedFieldNames(sc.Frontmatter) {
			f := sc.Frontmatter[name]
			if f.Pattern == "" {
				continue
			}
			line := fmt.Sprintf("%s must match %s", name, f.Pattern)
			if f.Hint != "" {
				line += " — " + f.Hint
			}
			out = append(out, line)
		}
	case "DOC011":
		out = append(out, "declared fields: "+strings.Join(sortedFieldNames(sc.Frontmatter), ", "))
	case "DOC020", "DOC021", "DOC022":
		out = describeHeadings(sc.Body, "")
	case "DOC030", "DOC032", "DOC033", "DOC035", "DOC036":
		for _, name := range sortedKeys(sc.Blocks) {
			b := describeBlock(name, sc.Blocks[name])
			out = append(out, b.Syntax)
		}
	case "DOC031", "DOC037":
		for _, name := range sortedKeys(sc.Spans) {
			out = append(out, fmt.Sprintf("[text]{.%s}  %s", name, sc.Spans[name].Description))
		}
	case "DOC038", "DOC039", "DOC040", "DOC041":
		for _, name := range sortedKeys(sc.Fields) {
			f := sc.Fields[name]
			completion := f.Completion
			if completion == "" {
				completion = "before-execution"
			}
			req := "optional"
			if f.Required {
				req = "required"
			}
			out = append(out, fmt.Sprintf("key=%s  %s, %s", name, req, completion))
		}
	}
	return out
}

// describeHeadings flattens the body outline into one line per heading, with the
// requirement that applies to it.
func describeHeadings(rules []schema.BodyRule, indent string) []string {
	var out []string
	for _, r := range rules {
		out = append(out, indent+strings.Repeat("#", r.Level)+" "+r.Heading+"  "+requirement(r.Required, r.RequiredWhen))
		out = append(out, describeHeadings(r.Children, indent+"  ")...)
	}
	return out
}

// resolveProfile returns the shared schema/theme source for one invocation.
// It never contacts the network: managed packs are installed and updated only
// by `docc profile` commands.
func resolveProfile(start string) (*profile.Resolved, error) {
	paths, err := profile.XDGPaths()
	if err != nil {
		return nil, err
	}
	return profile.Resolve(start, paths)
}

// loadSchemas resolves an explicit directory or the selected profile pack.
func loadSchemas(schemaDir, start string) (*schema.Set, error) {
	if schemaDir != "" {
		return schema.Load(schemaDir)
	}
	resolved, err := resolveProfile(start)
	if err != nil {
		return nil, err
	}
	return schema.Load(resolved.SchemaDir)
}

// loadSchemasCached is loadSchemas memoised across the files of one `docc check`
// run. The key is the explicit directory or the fully resolved profile schema
// directory, so documents sharing an immutable pack load it once.
func loadSchemasCached(cache map[string]*schema.Set, schemaDir, start string) (*schema.Set, error) {
	key := schemaDir
	if key == "" {
		resolved, err := resolveProfile(start)
		if err != nil {
			return nil, err
		}
		key = resolved.SchemaDir
	}
	if set, ok := cache[key]; ok {
		return set, nil
	}
	set, err := schema.Load(key)
	if err != nil {
		return nil, err
	}
	cache[key] = set
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

// loadThemes resolves an explicit directory or the selected profile pack. It
// returns the directory so theme-relative image paths can be found, and the
// resolution itself so a build can record what produced the file. An explicit
// --theme-dir has no profile to report, and the resolution is then nil.
func loadThemes(themeDir, start string) (*theme.Set, string, *profile.Resolved, error) {
	if themeDir != "" {
		set, err := theme.Load(themeDir)
		return set, themeDir, nil, err
	}
	resolved, err := resolveProfile(start)
	if err != nil {
		return nil, "", nil, err
	}
	set, err := theme.Load(resolved.ThemeDir)
	return set, resolved.ThemeDir, resolved, err
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
