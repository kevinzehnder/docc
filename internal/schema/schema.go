// Package schema defines the document-type contract: which frontmatter fields
// exist and how they are typed, which headings the body must contain, which
// named rules apply, and how markdown maps onto styles in the selected theme.
//
// Schemas are data, not Go code. Adding a document type is adding a YAML file
// to a project's .docc/schemas directory.
package schema

import "fmt"

// Schema is one document type.
type Schema struct {
	// Type is the identifier matched against frontmatter `document_type`.
	Type string `yaml:"type"`
	// Extends names another schema whose frontmatter, types and rules are
	// inherited. Fields declared here win.
	Extends     string `yaml:"extends"`
	Description string `yaml:"description"`

	// Theme names the visual definition in .docc/themes used to render this
	// type. Empty means the type is check-only and cannot be built.
	Theme string `yaml:"theme"`

	// Example is a compact, complete document of this type, shown by
	// `docc example`. It lives in the schema so the contract and its
	// demonstration cannot drift apart; a test checks it compiles clean.
	Example string `yaml:"example"`

	// Types declares reusable object shapes referenced by Field.Type.
	Types map[string]Fields `yaml:"types"`
	// Frontmatter declares the top-level metadata fields.
	Frontmatter Fields `yaml:"frontmatter"`
	// Body declares the required heading structure, in document order.
	Body []BodyRule `yaml:"body"`
	// Blocks declares the semantic `:::name` blocks this type permits. A
	// schema that declares any makes undeclared block names an error.
	Blocks map[string]BlockSpec `yaml:"blocks"`
	// Spans declares the inline `[text]{.type}` span types this type permits.
	// A schema that declares any makes untyped or undeclared spans an error.
	// Types prefixed `docc-` are reserved for the compiler and need no
	// declaration.
	Spans map[string]SpanSpec `yaml:"spans"`
	// Fields declares the intentionally incomplete fields of the type: blanks
	// that are content, written `[____]{.docc-field key=<name>}`. The map key
	// is the span key.
	Fields map[string]FieldSpec `yaml:"fields"`
	// Styles maps markdown constructs to style ids defined by the theme.
	Styles map[string]string `yaml:"styles"`
	// Rules lists named cross-cutting checks to run.
	Rules []Rule `yaml:"rules"`
	// Render configures numbering applied to the body at render time.
	Render Render `yaml:"render"`
}

// Render is the document type's opt-in to numbering that the source markdown
// does not express: an outline over the headings, and a marginal number on each
// paragraph of prose.
//
// It lives in the schema rather than the theme because it is a fact about the
// document type — a brief has numbered sections and marginal numbers, a letter
// has neither. What those numbers look like is the theme's business, which is
// why each rule names a definition rather than describing one.
type Render struct {
	// HeadingNumbering numbers headings by their markdown level: level 1 takes
	// the definition's level 0, and so on.
	HeadingNumbering *NumberingRule `yaml:"heading_numbering"`
	// ParagraphNumbering numbers top-level paragraphs of prose at level 0,
	// continuously, across the headings between them.
	ParagraphNumbering *NumberingRule `yaml:"paragraph_numbering"`
	// PageBreakBeforeHeadings names headings that must begin a new page. It is
	// here for the same reason numbering is: where a document type breaks is a
	// fact about the type, not something the markdown should carry. A deed's
	// certification follows the signatures on a fresh sheet whatever the
	// signatures happen to fill.
	PageBreakBeforeHeadings []string `yaml:"page_break_before_headings"`
}

// NumberingRule selects a list definition from the theme and says where in the
// body it starts applying.
type NumberingRule struct {
	// Definition names an entry in the theme's `numbering:` map.
	Definition string `yaml:"definition"`
	// StartAtHeading names the heading that is itself the first numbered block.
	StartAtHeading string `yaml:"start_at_heading"`
	// StartAfterHeading names a heading that is not numbered but after which
	// numbering begins. This is what a marginal number wants: the count starts
	// with the prose, not with the heading above it.
	//
	// Set neither and numbering applies to the whole body. Setting both is a
	// schema error rather than a precedence rule nobody would remember.
	StartAfterHeading string `yaml:"start_after_heading"`
}

// Marker returns the heading text numbering keys off, and whether that heading
// is itself numbered.
func (r *NumberingRule) Marker() (heading string, inclusive bool) {
	if r == nil {
		return "", false
	}
	if r.StartAtHeading != "" {
		return r.StartAtHeading, true
	}
	return r.StartAfterHeading, false
}

