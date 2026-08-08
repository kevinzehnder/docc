# docc starter configuration

`docc init` installed this generic starter configuration. It contains two intentionally
Swiss-oriented document types:

- `letter` — business or legal correspondence in a left-window envelope layout.
- `legal` — a civil-law brief with the usual Swiss headings and exhibit checks.

They are starting points, not production house style. Before sending a document:

1. Replace the `YOUR …` values in `themes/starter-legal.yaml` with the approved
   legal-letterhead data and check the resulting layout against your stationery.
2. Adapt the schemas to the fields, rules, language, and document structure your
   organisation actually uses.
3. Rename themes and update each schema's `theme:` value if you make a separate
   house style.

The sample documents live in `examples/docc/`. From the project root, try:

```sh
docc check examples/docc/letter.md
docc build examples/docc/letter.md
docc check examples/docc/legal.md
docc build examples/docc/legal.md
```

`docc` finds this `.docc` directory by walking up from the input document.
No LLM integration is required; an author can write the Markdown and use
`docc check`. An agent can use `docc check --json` to correct its draft against
the same schema.
