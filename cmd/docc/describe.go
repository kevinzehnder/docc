package main

// `docc describe` and `docc example` are the discovery half of the agent
// interface: describe reports a document type's whole contract as data, and
// example prints a compact document that satisfies it. Both are generated
// from the resolved schema, so there is no prose copy of the contract that
// could drift from what the compiler actually checks.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kevinzehnder/docc/internal/schema"
)

// describedType is the JSON shape of one document type's contract.
type describedType struct {
	Type        string           `json:"type"`
	Description string           `json:"description,omitempty"`
	Theme       string           `json:"theme,omitempty"`
	Frontmatter []describedField `json:"frontmatter"`
	Body        []describedRule  `json:"body,omitempty"`
	Blocks      []describedBlock `json:"blocks,omitempty"`
	Spans       []describedSpan  `json:"spans,omitempty"`
	Rules       []describedCheck `json:"rules,omitempty"`
	// HasExample reports whether `docc example <type>` will print a document.
	HasExample bool `json:"has_example"`
}

type describedField struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Nullable bool     `json:"nullable,omitempty"`
	Values   []string `json:"values,omitempty"`
	Pattern  string   `json:"pattern,omitempty"`
	Default  any      `json:"default,omitempty"`
	Hint     string   `json:"hint,omitempty"`
}

type describedRule struct {
	Heading  string          `json:"heading"`
	Level    int             `json:"level"`
	Required bool            `json:"required"`
	Children []describedRule `json:"children,omitempty"`
}

type describedBlock struct {
	Name          string             `json:"name"`
	Description   string             `json:"description,omitempty"`
	Attributes    []string           `json:"attributes,omitempty"`
	Discriminator string             `json:"discriminator,omitempty"`
	Variants      []describedVariant `json:"variants,omitempty"`
	RequiredSpans []string           `json:"required_spans,omitempty"`
	// Syntax is one valid opening line to imitate.
	Syntax string `json:"syntax"`
}

type describedVariant struct {
	Name          string   `json:"name"`
	RequiredSpans []string `json:"required_spans,omitempty"`
}

type describedSpan struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Syntax is one valid span to imitate.
	Syntax string `json:"syntax"`
}

type describedCheck struct {
	ID       string `json:"id"`
	Check    string `json:"check"`
	Severity string `json:"severity"`
}

func cmdDescribe(args []string) int {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: docc describe [flags] <type>")
		return 2
	}
	sc, code := schemaForType(cf, fs.Arg(0))
	if sc == nil {
		return code
	}

	d := describe(sc)
	if cf.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(d); err != nil {
			fmt.Fprintln(os.Stderr, "docc:", err)
			return 2
		}
		return 0
	}
	printDescribed(d)
	return 0
}

func cmdExample(args []string) int {
	fs := flag.NewFlagSet("example", flag.ContinueOnError)
	var cf commonFlags
	cf.bind(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: docc example [flags] <type>")
		return 2
	}
	sc, code := schemaForType(cf, fs.Arg(0))
	if sc == nil {
		return code
	}
	if sc.Example == "" {
		fmt.Fprintf(os.Stderr, "docc: schema %q declares no example\n  add an `example: |` document to the schema file\n", sc.Type)
		return 1
	}
	fmt.Print(sc.Example)
	if !strings.HasSuffix(sc.Example, "\n") {
		fmt.Println()
	}
	return 0
}

// schemaForType loads the nearest schemas and resolves one type, reporting
// errors itself. A nil schema is accompanied by the exit code to return.
func schemaForType(cf commonFlags, docType string) (*schema.Schema, int) {
	set, err := loadSchemas(cf.schemaDir, ".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return nil, 2
	}
	sc, err := set.Get(docType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docc:", err)
		return nil, 2
	}
	return sc, 0
}

