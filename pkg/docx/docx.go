// Package docx will emit Word documents from a validated document tree.
//
// The intended design follows goproject/pkg/worddoc: read a Word-authored
// .dotx, fill {{key}} markers run-aware, and splice generated block XML at
// named markers. The piece worddoc does not have is a markdown block tree ->
// WordprocessingML converter that maps headings, lists and fenced divs onto the
// style names a schema declares.
//
// Not yet implemented.
package docx
