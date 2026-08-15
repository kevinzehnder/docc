package profile

import (
	"fmt"
	"strings"

	"github.com/kevinzehnder/docc/internal/schema"
)

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
		"- `config/themes/` — page geometry, styles and letterhead furniture\n\n" +
		"Point docc at them explicitly, because this is not the usual hidden `.docc/`:\n\n")
	fmt.Fprintf(&b, "```sh\n%s types    --schema-dir config/schemas\n%s themes   --theme-dir  config/themes\n%s describe --schema-dir config/schemas <type>\n%s example  --schema-dir config/schemas <type>\n```\n\n", docc, docc, docc, docc)
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
	fmt.Fprintf(&b, "   ```sh\n   %s check --json --schema-dir config/schemas document.md\n   ```\n\n", docc)
	b.WriteString("5. Build only after it validates cleanly:\n\n")
	fmt.Fprintf(&b, "   ```sh\n   %s build --schema-dir config/schemas --theme-dir config/themes \\\n       --output document.docx document.md\n   ```\n\n", docc)

	b.WriteString("`build` re-validates before rendering. Never pass `--force` for a deliverable.\n" +
		"Exit code `0` is clean, `1` reports diagnostics or a build failure, `2` is a\n" +
		"usage or configuration error. Legal and contractual output is a **draft that\n" +
		"requires human review**.\n\n")

	b.WriteString("## Diagnostics\n\n" +
		"- `DOC004` required field missing · `DOC006` wrong scalar type · `DOC007` bad date\n" +
		"- `DOC008` disallowed enum value · `DOC010` pattern failed · `DOC011` unknown key\n" +
		"- `DOC020`–`DOC022` body-structure problems\n\n")
	fmt.Fprintf(&b, "Explain any engine code with `%s explain DOC010`. Codes a schema defines\ncarry their own hint inside the schema file.\n\n", docc)

	if len(r.Examples) > 0 {
		b.WriteString("## Try it\n\nA complete example ships for each type:\n\n")
		fmt.Fprintf(&b, "```sh\n%s build --schema-dir config/schemas --theme-dir config/themes \\\n    --output out.docx examples/%s\n```\n", docc, r.Examples[0])
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
		"\"$DOCC\" check --schema-dir config/schemas \"$EX\"\n"+
		"echo \"-- build $EX --\"\n"+
		"\"$DOCC\" build --schema-dir config/schemas --theme-dir config/themes --output \"$OUT\" \"$EX\"\n\n"+
		"if [ -s \"$OUT\" ]; then\n"+
		"  echo \"wrote    : $OUT ($(wc -c < \"$OUT\") bytes)\"\n"+
		"  echo \"PROBE RESULT: PASS — the compiler runs here and produced a .docx.\"\n"+
		"else\n"+
		"  echo \"PROBE RESULT: FAIL — build reported success but wrote no .docx.\"\n"+
		"  exit 2\n"+
		"fi\n", r.Examples[0])
	return []byte(b.String())
}
