package edit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// TestApplyCreateThenEditPopulatesNewFile covers the "extract to new file"
// shape: a CreateFile op produces an empty file that a subsequent text edit
// fills in, while the original file is edited to remove the extracted code.
func TestApplyCreateThenEditPopulatesNewFile(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig.go")
	newFile := filepath.Join(dir, "extracted.go")
	if err := os.WriteFile(orig, []byte("package p\n\nfunc Keep() {}\nfunc Move() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileOps: []edit.FileOperation{
			{Kind: edit.FileOpCreate, Path: newFile},
		},
		FileEdits: []edit.FileEdit{
			{
				Path: newFile,
				Edits: []edit.TextEdit{
					{Range: edit.Range{Start: edit.Position{Line: 0, Character: 0}, End: edit.Position{Line: 0, Character: 0}}, NewText: "package p\n\nfunc Move() {}\n"},
				},
			},
			{
				Path: orig,
				Edits: []edit.TextEdit{
					{Range: edit.Range{Start: edit.Position{Line: 3, Character: 0}, End: edit.Position{Line: 4, Character: 0}}, NewText: ""},
				},
			},
		},
	}

	if _, err := edit.ApplyWithin(we, dir); err != nil {
		t.Fatalf("ApplyWithin: %v", err)
	}

	gotNew, err := os.ReadFile(newFile)
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(gotNew) != "package p\n\nfunc Move() {}\n" {
		t.Fatalf("new file content = %q", gotNew)
	}
	gotOrig, err := os.ReadFile(orig)
	if err != nil {
		t.Fatalf("read orig: %v", err)
	}
	if string(gotOrig) != "package p\n\nfunc Keep() {}\n" {
		t.Fatalf("orig content = %q", gotOrig)
	}
}

func TestApplyDeleteFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "gone.go")
	if err := os.WriteFile(target, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileOps: []edit.FileOperation{{Kind: edit.FileOpDelete, Path: target}},
	}
	if _, err := edit.ApplyWithin(we, dir); err != nil {
		t.Fatalf("ApplyWithin: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected target deleted, stat err = %v", err)
	}
}

func TestApplyRenameFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.go")
	dst := filepath.Join(dir, "new.go")
	if err := os.WriteFile(src, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileOps: []edit.FileOperation{{Kind: edit.FileOpRename, Path: src, NewPath: dst}},
	}
	if _, err := edit.ApplyWithin(we, dir); err != nil {
		t.Fatalf("ApplyWithin: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected src gone, stat err = %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "package p\n" {
		t.Fatalf("dst content = %q", got)
	}
}

// TestApplyFileOpsRollbackMidBatch verifies all-or-nothing: when a later
// operation fails, every earlier applied step (a created file and an edited
// file) is rolled back.
func TestApplyFileOpsRollbackMidBatch(t *testing.T) {
	dir := t.TempDir()
	created := filepath.Join(dir, "created.go")
	existing := filepath.Join(dir, "existing.go")
	if err := os.WriteFile(existing, []byte("package p\nvar X = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileOps: []edit.FileOperation{
			{Kind: edit.FileOpCreate, Path: created},
			// Delete of a non-existent file without IgnoreIfNotExists fails,
			// and runs last (after create + text edit), forcing a rollback.
			{Kind: edit.FileOpDelete, Path: filepath.Join(dir, "missing.go")},
		},
		FileEdits: []edit.FileEdit{
			{
				Path: existing,
				Edits: []edit.TextEdit{
					{Range: edit.Range{Start: edit.Position{Line: 1, Character: 8}, End: edit.Position{Line: 1, Character: 9}}, NewText: "42"},
				},
			},
		},
	}

	if _, err := edit.ApplyWithin(we, dir); err == nil {
		t.Fatal("expected ApplyWithin to fail on delete of missing file")
	}

	// The created file must be rolled back (removed).
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("expected created file rolled back, stat err = %v", err)
	}
	// The existing file must be restored to its original content.
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read existing: %v", err)
	}
	if string(got) != "package p\nvar X = 1\n" {
		t.Fatalf("existing not rolled back: %q", got)
	}
}

func TestApplyCreateExistingFileWithoutOverwriteFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.go")
	if err := os.WriteFile(target, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileOps: []edit.FileOperation{{Kind: edit.FileOpCreate, Path: target}},
	}
	if _, err := edit.ApplyWithin(we, dir); err == nil {
		t.Fatal("expected create of existing file without overwrite to fail")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "package p\n" {
		t.Fatalf("existing file must be untouched, got %q", got)
	}
}

func TestApplyCreateOverwriteRollback(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.go")
	if err := os.WriteFile(target, []byte("original content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileOps: []edit.FileOperation{
			{Kind: edit.FileOpCreate, Path: target, Overwrite: true},
			// Trailing failing op (delete of a missing file) forces rollback
			// after the overwrite has truncated the existing file.
			{Kind: edit.FileOpDelete, Path: filepath.Join(dir, "missing.go")},
		},
	}
	if _, err := edit.ApplyWithin(we, dir); err == nil {
		t.Fatal("expected batch to fail on delete of missing file")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "original content\n" {
		t.Fatalf("create-overwrite not rolled back: %q", got)
	}
}

func TestApplyRenameOverwriteRollback(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.go")
	dst := filepath.Join(dir, "dst.go")
	if err := os.WriteFile(src, []byte("src content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("dst content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileOps: []edit.FileOperation{
			{Kind: edit.FileOpRename, Path: src, NewPath: dst, Overwrite: true},
			{Kind: edit.FileOpDelete, Path: filepath.Join(dir, "missing.go")},
		},
	}
	if _, err := edit.ApplyWithin(we, dir); err == nil {
		t.Fatal("expected batch to fail on delete of missing file")
	}
	gotSrc, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read src after rollback: %v", err)
	}
	if string(gotSrc) != "src content\n" {
		t.Fatalf("rename src not restored: %q", gotSrc)
	}
	gotDst, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst after rollback: %v", err)
	}
	if string(gotDst) != "dst content\n" {
		t.Fatalf("rename-overwrite dst not restored: %q", gotDst)
	}
}
