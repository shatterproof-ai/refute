package edit

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPositionToOffset_NegativeInputsReturnSentinel(t *testing.T) {
	// "hello\nworld\n": line 0 = "hello", line 1 = "world"
	content := []byte("hello\nworld\n")
	cases := []struct {
		name string
		pos  Position
	}{
		// Line > 0 so the loop walks past the first newline before matching;
		// without the guard, offset + Character yields a positive non-negative
		// value that passes the > len(content) check, returning a bogus offset.
		{"negative Character on non-zero line", Position{Line: 1, Character: -1}},
		{"negative Character on zero line", Position{Line: 0, Character: -1}},
		{"negative Line", Position{Line: -1, Character: 0}},
		{"both negative", Position{Line: -1, Character: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := positionToOffset(content, tc.pos)
			if got != -1 {
				t.Errorf("positionToOffset(%q, %+v) = %d; want -1", content, tc.pos, got)
			}
		})
	}
}

func FuzzPositionToOffset(f *testing.F) {
	add := func(content string, line, char int) {
		var hdr [8]byte
		binary.LittleEndian.PutUint32(hdr[0:4], uint32(int32(line)))
		binary.LittleEndian.PutUint32(hdr[4:8], uint32(int32(char)))
		f.Add(append(hdr[:], []byte(content)...))
	}
	add("hello\nworld\n", 1, 2)
	add("a\r\nb\r\nc", 1, 1)
	add("", 0, 0)
	add("only one line", 0, 5)
	add("past\neof\n", 99, 0)
	add("neg", -1, 0)
	add("muélti", 0, 3)

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 8 {
			return
		}
		line := int(int32(binary.LittleEndian.Uint32(data[0:4])))
		char := int(int32(binary.LittleEndian.Uint32(data[4:8])))
		content := data[8:]
		// Invariant: never panic.
		_ = positionToOffset(content, Position{Line: line, Character: char})
	})
}

func FuzzApplyEdits(f *testing.F) {
	encode := func(content string, edits []TextEdit) []byte {
		var buf []byte
		var hdr [4]byte
		binary.LittleEndian.PutUint32(hdr[:], uint32(len(content)))
		buf = append(buf, hdr[:]...)
		buf = append(buf, []byte(content)...)
		for _, e := range edits {
			var er [20]byte
			binary.LittleEndian.PutUint32(er[0:4], uint32(int32(e.Range.Start.Line)))
			binary.LittleEndian.PutUint32(er[4:8], uint32(int32(e.Range.Start.Character)))
			binary.LittleEndian.PutUint32(er[8:12], uint32(int32(e.Range.End.Line)))
			binary.LittleEndian.PutUint32(er[12:16], uint32(int32(e.Range.End.Character)))
			binary.LittleEndian.PutUint32(er[16:20], uint32(len(e.NewText)))
			buf = append(buf, er[:]...)
			buf = append(buf, []byte(e.NewText)...)
		}
		return buf
	}

	f.Add(encode("hello\nworld\n", []TextEdit{{
		Range:   Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 5}},
		NewText: "HELLO",
	}}))
	f.Add(encode("a\nb\nc\n", []TextEdit{
		{Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 1}}, NewText: "X"},
		{Range: Range{Start: Position{Line: 2, Character: 0}, End: Position{Line: 2, Character: 1}}, NewText: "Z"},
	}))
	f.Add(encode("line\n", []TextEdit{{
		Range:   Range{Start: Position{Line: 0, Character: 4}, End: Position{Line: 0, Character: 4}},
		NewText: "!",
	}}))
	f.Add(encode("", nil))
	f.Add(encode("x", []TextEdit{{
		Range:   Range{Start: Position{Line: 5, Character: 0}, End: Position{Line: 5, Character: 1}},
		NewText: "out of bounds",
	}}))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 4 {
			return
		}
		contentLen := int(binary.LittleEndian.Uint32(data[0:4]))
		if contentLen < 0 || contentLen > len(data)-4 {
			return
		}
		content := make([]byte, contentLen)
		copy(content, data[4:4+contentLen])
		rest := data[4+contentLen:]

		var edits []TextEdit
		const maxEdits = 32
		for len(rest) >= 20 && len(edits) < maxEdits {
			startLine := int(int32(binary.LittleEndian.Uint32(rest[0:4])))
			startChar := int(int32(binary.LittleEndian.Uint32(rest[4:8])))
			endLine := int(int32(binary.LittleEndian.Uint32(rest[8:12])))
			endChar := int(int32(binary.LittleEndian.Uint32(rest[12:16])))
			textLen := int(binary.LittleEndian.Uint32(rest[16:20]))
			rest = rest[20:]
			if textLen < 0 || textLen > len(rest) {
				break
			}
			edits = append(edits, TextEdit{
				Range: Range{
					Start: Position{Line: startLine, Character: startChar},
					End:   Position{Line: endLine, Character: endChar},
				},
				NewText: string(rest[:textLen]),
			})
			rest = rest[textLen:]
		}

		// Invariant: never panic; an error is acceptable for any input.
		_, _ = applyEdits(content, edits)
	})
}

func TestCommitPendingFiles_RollsBackCommittedFilesOnLaterFailure(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.go")
	secondPath := filepath.Join(dir, "second.go")

	firstOriginal := []byte("package main\n\nfunc first() {}\n")
	secondOriginal := []byte("package main\n\nfunc second() {}\n")
	if err := os.WriteFile(firstPath, firstOriginal, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, secondOriginal, 0o644); err != nil {
		t.Fatal(err)
	}

	firstTmp := filepath.Join(dir, "first.tmp")
	if err := os.WriteFile(firstTmp, []byte("package main\n\nfunc updatedFirst() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missingSecondTmp := filepath.Join(dir, "missing-second.tmp")

	err := commitPendingFiles([]pendingFile{
		{origPath: firstPath, tmpPath: firstTmp},
		{origPath: secondPath, tmpPath: missingSecondTmp},
	})
	if err == nil {
		t.Fatal("expected commitPendingFiles to fail")
	}
	if !strings.Contains(err.Error(), "missing-second.tmp") {
		t.Fatalf("expected missing temp path in error, got %v", err)
	}

	gotFirst, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotFirst) != string(firstOriginal) {
		t.Errorf("first file was not rolled back\ngot:  %q\nwant: %q", string(gotFirst), string(firstOriginal))
	}

	gotSecond, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSecond) != string(secondOriginal) {
		t.Errorf("second file was not restored\ngot:  %q\nwant: %q", string(gotSecond), string(secondOriginal))
	}
}
