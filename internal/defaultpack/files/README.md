# docc starter profiles

This is the starter profile pack that ships inside the docc binary. `docc init`
copies it out as an editable checkout; without any configuration, docc resolves
the embedded copy directly. It contains two intentionally Swiss-oriented
document types:

- `letter` — business or legal correspondence in a left-window envelope layout.
- `legal` — a civil-law brief with the usual Swiss headings and exhibit checks.

They are starting points, not production house style. Before sending a document:

1. Replace the `YOUR …` values in `themes/starter-legal.yaml` with the approved
   legal-letterhead data and check the resulting layout against your stationery.
2. Adapt the schemas to the fields, rules, language, and document structure your
   organisation actually uses.
3. Rename themes and update each schema's `theme:` value if you make a separate
   house style.

`docc init` also writes sample documents into `examples/`. From the checkout
root, try:

```sh
docc check examples/letter.md
docc build examples/letter.md
docc check examples/legal.md
docc build examples/legal.md
```

`docc` finds this pack through `docc-profile.yaml` by walking up from the input
document. No LLM integration is required; an author can write the Markdown and
use `docc check`. An agent can use `docc check --json` to correct its draft
against the same schema.
