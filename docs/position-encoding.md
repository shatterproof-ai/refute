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

## Implementation note

Convert at the LSP boundary. Read the source line as a string, slice to the
1-indexed byte offset, count UTF-16 code units via `utf16.Encode` (or the
smaller-memory `utf8.DecodeRuneInString` + UTF-16 counter). Send that count as
the LSP character. Reverse on the way back.
