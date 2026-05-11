package edit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
