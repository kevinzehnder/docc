## Cowork host notes

The VM is x86_64, which is the architecture the bundled binary is built for.
`probe.sh` checks it and reports the architecture it found.

DOCX is the compiler's supported output. When the user asks for a PDF, build the
DOCX first and then use Cowork's own document/PDF capability. The compiler's
`--to pdf` needs `soffice`, which is not installed here.

Write deliverables into the workspace so the user receives them.
