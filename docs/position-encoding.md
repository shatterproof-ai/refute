# Position Encoding

`refute` uses **1-indexed byte-offset columns** in its CLI and JSON output.
LSP servers use **0-indexed UTF-16 code-unit columns**.

For ASCII source (the vast majority of code), these are identical after the
1→0 conversion. For source containing multi-byte Unicode (non-ASCII identifiers,
emoji in string literals, etc.) the two encodings diverge, which means a
`refute` column computed from a byte count will NOT match the LSP column.

## Current behavior

The conversion in `internal/backend/lsp/adapter.go` is a straight subtract-1.
This is correct for ASCII only.

## If you need Unicode

Convert at the LSP boundary. Read the source line as a string, slice to the
1-indexed byte offset, count UTF-16 code units via `utf16.Encode` (or the
smaller-memory `utf8.DecodeRuneInString` + UTF-16 counter). Send that count
as the LSP character. Reverse on the way back.

This is deferred to a follow-up. File an issue and reference this document
if you hit a Unicode case.
