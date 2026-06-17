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

func TestPositionToOffset_BoundsCharacterToLine(t *testing.T) {
	// "hello" occupies bytes 0..4 with its newline at index 5; "world"
	// starts at offset 6 with its newline at index 11.
	content := []byte("hello\nworld\n")
	cases := []struct {
		name string
		pos  Position
		want int
	}{
		{"char within line", Position{Line: 0, Character: 3}, 3},
		// Character pointing at the terminating newline is the valid
		// line-end position (offset+Character == line-end), not overflow.
		{"char at line-end newline", Position{Line: 0, Character: 5}, 5},
		// Character past the newline must not spill into the next line.
		{"char one past line-end", Position{Line: 0, Character: 6}, -1},
		{"char far past line-end", Position{Line: 0, Character: 99}, -1},
		{"second line char at end", Position{Line: 1, Character: 5}, 11},
		{"second line char past end", Position{Line: 1, Character: 6}, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := positionToOffset(content, tc.pos)
			if got != tc.want {
				t.Errorf("positionToOffset(%q, %+v) = %d; want %d", content, tc.pos, got, tc.want)
			}
		})
	}
}

func TestPositionToOffset_LastLineWithoutNewline(t *testing.T) {
	content := []byte("hello") // no trailing newline
	if got := positionToOffset(content, Position{Line: 0, Character: 5}); got != 5 {
		t.Errorf("char at EOF: got %d, want 5", got)
	}
	if got := positionToOffset(content, Position{Line: 0, Character: 6}); got != -1 {
		t.Errorf("char past EOF: got %d, want -1", got)
	}
}

func TestApplyEdits_OutOfRangeCharacterIsError(t *testing.T) {
	content := []byte("hello\nworld\n")
	// Character 10 on line 0 would spill past "hello"'s newline into "world"
	// if the in-line advance were unbounded.
	edits := []TextEdit{{
		Range:   Range{Start: Position{Line: 0, Character: 10}, End: Position{Line: 0, Character: 11}},
		NewText: "X",
	}}
	_, err := applyEdits(content, edits)
	if err == nil {
		t.Fatal("expected out-of-bounds error for over-long character offset")
	}
	if !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("expected 'out of bounds' error, got: %v", err)
	}
}

