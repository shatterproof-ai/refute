package capture

import (
	"strings"
	"testing"
)

const truncationMarker = " ... [stderr truncated]"

// write captures content via the helper's File() handle, mirroring how a
// subprocess would write to the assigned Stderr.
func write(t *testing.T, s *Stderr, content string) {
	t.Helper()
	if _, err := s.File().Write([]byte(content)); err != nil {
		t.Fatalf("write capture: %v", err)
	}
}

func TestReadReturnsTrimmedContent(t *testing.T) {
	s, err := New("capture-test-*")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Cleanup()

	write(t, s, "  boom: parse error\n\n")
	got := s.Read(DefaultMaxBytes)
	if got != "boom: parse error" {
		t.Fatalf("Read = %q, want %q", got, "boom: parse error")
	}
}

func TestReadEmptyAndWhitespaceOnly(t *testing.T) {
	s, err := New("capture-test-*")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Cleanup()

	if got := s.Read(DefaultMaxBytes); got != "" {
		t.Fatalf("Read of empty capture = %q, want empty", got)
	}

	write(t, s, "   \n\t  ")
	if got := s.Read(DefaultMaxBytes); got != "" {
		t.Fatalf("Read of whitespace-only capture = %q, want empty", got)
	}
}

// TestReadLimitBoundary exercises the io.LimitReader(max+1) boundary: output of
// exactly max bytes must not be marked truncated, while max+1 bytes must be
// truncated to max bytes with the marker appended.
func TestReadLimitBoundary(t *testing.T) {
	const max = 8

	t.Run("exactly max is not truncated", func(t *testing.T) {
		s, err := New("capture-test-*")
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer s.Cleanup()

		write(t, s, strings.Repeat("a", max))
		got := s.Read(max)
		if strings.Contains(got, truncationMarker) {
			t.Fatalf("Read = %q, unexpected truncation marker", got)
		}
		if got != strings.Repeat("a", max) {
			t.Fatalf("Read = %q, want %d 'a's", got, max)
		}
	})

	t.Run("one over max is truncated and marked", func(t *testing.T) {
		s, err := New("capture-test-*")
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer s.Cleanup()

		write(t, s, strings.Repeat("a", max+1))
		got := s.Read(max)
		want := strings.Repeat("a", max) + truncationMarker
		if got != want {
			t.Fatalf("Read = %q, want %q", got, want)
		}
	})
}

// TestReadTruncationMarkerOnlyWhenNonEmpty ensures a truncated capture still
// omits the marker when the retained message trims to empty.
func TestReadTruncationMarkerOnlyWhenNonEmpty(t *testing.T) {
	const max = 4
	s, err := New("capture-test-*")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Cleanup()

	// First max bytes trim to empty, overflow byte is also whitespace; result
	// must be empty with no marker.
	write(t, s, "     ")
	if got := s.Read(max); got != "" {
		t.Fatalf("Read = %q, want empty (no marker on empty message)", got)
	}
}

func TestReadAfterCleanup(t *testing.T) {
	s, err := New("capture-test-*")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	write(t, s, "diagnostic")
	s.Cleanup()

	if got := s.Read(DefaultMaxBytes); got != "" {
		t.Fatalf("Read after Cleanup = %q, want empty", got)
	}
	// Cleanup is idempotent.
	s.Cleanup()
}

func TestNilReceiver(t *testing.T) {
	var s *Stderr
	if got := s.Read(DefaultMaxBytes); got != "" {
		t.Fatalf("nil Read = %q, want empty", got)
	}
	if s.File() != nil {
		t.Fatal("nil File() should be nil")
	}
	s.Cleanup() // must not panic
}
