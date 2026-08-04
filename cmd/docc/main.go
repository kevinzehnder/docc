// Command docc compiles structured markdown documents: it checks them against
// a schema and (once the emitter lands) builds them into Word documents.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/project"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
)

// buildVersion is stamped by the build via -ldflags.
var buildVersion = "dev"

const usage = `docc — a compiler for structured documents

usage:
  docc check [flags] <file.md>...   validate documents against their schema
  docc types [flags]                list known document types
  docc explain <CODE>               describe a diagnostic code
  docc version

flags:
  --schema-dir <dir>   schema directory (default: nearest .docc/schemas)
  --type <type>        override the frontmatter document_type
  --json               machine-readable output
  --strict             treat warnings as errors
  --no-color           disable coloured output

exit codes:
  0  no errors
  1  diagnostics reported
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
	case "types":
		return cmdTypes(rest)
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
	docType   string
	jsonOut   bool
	strict    bool
	noColor   bool
}

func (c *commonFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&c.schemaDir, "schema-dir", "", "schema directory (default: nearest .docc/schemas)")
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
			fmt.Printf("{\"type\":%q,\"description\":%q,\"template\":%q}\n", sc.Type, sc.Description, sc.Template)
			continue
		}
		fmt.Printf("%-14s %s\n", sc.Type, sc.Description)
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
	"DOC001": "the file has no YAML frontmatter. Every document starts with a `---` delimited block declaring at least `document_type`.",
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
