package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
)

// watchedSpans returns the span types the packaged types require to agree, as
// the union over them. It decides whether the cross-document section applies at
// all: a profile whose types never declare `spans_agree` has nothing to compare
// across files, and explaining DOC029 to that reader is noise. Derived from the
// schemas for the same reason the type table is — a profile that adds such a
// rule gets the instructions for it without anyone remembering to write them.
func watchedSpans(schemas *schema.Set, types []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, typ := range types {
		sc, err := schemas.Get(typ)
		if err != nil {
			continue
		}
		for _, span := range sema.WatchedSpanTypes(sc) {
			if seen[span] {
				continue
			}
			seen[span] = true
			out = append(out, span)
		}
	}
	sort.Strings(out)
	return out
}

// skillDoc generates the skill's instructions. It is generated rather than
// carried alongside the profile so that the types it documents are the types
// the profile actually declares — a hand-maintained file drifts, and the drift
// is only discovered by whoever tries to use it.
func skillDoc(name string, schemas *schema.Set, r *SkillResult, notes []byte) []byte {
	var b strings.Builder
	docc := "docc"
	if r.Binary != "" {
		docc = "\"./$DOCC\""
	}

	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", name)
	fmt.Fprintf(&b, "description: %s\n", skillDescription(name, r))
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s — schema-backed document compiler\n\n", name)
	b.WriteString("`docc` compiles Markdown with YAML frontmatter into a Word `.docx`.\n" +
		"The schema is the contract; the theme is the layout. This skill carries the\n" +
		"document types and layouts, so the output matches the house style exactly.\n\n" +
		"All paths below are relative to **this skill's directory**. Run commands from\n" +
		"that directory, or prefix each path with the skill directory's absolute path.\n\n")

	if r.Binary != "" {
		b.WriteString("## First run — enable the bundled binary\n\n" +
			"The compiler ships inside this skill; there is no install and no network.\n" +
			"Packaging may drop the exec bit, so restore it once:\n\n")
		fmt.Fprintf(&b, "```sh\nDOCC=bin/%s\nchmod +x \"$DOCC\"\n\"./$DOCC\" version\n```\n\n", r.Binary)
		b.WriteString("A version line means the compiler runs here. `sh probe.sh` does the same\n" +
			"check plus a real build, and prints `PROBE RESULT: PASS` when the skill is\n" +
			"fully operational.\n\n")
	} else {
		b.WriteString("## First run — confirm the compiler\n\n" +
			"This skill carries configuration only; `docc` must already be on PATH.\n\n" +
			"```sh\ndocc version\n```\n\n" +
			"If that fails, docc is not installed in this environment and nothing below\n" +
			"will work. `sh probe.sh` checks it and runs a real build.\n\n")
	}

	fmt.Fprintf(&b, "## Point docc at this profile\n\n"+
		"This skill is a profile pack: its `%s` names the schemas and themes below.\n"+
		"Set `%s` to **this skill's absolute directory** and every command finds them,\n"+
		"whatever directory the document lives in:\n\n", manifestName, EnvProfile)
	fmt.Fprintf(&b, "```sh\nexport %s=\"$(pwd)\"    # run this from the skill directory\n```\n\n", EnvProfile)
	fmt.Fprintf(&b, "Each shell command may be run in a fresh shell, so if the export does not\n"+
		"survive, prefix each command instead: `%s=/abs/path/to/skill %s check doc.md`.\n"+
		"The equivalent explicit form is `--schema-dir config/schemas --theme-dir\n"+
		"config/themes`, which still works and needs no environment at all.\n\n", EnvProfile, docc)

	b.WriteString("## Document types\n\n" +
		"The types below are the whole contract. There is no generic mode: a document\n" +
		"declares one of these in its frontmatter as `document_type`.\n\n" +
		"| Type | Purpose |\n|---|---|\n")
	for _, typ := range r.Types {
		desc := ""
		if sc, err := schemas.Get(typ); err == nil {
			desc = strings.TrimSpace(sc.Description)
		}
		if desc == "" {
			desc = "—"
		}
		fmt.Fprintf(&b, "| `%s` | %s |\n", typ, desc)
	}

	b.WriteString("\nThe schemas and themes live under `config/`:\n\n" +
		"- `config/schemas/` — the document types, their fields and body rules\n" +
		"- `config/themes/` — page geometry, styles and letterhead furniture\n\n")
	fmt.Fprintf(&b, "```sh\n%s types\n%s themes\n%s describe <type>\n%s example <type>\n%s doctor    # confirm the profile resolved to this skill\n```\n\n", docc, docc, docc, docc, docc)
	b.WriteString("`describe` prints the resolved contract for a type — every required field,\n" +
		"the body structure and the blocks it permits. Read it before writing a\n" +
		"document of a type for the first time.\n\n")

	b.WriteString("## Authoring workflow\n\n" +
		"1. Run `describe` for the type to learn its required frontmatter, its body\n" +
		"   structure, and the semantic blocks and spans it permits.\n" +
		"2. Ask the user for any missing facts — names, addresses, dates, references,\n" +
		"   amounts. **Do not invent them.** A plausible invented reference number is\n" +
		"   worse than an empty one, because nobody notices it.\n" +
		"3. Write only the Markdown body and YAML frontmatter. Do not hand-write\n" +
		"   letterhead, address block, subject line, signature or footer: the theme\n" +
		"   renders those, and a hand-written copy will differ from it.\n" +
		"4. Validate and fix every diagnostic. One run reports the complete list:\n\n")
	fmt.Fprintf(&b, "   ```sh\n   %s check --json document.md\n   ```\n\n", docc)
	b.WriteString("5. Build only after it validates cleanly:\n\n")
	fmt.Fprintf(&b, "   ```sh\n   %s build --output document.docx document.md\n   ```\n\n", docc)

	b.WriteString("`build` re-validates before rendering. Never pass `--force` for a deliverable.\n" +
		"`--strict` turns warnings into errors. Legal and contractual output is a\n" +
		"**draft that requires human review**.\n\n" +
		"Exit code `0` is clean, `1` reports diagnostics or a build failure, `2` means\n" +
		"the command line is wrong, and `3` that the schemas or themes could not be\n" +
		"resolved at all — `3` is a setup problem, never a problem with the document.\n" +
		"Read `--json` rather than scraping stderr. It has two shapes: a validation\n" +
		"result carries `ok`, `errors`, `warnings` and `diagnostics`, while a command\n" +
		"that could not run carries `ok: false`, an `error` message and a `kind` of\n" +
		"`usage`, `config` or `error`.\n\n")

	b.WriteString("## Frontmatter\n\n" +
		"A file becomes a docc document by declaring the format marker. Without it\n" +
		"nothing is validated and `check` reports `DOC024`, which is the answer to\n" +
		"\"why did my document pass with no output\".\n\n" +
		"```yaml\n---\ndocc: 1\ndocument_type: <one of the types above>\n---\n```\n\n" +
		"- Dates are ISO: `2026-08-04`.\n" +
		"- Quote a value whose leading zeros carry meaning, or YAML discards them:\n" +
		"  `postal_code: \"3000\"`.\n" +
		"- A field that is required *and* nullable must still be present. Write `~`\n" +
		"  only where the schema says an explicitly absent value is valid.\n\n")

	if spans := watchedSpans(schemas, r.Types); len(spans) > 0 {
		b.WriteString("## Several documents in one matter\n\n" +
			"These types require some values to say the same thing wherever they\n" +
			"appear: ")
		for i, span := range spans {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "`%s`", span)
		}
		b.WriteString(". Across several documents that agreement is only computed\n" +
			"when they are open together, so check them in **one** invocation:\n\n")
		fmt.Fprintf(&b, "```sh\n%s check --strict *.md\n```\n\n", docc)
		b.WriteString("Checking each file on its own passes every one of them and never compares\n" +
			"them, which is exactly the mistake that repetition invites. A disagreement\n" +
			"across files is `DOC029`. It is a warning, because files named on one command\n" +
			"line are not necessarily one matter; `--strict` makes it bind, which is what a\n" +
			"matter being filed should run.\n\n" +
			"Before delivering, confirm the set is complete — the types the matter\n" +
			"requires, not merely the ones drafted. Nothing counts the documents for you.\n\n")
	}

	b.WriteString("## Diagnostics\n\n" +
		"- `DOC004` required field missing · `DOC006` wrong scalar type · `DOC007` bad date\n" +
		"- `DOC008` disallowed enum value · `DOC010` pattern failed · `DOC011` unknown key\n" +
		"- `DOC020`–`DOC022` body-structure problems\n\n")
	fmt.Fprintf(&b, "Explain any engine code with `%s explain DOC010`. Codes a schema defines\ncarry their own hint inside the schema file.\n\n", docc)

	b.WriteString("## PDF\n\n" +
		"`.docx` is the compiler's supported output. When the user asks for a PDF,\n" +
		"build the `.docx` first and then use whatever document conversion this host\n" +
		"offers. `--to pdf` exists for compatibility and needs `soffice` installed,\n" +
		"which most hosts do not have.\n\n")

	b.WriteString("## When the request does not fit a type\n\n" +
		"The schemas and themes here are configuration, not values to work around. If\n" +
		"a requested document cannot be expressed by any type above, say what does not\n" +
		"fit and ask whether the owner of this profile wants to change it. Do not\n" +
		"weaken a required field, drop a rule, or reach for `--force` to make a draft\n" +
		"validate: a document that passes because the contract was lowered is worse\n" +
		"than one that visibly fails.\n\n")

	if len(r.Examples) > 0 {
		b.WriteString("## Try it\n\nA complete example ships for each type:\n\n")
		fmt.Fprintf(&b, "```sh\n%s build --output out.docx examples/%s\n```\n", docc, r.Examples[0])
	}
	if len(notes) > 0 {
		if !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteString("\n")
		}
		b.Write(notes)
		if !strings.HasSuffix(string(notes), "\n") {
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}