func describe(sc *schema.Schema) describedType {
	d := describedType{
		Type:        sc.Type,
		Description: sc.Description,
		Theme:       sc.Theme,
		HasExample:  sc.Example != "",
	}
	for _, name := range sortedFieldNames(sc.Frontmatter) {
		f := sc.Frontmatter[name]
		d.Frontmatter = append(d.Frontmatter, describedField{
			Name: name, Type: f.Type, Required: f.Required, Nullable: f.Nullable,
			Values: f.Values, Pattern: f.Pattern, Default: f.Default, Hint: f.Hint,
		})
	}
	d.Body = describeBody(sc.Body)
	for _, name := range sortedKeys(sc.Blocks) {
		d.Blocks = append(d.Blocks, describeBlock(name, sc.Blocks[name]))
	}
	for _, name := range sortedKeys(sc.Spans) {
		s := sc.Spans[name]
		d.Spans = append(d.Spans, describedSpan{
			Name: name, Description: s.Description,
			Syntax: fmt.Sprintf("[literal text]{.%s key=<key>}", name),
		})
	}
	for _, r := range sc.Rules {
		sev := r.Severity
		if sev == "" {
			sev = "error"
		}
		d.Rules = append(d.Rules, describedCheck{ID: r.ID, Check: r.Check, Severity: sev})
	}
	return d
}

func describeBody(rules []schema.BodyRule) []describedRule {
	out := make([]describedRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, describedRule{
			Heading: r.Heading, Level: r.Level, Required: r.Required,
			Children: describeBody(r.Children),
		})
	}
	return out
}

func describeBlock(name string, spec schema.BlockSpec) describedBlock {
	b := describedBlock{
		Name:          name,
		Description:   spec.Description,
		Attributes:    spec.Attributes,
		Discriminator: spec.Discriminator,
		RequiredSpans: spec.RequiredSpans,
		Syntax:        fmt.Sprintf("::: %s", name),
	}
	for _, v := range sortedKeys(spec.Variants) {
		b.Variants = append(b.Variants, describedVariant{
			Name: v, RequiredSpans: spec.Variants[v].RequiredSpans,
		})
	}
	if spec.Discriminator != "" && len(b.Variants) > 0 {
		b.Syntax = fmt.Sprintf("::: %s {#<id> %s=%s}", name, spec.Discriminator, b.Variants[0].Name)
	}
	return b
}

// printDescribed renders the contract for humans. Agents use --json.
func printDescribed(d describedType) {
	fmt.Printf("%s — %s\n", d.Type, d.Description)
	if d.Theme != "" {
		fmt.Printf("theme: %s\n", d.Theme)
	}

	fmt.Println("\nfrontmatter:")
	for _, f := range d.Frontmatter {
		req := ""
		if f.Required {
			req = "  (required)"
		}
		fmt.Printf("  %-22s %s%s\n", f.Name, f.Type, req)
	}

	if len(d.Body) > 0 {
		fmt.Println("\nbody:")
		printBodyRules(d.Body, "  ")
	}
	if len(d.Blocks) > 0 {
		fmt.Println("\nblocks:")
		for _, b := range d.Blocks {
			fmt.Printf("  %s\n", b.Syntax)
			for _, v := range b.Variants {
				fmt.Printf("    %s=%s requires: .%s\n", b.Discriminator, v.Name,
					strings.Join(v.RequiredSpans, ", ."))
			}
			if len(b.RequiredSpans) > 0 {
				fmt.Printf("    requires: .%s\n", strings.Join(b.RequiredSpans, ", ."))
			}
		}
	}
	if len(d.Spans) > 0 {
		fmt.Println("\nspans:")
		for _, s := range d.Spans {
			fmt.Printf("  %s\n", s.Syntax)
		}
	}
	if len(d.Rules) > 0 {
		fmt.Println("\nrules:")
		for _, r := range d.Rules {
			fmt.Printf("  %-8s %s (%s)\n", r.ID, r.Check, r.Severity)
		}
	}
	if d.HasExample {
		fmt.Printf("\nrun `docc example %s` for a complete valid document\n", d.Type)
	}
}

func printBodyRules(rules []describedRule, indent string) {
	for _, r := range rules {
		req := ""
		if r.Required {
			req = "  (required)"
		}
		fmt.Printf("%s%s %s%s\n", indent, strings.Repeat("#", r.Level), r.Heading, req)
		printBodyRules(r.Children, indent+"  ")
	}
}

func sortedFieldNames(f schema.Fields) []string {
	out := make([]string, 0, len(f))
	for k := range f {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
