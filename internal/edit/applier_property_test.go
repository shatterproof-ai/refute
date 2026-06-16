package edit

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	"pgregory.net/rapid"
)

const (
	maxPropertyContentBytes = 128
	maxPropertyEdits        = 8
	maxPropertyNewTextBytes = 16
)

type propertyEditRange struct {
	start   int
	end     int
	newText []byte
}

type propertyEditCase struct {
	content []byte
	ranges  []propertyEditRange
	edits   []TextEdit
}

func TestApplyEditsIdentityProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		content := drawPropertyContent(t)

		got, err := applyEdits(content, nil)
		if err != nil {
			t.Fatalf("applyEdits(%q, nil): %v", content, err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("identity failed: got %q, want %q", got, content)
		}
	})
}

func TestApplyEditsLengthAndLocalityProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tc := drawPropertyEditCase(t)

		got, err := applyEdits(tc.content, tc.edits)
		if err != nil {
			t.Fatalf("applyEdits(%q, %+v): %v", tc.content, tc.edits, err)
		}

		wantLen := len(tc.content)
		for _, r := range tc.ranges {
			wantLen += len(r.newText) - (r.end - r.start)
		}
		if len(got) != wantLen {
			t.Fatalf("length mismatch: got %d, want %d; content=%q edits=%+v result=%q", len(got), wantLen, tc.content, tc.edits, got)
		}

		assertLocality(t, tc.content, got, tc.ranges)
	})
}

func TestApplyEditsRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		tc := drawPropertyEditCase(t)

		forward, err := applyEdits(tc.content, tc.edits)
		if err != nil {
			t.Fatalf("forward applyEdits(%q, %+v): %v", tc.content, tc.edits, err)
		}

		inverse := inverseEdits(tc.content, forward, sortedPropertyRanges(tc.ranges))
		got, err := applyEdits(forward, inverse)
		if err != nil {
			t.Fatalf("inverse applyEdits(%q, %+v): %v; original=%q forwardEdits=%+v", forward, inverse, err, tc.content, tc.edits)
		}
		if !bytes.Equal(got, tc.content) {
			t.Fatalf("round-trip mismatch: got %q, want %q; forward=%q forwardEdits=%+v inverse=%+v", got, tc.content, forward, tc.edits, inverse)
		}
	})
}

func TestPositionToOffsetBoundsProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		content := drawPropertyContent(t)
		line := rapid.IntRange(-5, len(content)+5).Draw(t, "line")
		character := rapid.IntRange(-5, len(content)+5).Draw(t, "character")

		got := positionToOffset(content, Position{Line: line, Character: character})
		if got < -1 || got > len(content) {
			t.Fatalf("positionToOffset(%q, line=%d character=%d) = %d, want [-1,%d]", content, line, character, got, len(content))
		}
	})
}

func TestPositionToOffsetStartOfLineProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		content := drawPropertyContent(t)
		starts := lineStartOffsets(content)
		line := rapid.IntRange(0, len(starts)-1).Draw(t, "line")

		got := positionToOffset(content, Position{Line: line, Character: 0})
		if got != starts[line] {
			t.Fatalf("positionToOffset(%q, line=%d character=0) = %d, want line start %d", content, line, got, starts[line])
		}
	})
}

func TestPositionToOffsetRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		content := drawPropertyContent(t)
		offset := rapid.IntRange(0, len(content)).Draw(t, "offset")
		pos := offsetToPosition(content, offset)

		got := positionToOffset(content, pos)
		if got != offset {
			t.Fatalf("positionToOffset(%q, offsetToPosition(%d)=%+v) = %d, want %d", content, offset, pos, got, offset)
		}
	})
}

func drawPropertyContent(t *rapid.T) []byte {
	content := rapid.SliceOfN(rapid.Byte(), 0, maxPropertyContentBytes).Draw(t, "content")
	return append([]byte(nil), content...)
}

func drawPropertyEditCase(t *rapid.T) propertyEditCase {
	content := drawPropertyContent(t)
	count := rapid.IntRange(0, maxPropertyEdits).Draw(t, "edit count")

	ranges := make([]propertyEditRange, 0, count)
	cursor := 0
	for i := 0; i < count && cursor <= len(content); i++ {
		start := rapid.IntRange(cursor, len(content)).Draw(t, fmt.Sprintf("edit %d start", i))
		end := rapid.IntRange(start, len(content)).Draw(t, fmt.Sprintf("edit %d end", i))
		newText := rapid.SliceOfN(rapid.Byte(), 0, maxPropertyNewTextBytes).Draw(t, fmt.Sprintf("edit %d new text", i))
		ranges = append(ranges, propertyEditRange{
			start:   start,
			end:     end,
			newText: append([]byte(nil), newText...),
		})

		cursor = end
		if start == end {
			cursor = start + 1
		}
	}

	permuted := rapid.Permutation(ranges).Draw(t, "edit order")
	edits := make([]TextEdit, 0, len(permuted))
	for _, r := range permuted {
		edits = append(edits, TextEdit{
			Range: Range{
				Start: offsetToPosition(content, r.start),
				End:   offsetToPosition(content, r.end),
			},
			NewText: string(r.newText),
		})
	}

	return propertyEditCase{
		content: content,
		ranges:  sortedPropertyRanges(ranges),
		edits:   edits,
	}
}

func sortedPropertyRanges(ranges []propertyEditRange) []propertyEditRange {
	sorted := append([]propertyEditRange(nil), ranges...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].start != sorted[j].start {
			return sorted[i].start < sorted[j].start
		}
		return sorted[i].end < sorted[j].end
	})
	return sorted
}

func assertLocality(t *rapid.T, original []byte, edited []byte, ranges []propertyEditRange) {
	t.Helper()

	originalCursor := 0
	editedCursor := 0
	for _, r := range ranges {
		segmentLen := r.start - originalCursor
		if !bytes.Equal(edited[editedCursor:editedCursor+segmentLen], original[originalCursor:r.start]) {
			t.Fatalf("locality mismatch before range [%d,%d): got %q, want %q; original=%q edited=%q", r.start, r.end, edited[editedCursor:editedCursor+segmentLen], original[originalCursor:r.start], original, edited)
		}
		editedCursor += segmentLen + len(r.newText)
		originalCursor = r.end
	}

	if !bytes.Equal(edited[editedCursor:], original[originalCursor:]) {
		t.Fatalf("locality mismatch after final range: got %q, want %q; original=%q edited=%q", edited[editedCursor:], original[originalCursor:], original, edited)
	}
}

func inverseEdits(original []byte, edited []byte, ranges []propertyEditRange) []TextEdit {
	inverse := make([]TextEdit, 0, len(ranges))
	delta := 0
	for _, r := range ranges {
		start := r.start + delta
		end := start + len(r.newText)
		inverse = append(inverse, TextEdit{
			Range: Range{
				Start: offsetToPosition(edited, start),
				End:   offsetToPosition(edited, end),
			},
			NewText: string(original[r.start:r.end]),
		})
		delta += len(r.newText) - (r.end - r.start)
	}
	return inverse
}

func offsetToPosition(content []byte, offset int) Position {
	line := 0
	lineStart := 0
	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return Position{Line: line, Character: offset - lineStart}
}

func lineStartOffsets(content []byte) []int {
	starts := []int{0}
	for i, b := range content {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}