func skillDescription(name string, r *SkillResult) string {
	// A pack that states its own description knows something the generator
	// does not: which instructions should reach for it in the first place.
	if r.Description != "" {
		return r.Description
	}
	types := strings.Join(r.Types, ", ")
	carries := "Carries its document types and layouts; needs docc on PATH."
	if r.Binary != "" {
		carries = "Bundles its compiler, types and layouts; no install, no network."
	}
	return fmt.Sprintf("Validate and render schema-backed Markdown to .docx with the %s document profile. Document types: %s. %s", name, types, carries)
}

// probeScript writes a one-shot self-test. A skill that cannot say whether it
// works in the environment it landed in is a support ticket.
func probeScript(r *SkillResult) []byte {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env sh\n" +
		"# One-shot self-test for this docc skill: confirm the compiler runs here and\n" +
		"# that a bundled example builds to a .docx.\n" +
		"#\n" +
		"# Generated by `docc profile package`. Run it from the skill's directory.\n\n" +
		"set -eu\n\n" +
		"DIR=$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\ncd \"$DIR\"\n\n" +
		"# This skill is its own profile pack; resolving through it is part of what\n" +
		"# the probe proves.\n" +
		"DOCC_PROFILE=\"$DIR\"\nexport DOCC_PROFILE\n\n" +
		"echo \"== docc skill probe ==\"\n" +
		"echo \"uname -s : $(uname -s 2>/dev/null || echo '?')\"\n" +
		"echo \"uname -m : $(uname -m 2>/dev/null || echo '?')\"\n\n")

	if r.Binary != "" {
		fmt.Fprintf(&b, "BIN=\"bin/%s\"\n"+
			"if [ ! -f \"$BIN\" ]; then\n"+
			"  echo \"PROBE RESULT: FAIL — bundled binary $BIN is missing from the skill.\"; exit 3\n"+
			"fi\n"+
			"# Packaging may strip the exec bit; restore it.\n"+
			"chmod +x \"$BIN\" 2>/dev/null || true\n"+
			"DOCC=\"./$BIN\"\n\n", r.Binary)
		b.WriteString("if ! VER=$(\"$DOCC\" version 2>&1); then\n" +
			"  echo \"----- output -----\"; echo \"$VER\"; echo \"------------------\"\n" +
			"  echo \"PROBE RESULT: FAIL — this environment refused to execute the bundled binary.\"\n" +
			"  exit 1\n" +
			"fi\n")
	} else {
		b.WriteString("DOCC=docc\n\n" +
			"if ! VER=$(\"$DOCC\" version 2>&1); then\n" +
			"  echo \"PROBE RESULT: FAIL — docc is not on PATH; this skill carries configuration only.\"\n" +
			"  exit 1\n" +
			"fi\n")
	}
	b.WriteString("echo \"docc     : $VER\"\n\n")

	if len(r.Examples) == 0 {
		b.WriteString("echo \"PROBE RESULT: PASS — compiler runs. No bundled example to build.\"\n")
		return []byte(b.String())
	}
	fmt.Fprintf(&b, "EX=\"examples/%s\"\nOUT=\"$DIR/probe-out.docx\"\n\n"+
		"echo \"-- check $EX --\"\n"+
		"\"$DOCC\" check \"$EX\"\n"+
		"echo \"-- build $EX --\"\n"+
		"\"$DOCC\" build --output \"$OUT\" \"$EX\"\n\n"+
		"if [ -s \"$OUT\" ]; then\n"+
		"  echo \"wrote    : $OUT ($(wc -c < \"$OUT\") bytes)\"\n"+
		"  echo \"PROBE RESULT: PASS — the compiler runs here and produced a .docx.\"\n"+
		"else\n"+
		"  echo \"PROBE RESULT: FAIL — build reported success but wrote no .docx.\"\n"+
		"  exit 2\n"+
		"fi\n", r.Examples[0])
	return []byte(b.String())
}