func TestApplyEdits_SamePositionInsertsKeepArrayOrder(t *testing.T) {
	pos := Position{Line: 0, Character: 1}
	// Two zero-width inserts at the same position (between A and B).
	got, err := applyEdits([]byte("AB"), []TextEdit{
		{Range: Range{Start: pos, End: pos}, NewText: "X"},
		{Range: Range{Start: pos, End: pos}, NewText: "Y"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// LSP: same-position inserts appear in array order -> A X Y B.
	if string(got) != "AXYB" {
		t.Errorf("two same-position inserts: got %q, want %q", string(got), "AXYB")
	}
}

func TestApplyEdits_SamePositionThreeInserts(t *testing.T) {
	pos := Position{Line: 0, Character: 1}
	got, err := applyEdits([]byte("AB"), []TextEdit{
		{Range: Range{Start: pos, End: pos}, NewText: "1"},
		{Range: Range{Start: pos, End: pos}, NewText: "2"},
		{Range: Range{Start: pos, End: pos}, NewText: "3"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "A123B" {
		t.Errorf("three same-position inserts: got %q, want %q", string(got), "A123B")
	}
}

func TestApplyEdits_SamePositionInsertThenReplace(t *testing.T) {
	// An insert and a replace that both start at the same position. Array
	// order is [insert "Z" at char 1, replace chars 1..3 with "Q"].
	// On original "abcd": insert Z at 1 then replace "bc"->"Q" => "aZQd".
	got, err := applyEdits([]byte("abcd"), []TextEdit{
		{Range: Range{Start: Position{Line: 0, Character: 1}, End: Position{Line: 0, Character: 1}}, NewText: "Z"},
		{Range: Range{Start: Position{Line: 0, Character: 1}, End: Position{Line: 0, Character: 3}}, NewText: "Q"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "aZQd" {
		t.Errorf("insert+replace at same start: got %q, want %q", string(got), "aZQd")
	}
}

func TestPositionToOffset_MultiByteContentUsesByteOffsets(t *testing.T) {
	// positionToOffset works in bytes: Character is a byte offset within the
	// line, not a rune or UTF-16 index (the LSP->byte conversion happens
	// upstream). With multi-byte runes the byte offset and the rune count
	// diverge, so this pins that the function counts bytes and still bounds
	// Character to the line. "café" is 5 bytes (é = 2 bytes) with its newline
	// at byte 5; line 1 "汉字x" starts at byte 6 (汉/字 = 3 bytes each, x = 1).
	content := []byte("café\n汉字x\n")
	cases := []struct {
		name string
		pos  Position
		want int
	}{
		{"start of multi-byte line", Position{Line: 0, Character: 0}, 0},
		{"byte at start of é", Position{Line: 0, Character: 3}, 3},
		{"line-end newline after 2-byte é", Position{Line: 0, Character: 5}, 5},
		{"start of CJK line", Position{Line: 1, Character: 0}, 6},
		{"byte at start of 字", Position{Line: 1, Character: 3}, 9},
		{"line-end of CJK line", Position{Line: 1, Character: 7}, 13},
		{"one byte past CJK line spills to next line", Position{Line: 1, Character: 8}, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := positionToOffset(content, tc.pos)
			if got != tc.want {
				t.Errorf("positionToOffset(%q, %+v) = %d; want %d", content, tc.pos, got, tc.want)
			}
		})
	}
}

func TestApplyEdits_MultiByteContentReplacement(t *testing.T) {
	// Replacing a run of multi-byte runes must use the byte offsets the range
	// resolves to and leave the surrounding multi-byte content byte-for-byte
	// intact. "café " is 6 bytes (é = 2) so 汉字 occupy bytes 6..11 and the
	// newline is the line-end at byte 12.
	content := []byte("café 汉字\n")
	edits := []TextEdit{{
		Range:   Range{Start: Position{Line: 0, Character: 6}, End: Position{Line: 0, Character: 12}},
		NewText: "ASCII",
	}}
	got, err := applyEdits(content, edits)
	if err != nil {
		t.Fatalf("applyEdits: %v", err)
	}
	if string(got) != "café ASCII\n" {
		t.Fatalf("multi-byte replacement: got %q, want %q", string(got), "café ASCII\n")
	}
}

func TestApplyEdits_MultiByteSamePositionInsertOrder(t *testing.T) {
	// The same-position array-order rule must hold inside multi-byte content
	// and with multi-byte insert text. "aé" is 3 bytes; Character 1 is the
	// rune boundary between 'a' and 'é', so both inserts land there and must
	// appear in array order without splitting the following 2-byte 'é'.
	pos := Position{Line: 0, Character: 1}
	got, err := applyEdits([]byte("aé"), []TextEdit{
		{Range: Range{Start: pos, End: pos}, NewText: "✓"},
		{Range: Range{Start: pos, End: pos}, NewText: "X"},
	})
	if err != nil {
		t.Fatalf("applyEdits: %v", err)
	}
	if string(got) != "a✓Xé" {
		t.Fatalf("multi-byte same-position inserts: got %q, want %q", string(got), "a✓Xé")
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

func TestRollback_SurfacesRestoreFailure(t *testing.T) {
	dir := t.TempDir()
	origPath := filepath.Join(dir, "first.go")
	// A backup path that does not exist forces os.Rename(backup, orig) to
	// fail, simulating a rollback that cannot restore the original file.
	backupPath := filepath.Join(dir, "first.go.deadbeef.refute.bak")

	err := rollback([]pendingFile{{origPath: origPath, backupPath: backupPath}})
	if err == nil {
		t.Fatal("expected rollback to report a restore failure")
	}
	if !strings.Contains(err.Error(), origPath) {
		t.Errorf("rollback error should name the inconsistent file %q; got: %v", origPath, err)
	}
	if !strings.Contains(err.Error(), backupPath) {
		t.Errorf("rollback error should name the leftover backup %q; got: %v", backupPath, err)
	}
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
