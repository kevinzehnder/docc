# Swiss Post: letter and envelope address layout

This is the layout reference for `docc` letter themes that place a recipient
address so it is visible through a standard **left-addressed C5/B5 landscape
window envelope**.

> These are requirements for the *address side of the envelope*. A letter page
> must be printed, folded and tested with the actual envelope stock: window
> locations can differ between suppliers.

## Authoritative source

- Swiss Post, [*Spezifikationen Briefgestaltung von A–Z*](https://www.post.ch/-/media/portal-opp/pm/dokumente/briefe-spezifikation-gestaltung.pdf), issue July 2026, pp. 7, 10–12, 23, 31–32.
- Swiss Post, [Briefe richtig gestalten und verpacken](https://www.post.ch/de/briefe-versenden/adressieren-und-gestalten/briefe-gestalten).

The PDF is the normative technical reference. The following values are
transcribed from it; check the current edition before changing production
layouts.

## Canonical left-window target on the letter page

For a C5/B5 landscape envelope addressed on the left (PDF p. 12), position the
recipient address on an A4 letter page at:

| Item | Value |
| --- | --- |
| Address text, left edge | **20 mm** from the left page edge |
| Address-field top edge | **50 mm** from the top page edge |
| First address line | at least **10 mm** below the field top — target **y = 60 mm** |
| Clear space at field right/bottom | **12 mm** |
| Coding zone | keep **140 × 15 mm** clear for formats up to B5 (including C5) |

The envelope’s upper-left sender zone is **120 × 40 mm**. Its upper-right
franking zone is **74 × 38 mm**. Those zones are relevant when printing the
envelope itself; a letterhead sender block is normally concealed by the
envelope.

## Recipient address content and typography

Swiss Post requires/recommends (PDF p. 31):

- 3–6 lines; no blank lines; left aligned.
- Address lines run in the longitudinal direction of the envelope.
- A complete address ends with street plus house number (or `Postfach`) and,
  on the final line, the PLZ and full town name.
- Use black, non-bold text. Do not use italic, decorative, blackletter,
  negative/reversed, condensed, expanded or letter-spaced type.
- Use a sans serif font such as Frutiger, Arial, Helvetica or Univers.
- Font size: 9–28 pt; **10 pt is the ideal size**.
- Visible space between lines: 1–1.5 mm.
- Do not underline or letter-space the PLZ or town.
- The entire address must be visible through the window.

Preferred ordering for an organisation addressed to a person is:

```text
Müller AG
Herr R. Bürki
Zollikerstrasse 788
8008 Zürich
```

For a private recipient, Swiss Post illustrates:

```text
Herr
Hans Schweizer
Gerechtigkeitsgasse 10
3011 Bern
```

## Sender address on an envelope

A sender address printed on the envelope must be recognisable without research
and include company/person name, street or `Postfach`, and PLZ plus town (PDF
pp. 10–11). It belongs left of or above the recipient (and, in landscape,
higher than the recipient). A sender line immediately above the recipient must
be one line and separated by a visible horizontal rule.

A separate sender window needs at least 12 mm to the envelope edge and 3 mm to
its window edge. It must be clearly separate from the recipient window.

## Address field and window restrictions

Address and coding zones must be white or only very lightly tinted. They may
not contain patterns, gradients, fluorescent colours, frames/borders, or
unrelated marks (PDF pp. 7, 23).

The minimum address-window size is **100 × 45 mm**; the minimum address-label
size is **70 × 35 mm**.

## `docc` implementation policy

`jlmy-letter` is the canonical C5 left-window theme:

- recipient text begins at x = 25 mm (5 mm inside the field's 20 mm left edge) and is targeted at y = 60 mm;
- recipient text uses a black 10 pt sans-serif address style;
- the letter body and recipient address share a balanced 25 mm left/right margin;
- positioning must be regression-tested by rendering, folding and checking a
  printed sample in the selected envelope.

A right-window layout must be a separate, measured theme until `docc` supports
conditional furniture/style selection. A boolean alone cannot change a theme’s
fixed furniture geometry.
