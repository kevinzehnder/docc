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
	"slices"
	"sort"
	"strings"

	"github.com/kevinzehnder/docc/internal/emit"
	"github.com/kevinzehnder/docc/internal/schema"
)

// describedType is the JSON shape of one document type's contract.
type describedType struct {
	Type        string           `json:"type"`
	Description string           `json:"description,omitempty"`
	Extends     string           `json:"extends,omitempty"`
	Theme       string           `json:"theme,omitempty"`
	Frontmatter []describedField `json:"frontmatter"`
	Body        []describedRule  `json:"body,omitempty"`
	Blocks      []describedBlock `json:"blocks,omitempty"`
	Spans       []describedSpan  `json:"spans,omitempty"`
	// Blanks are the intentionally incomplete fields — the `.docc-field` spans
	// a document of this type writes as visible blanks.
	Blanks []describedBlank `json:"blanks,omitempty"`
	// Styles is the markdown-construct-to-theme-style map: what the schema sets,
	// what it could set, and which constructs are formatted regardless.
	Styles describedStyles  `json:"styles"`
	Rules  []describedCheck `json:"rules,omitempty"`
	// HasExample reports whether `docc example <type>` will print a document.
	HasExample bool `json:"has_example"`
	// FieldMap reports whether the theme was available to compute the `rendered`
	// annotations. When it is false, an empty `rendered` says nothing — the
	// theme was simply not consulted.
	FieldMap bool `json:"field_map"`
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
	// Members expands an object type declared in the schema's `types:` — the
	// contract is not complete without them, because `sender` alone says
	// nothing about the fields sema actually validates underneath it.
	Members []describedField `json:"members,omitempty"`
	// Rendered names the places the type's theme interpolates this field:
	// "prologue", "epilogue", "header:default", "footer:first". Empty means the
	// theme never prints it, which is a fact an author otherwise learns only by
	// opening the output.
	Rendered []string `json:"rendered,omitempty"`
}

type describedRule struct {
	Heading  string `json:"heading"`
	Level    int    `json:"level"`
	Required bool   `json:"required"`
	// RequiredWhen is the frontmatter condition that makes an otherwise optional
	// heading mandatory, e.g. `legal_doc_type == "Klageschrift"`.
	RequiredWhen string `json:"required_when,omitempty"`
	// Ordered means the children must appear in the order declared here.
	Ordered  bool            `json:"ordered,omitempty"`
	Children []describedRule `json:"children,omitempty"`
}

// describedStyles reports how markdown reaches the page.
type describedStyles struct {
	// Mapped are the constructs this schema styles, in effect.
	Mapped []describedStyleKey `json:"mapped,omitempty"`
	// Available are the keys the schema could set but does not.
	Available []describedStyleKey `json:"available,omitempty"`
	// Unread are keys the schema sets that nothing reads — they render exactly
	// as if absent.
	Unread []string `json:"unread,omitempty"`
	// Fixed are constructs the emitter formats itself; no theme can change them.
	Fixed []describedFixed `json:"fixed,omitempty"`
}

type describedStyleKey struct {
	Key     string `json:"key"`
	Style   string `json:"style,omitempty"`
	Purpose string `json:"purpose"`
	// Fallback is the key used when this one is unset.
	Fallback string `json:"fallback,omitempty"`
}

type describedFixed struct {
	Construct  string `json:"construct"`
	Formatting string `json:"formatting"`
}