// BlockSpec declares one semantic block kind. The zero value is a valid
// declaration: the block may appear, carries no required structure, and
// permits no attributes beyond `#id`.
type BlockSpec struct {
	Description string `yaml:"description"`
	// Attributes lists the permitted attribute keys, beyond `#id` and the
	// discriminator, which are always permitted.
	Attributes []string `yaml:"attributes"`
	// Discriminator names the attribute whose value selects one of Variants —
	// `kind` for a party block with person and company shapes.
	Discriminator string `yaml:"discriminator"`
	// Variants maps discriminator values to their structural requirements.
	Variants map[string]BlockVariant `yaml:"variants"`
	// RequiredSpans lists span types that must appear inside the block, for
	// blocks without variants.
	RequiredSpans []string `yaml:"required_spans"`
}

// BlockVariant is the structural requirement of one discriminator value.
type BlockVariant struct {
	RequiredSpans []string `yaml:"required_spans"`
}

// SpanSpec declares one inline span type.
type SpanSpec struct {
	Description string `yaml:"description"`
}

// FieldSpec declares one intentionally incomplete field. A blank is content,
// not missing content: it must appear visibly in the document, annotated as a
// `.docc-field` span, and the spec says when it stops being allowed to be
// blank.
type FieldSpec struct {
	Description string `yaml:"description"`
	// Required makes the field's absence — no span with its key at all — an
	// error at check time. A present-but-blank field satisfies Required.
	Required bool `yaml:"required"`
	// Completion says when the blank must be filled. "before-execution", the
	// default, blocks `build` while the field is blank. "handwritten" lets the
	// blank survive into the rendered document, because a human completes it
	// on paper.
	Completion string `yaml:"completion"`
}

// Fields is a set of named field declarations.
type Fields map[string]Field

// Field declares one frontmatter field.
type Field struct {
	// Type is a builtin (string, int, bool, date, enum, any), a list of one of
	// those written `list<T>`, or the name of an entry in Schema.Types.
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
	// Nullable permits an explicit YAML null (`~`) for a required field. Use it
	// for fields that are meaningfully "known to be absent", such as an
	// opposing party with no legal representative.
	Nullable bool `yaml:"nullable"`
	// Values enumerates the permitted values when Type is enum.
	Values []string `yaml:"values"`
	// Pattern is a Go regexp the value must match, for string-typed fields.
	Pattern string `yaml:"pattern"`
	// Hint is shown verbatim in diagnostics. Write it as an instruction.
	Hint    string `yaml:"hint"`
	Default any    `yaml:"default"`
}

// BodyRule declares one expected heading and, recursively, its subsections.
type BodyRule struct {
	// Heading is the exact heading text. Matching is case-insensitive and
	// ignores surrounding whitespace.
	Heading string `yaml:"heading"`
	Level   int    `yaml:"level"`
	// Required makes a missing heading an error rather than a warning.
	Required bool `yaml:"required"`
	// RequiredWhen makes the requirement conditional on a frontmatter field,
	// written `field == "value"`.
	RequiredWhen string `yaml:"required_when"`
	// Ordered, when true on a rule, means the child headings must appear in the
	// declared order rather than merely being present.
	Ordered  bool       `yaml:"ordered"`
	Children []BodyRule `yaml:"children"`
}

// Rule invokes a named check implemented in Go. Declarative field constraints
// cover most of the contract; the remainder — cross-references between
// frontmatter and body, placeholder detection — needs real code, so schemas
// select those by name rather than expressing them inline.
type Rule struct {
	// ID is the diagnostic code emitted, e.g. "LEG012".
	ID string `yaml:"id"`
	// Check names a registered check. See package sema.
	Check string `yaml:"check"`
	// Severity is "error" or "warning". Defaults to error.
	Severity string `yaml:"severity"`
	// Message overrides the check's default message.
	Message string `yaml:"message"`
	// Hint overrides the check's default hint.
	Hint string `yaml:"hint"`
	// Args passes check-specific configuration.
	Args map[string]any `yaml:"args"`
}

// Set is a resolved collection of schemas keyed by type.
type Set struct {
	byType map[string]*Schema
	// Root is the directory schemas were loaded from, retained for callers that
	// need the configuration location.
	Root string
}

// Get returns the schema for a document type.
func (s *Set) Get(docType string) (*Schema, error) {
	if s == nil || s.byType == nil {
		return nil, fmt.Errorf("no schemas loaded")
	}
	sc, ok := s.byType[docType]
	if !ok {
		return nil, fmt.Errorf("unknown document type %q (known types: %s)", docType, joinKeys(s.byType))
	}
	return sc, nil
}

// Types lists the known document types in sorted order.
func (s *Set) Types() []string {
	if s == nil {
		return nil
	}
	return sortedKeys(s.byType)
}
