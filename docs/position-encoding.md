# Position Encoding

`refute` uses **1-indexed byte-offset columns** in its CLI and JSON output.
LSP servers use **0-indexed UTF-16 code-unit columns**.

For ASCII source (the vast majority of code), these are identical after the
1→0 conversion. For source containing multi-byte Unicode (non-ASCII identifiers,
emoji in string literals, etc.) the two encodings diverge, which means a
`refute` column computed from a byte count will NOT match the LSP column.

## Current behavior

The LSP adapter converts byte-offset columns to UTF-16 code-unit columns before
sending requests to language servers. LSP result ranges are converted back to
byte offsets before edits are applied or rendered as JSON.

The ts-morph adapter follows the same boundary rule on the Go side. Public
`refute` symbol locations stay 1-indexed byte columns, and internal
`edit.Position` ranges stay 0-indexed byte offsets. Before invoking
`adapters/tsmorph/rename.cjs`, the Go adapter converts request columns to the
1-indexed UTF-16 columns used by JavaScript strings and ts-morph. Results from
the Node adapter are converted back to byte offsets before they enter the rest
of `refute`.

## Implementation note

Convert at protocol or subprocess boundaries. Read the source line as a string,
slice to the 1-indexed byte offset, count UTF-16 code units via `utf16.Encode`
(or the smaller-memory `utf8.DecodeRuneInString` + UTF-16 counter). Send that
count in the foreign protocol's expected indexing convention. Reverse on the
way back.