// describedBlank is one entry of the schema's `fields:` — a blank that is
// content rather than missing content.
type describedBlank struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	// Completion is "before-execution" (must be filled before build) or
	// "handwritten" (a human completes it on paper).
	Completion string `json:"completion"`
	Syntax     string `json:"syntax"`
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
	// Pattern is how the block renders — "plain", "labelled", "amount" or
	// "ruled" — selected by which `div.<name>.*` style the schema maps. It is
	// not declared anywhere in the block's own definition, which is exactly why
	// it is worth reporting.
	Pattern string `json:"pattern,omitempty"`
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
	cf.bindFrom(fs)
	if code, stop := parseFlags(fs, describeHelp, args); stop {
		return code
	}
	if fs.NArg() != 1 {
		return failf(cf, exitUsage, "usage: docc describe [flags] <type>")
	}
	sc, code := schemaForType(cf, fs.Arg(0))
	if sc == nil {
		return code
	}

	d := describe(sc, renderedFields(cf, sc))
	if cf.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(d); err != nil {
			return fail(cf, exitDiag, err)
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
	cf.bindFrom(fs)
	if code, stop := parseFlags(fs, exampleHelp, args); stop {
		return code
	}
	if fs.NArg() != 1 {
		return failf(cf, exitUsage, "usage: docc example [flags] <type>")
	}
	sc, code := schemaForType(cf, fs.Arg(0))
	if sc == nil {
		return code
	}
	if sc.Example == "" {
		return failf(cf, exitConfig,
			"schema %q declares no example\n  add an `example: |` document to the schema file", sc.Type)
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
	set, err := loadSchemas(cf.schemaDir, cf.start())
	if err != nil {
		return nil, fail(cf, exitConfig, err)
	}
	sc, err := set.Get(docType)
	if err != nil {
		return nil, fail(cf, exitConfig, err)
	}
	return sc, 0
}

// describe reports a type's whole contract. rendered maps a dotted field path
// to the places the theme prints it; a nil map means the theme could not be
// consulted, and the rendering annotation is left off entirely rather than
// wrongly claiming nothing is rendered.
func describe(sc *schema.Schema, rendered map[string][]string) describedType {
	d := describedType{
		Type:        sc.Type,
		Description: sc.Description,
		Extends:     sc.Extends,
		Theme:       sc.Theme,
		HasExample:  sc.Example != "",
		FieldMap:    rendered != nil,
	}
	d.Frontmatter = describeFields(sc, sc.Frontmatter, "", rendered, nil)
	d.Body = describeBody(sc.Body)
	for _, name := range sortedKeys(sc.Blocks) {
		b := describeBlock(name, sc.Blocks[name])
		b.Pattern = emit.BlockPattern(sc, name)
		d.Blocks = append(d.Blocks, b)
	}
	for _, name := range sortedKeys(sc.Spans) {
		s := sc.Spans[name]
		d.Spans = append(d.Spans, describedSpan{
			Name: name, Description: s.Description,
			Syntax: fmt.Sprintf("[literal text]{.%s key=<key>}", name),
		})
	}
	for _, name := range sortedKeys(sc.Fields) {
		f := sc.Fields[name]
		completion := f.Completion
		if completion == "" {
			completion = "before-execution"
		}
		d.Blanks = append(d.Blanks, describedBlank{
			Name: name, Description: f.Description, Required: f.Required,
			Completion: completion,
			Syntax:     fmt.Sprintf("[____________]{.docc-field key=%s}", name),
		})
	}
	d.Styles = describeStyles(sc)
	for _, r := range sc.Rules {
		sev := r.Severity
		if sev == "" {
			sev = "error"
		}
		d.Rules = append(d.Rules, describedCheck{ID: r.ID, Check: r.Check, Severity: sev})
	}
	return d
}

// describeStyles reports how markdown reaches the page: which constructs this
// schema styles, which it could, which mappings do nothing, and which
// constructs no theme can reach at all.
//
// The last two are the ones worth printing. A mapping nothing reads renders as
// if absent, and a construct with fixed formatting sends an author looking for
// a theme bug that is not there.
func describeStyles(sc *schema.Schema) describedStyles {
	var out describedStyles
	for _, k := range emit.StyleKeys(sc) {
		entry := describedStyleKey{Key: k.Key, Purpose: k.Purpose, Fallback: k.Fallback}
		if id := sc.Styles[k.Key]; id != "" {
			entry.Style = id
			out.Mapped = append(out.Mapped, entry)
			continue
		}
		out.Available = append(out.Available, entry)
	}
	out.Unread = emit.UnreadStyleKeys(sc)
	for _, f := range emit.FixedFormatting() {
		out.Fixed = append(out.Fixed, describedFixed{Construct: f.Construct, Formatting: f.Formatting})
	}
	return out
}

// describeFields expands one level of fields, recursing into the object shapes
// declared in the schema's `types:`. seen breaks a type that refers to itself,
// directly or through another; without it a self-referential shape would
// recurse until the stack ran out.
func describeFields(sc *schema.Schema, fields schema.Fields, prefix string, rendered map[string][]string, seen []string) []describedField {
	out := make([]describedField, 0, len(fields))
	for _, name := range sortedFieldNames(fields) {
		f := fields[name]
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		df := describedField{
			Name: name, Type: f.Type, Required: f.Required, Nullable: f.Nullable,
			Values: f.Values, Pattern: f.Pattern, Default: f.Default, Hint: f.Hint,
			Rendered: rendered[path],
		}

		// A list is described by its element: `list<party>` expands the party
		// shape, because that is what an author has to fill in.
		elem := strings.TrimSuffix(strings.TrimPrefix(f.Type, "list<"), ">")
		if members, ok := sc.Types[elem]; ok && !slices.Contains(seen, elem) {
			df.Members = describeFields(sc, members, path, rendered, append(seen, elem))
		}
		out = append(out, df)
	}
	return out
}

func describeBody(rules []schema.BodyRule) []describedRule {
	out := make([]describedRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, describedRule{
			Heading: r.Heading, Level: r.Level, Required: r.Required,
			RequiredWhen: r.RequiredWhen, Ordered: r.Ordered,
			Children: describeBody(r.Children),
		})
	}
	return out
}

// renderedFields maps each frontmatter path the type's theme interpolates to
// the places it appears. It is best-effort: a project with no themes, or a
// check-only type, simply gets no annotation, because `describe` must keep
// working where `build` cannot.
func renderedFields(cf commonFlags, sc *schema.Schema) map[string][]string {
	if sc.Theme == "" {
		return nil
	}
	set, _, err := loadThemes(cf.themeDir, cf.start())
	if err != nil {
		return nil
	}
	th, err := set.Get(sc.Theme)
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for _, ref := range th.FieldRefs() {
		out[ref.Path] = append(out[ref.Path], ref.Region)
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
//
// Everything the JSON carries is printed here too. The two used to disagree —
// nullability, patterns, enum values and defaults were computed and then thrown
// away on the human path, so a reader could not tell why an example passed `~`
// for a required field.
func printDescribed(d describedType) {
	fmt.Printf("%s — %s\n", d.Type, d.Description)
	if d.Extends != "" {
		fmt.Printf("extends: %s\n", d.Extends)
	}
	if d.Theme != "" {
		fmt.Printf("theme: %s\n", d.Theme)
	}

	fmt.Println("\nfrontmatter:")
	printFields(d.Frontmatter, "  ", d.FieldMap)

	if len(d.Body) > 0 {
		fmt.Println("\nbody:")
		printBodyRules(d.Body, "  ")
	}
	if len(d.Blocks) > 0 {
		fmt.Println("\nblocks:")
		for _, b := range d.Blocks {
			fmt.Printf("  %s\n", b.Syntax)
			if b.Pattern != "" {
				fmt.Printf("    renders: %s\n", b.Pattern)
			}
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
	if len(d.Blanks) > 0 {
		fmt.Println("\nblanks (write them visibly, even when the value is unknown):")
		for _, b := range d.Blanks {
			req := "optional"
			if b.Required {
				req = "required"
			}
			fmt.Printf("  %s\n    %s, completed %s", b.Syntax, req, b.Completion)
			if b.Description != "" {
				fmt.Printf(" — %s", b.Description)
			}
			fmt.Println()
		}
	}
	printStyles(d.Styles)

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

// printStyles prints how markdown reaches the page. The unread and fixed
// sections are the reason this exists: both are otherwise silent.
func printStyles(s describedStyles) {
	if len(s.Mapped) > 0 {
		fmt.Println("\nstyles (markdown construct → theme style):")
		w := 0
		for _, k := range s.Mapped {
			w = max(w, len(k.Key))
		}
		for _, k := range s.Mapped {
			fmt.Printf("  %-*s → %-20s %s\n", w, k.Key, k.Style, k.Purpose)
		}
	}
	if len(s.Available) > 0 {
		keys := make([]string, 0, len(s.Available))
		for _, k := range s.Available {
			keys = append(keys, k.Key)
		}
		fmt.Printf("\n  unmapped keys this type could set: %s\n", strings.Join(keys, ", "))
	}
	if len(s.Unread) > 0 {
		fmt.Println("\n  these mappings are never read — they render as if absent:")
		for _, u := range s.Unread {
			fmt.Printf("    %s\n", u)
		}
	}
	if len(s.Fixed) > 0 {
		fmt.Println("\nfixed formatting (no theme can change these):")
		w := 0
		for _, f := range s.Fixed {
			w = max(w, len(f.Construct))
		}
		for _, f := range s.Fixed {
			fmt.Printf("  %-*s  %s\n", w, f.Construct, f.Formatting)
		}
	}
}

// printFields prints one level of the frontmatter contract, recursing into
// expanded object members.
func printFields(fields []describedField, indent string, fieldMap bool) {
	for _, f := range fields {
		fmt.Printf("%s%-22s %s  %s\n", indent, f.Name, f.Type, requirement(f.Required, ""))
		for _, line := range fieldDetails(f, fieldMap) {
			fmt.Printf("%s  %s\n", indent, line)
		}
		printFields(f.Members, indent+"  ", fieldMap)
	}
}

// fieldDetails is the constraint half of a field's contract — everything an
// author needs in order to write a value that passes.
func fieldDetails(f describedField, fieldMap bool) []string {
	var out []string
	if f.Nullable {
		out = append(out, "nullable: an explicit ~ satisfies the requirement")
	}
	if len(f.Values) > 0 {
		out = append(out, "one of: "+strings.Join(f.Values, ", "))
	}
	if f.Pattern != "" {
		out = append(out, "pattern: "+f.Pattern)
	}
	if f.Default != nil {
		out = append(out, fmt.Sprintf("default: %v", f.Default))
	}
	if f.Hint != "" {
		out = append(out, f.Hint)
	}
	switch {
	case len(f.Rendered) > 0:
		out = append(out, "rendered in: "+strings.Join(f.Rendered, ", "))
	case fieldMap && len(f.Members) == 0:
		// The theme was consulted and never names this field. Saying so is the
		// point: a required `title` that appears nowhere in the output is a fact
		// an author otherwise discovers by opening the .docx.
		out = append(out, "not rendered by the theme — metadata only")
	}
	return out
}

func printBodyRules(rules []describedRule, indent string) {
	for _, r := range rules {
		fmt.Printf("%s%s %s  %s\n", indent, strings.Repeat("#", r.Level), r.Heading,
			requirement(r.Required, r.RequiredWhen))
		if r.Ordered && len(r.Children) > 0 {
			fmt.Printf("%s  (children must appear in this order)\n", indent)
		}
		printBodyRules(r.Children, indent+"  ")
	}
}

// requirement states in words whether something must be present. An unlabelled
// heading used to mean "optional", which readers had no way to know, and a
// conditional requirement was not reported at all.
func requirement(required bool, requiredWhen string) string {
	switch {
	case required:
		return "(required)"
	case requiredWhen != "":
		return "(required if " + requiredWhen + ")"
	default:
		return "(optional)"
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
