package edit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestApply_SingleFileRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	content := "package foo\n\nfunc oldName() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: path,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 2, Character: 5},
							End:   edit.Position{Line: 2, Character: 12},
						},
						NewText: "newName",
					},
				},
			},
		},
	}

	result, err := edit.Apply(we)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result.FilesModified != 1 {
		t.Errorf("expected FilesModified=1, got %d", result.FilesModified)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package foo\n\nfunc newName() {}\n"
	if string(got) != want {
		t.Errorf("file content mismatch\ngot:  %q\nwant: %q", string(got), want)
	}
}

func TestApply_MultipleEditsReverseOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	content := "aaa bbb ccc\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: path,
				Edits: []edit.TextEdit{
					// "bbb" at line 0, char 4-7
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 4},
							End:   edit.Position{Line: 0, Character: 7},
						},
						NewText: "xxx",
					},
					// "ccc" at line 0, char 8-11
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 8},
							End:   edit.Position{Line: 0, Character: 11},
						},
						NewText: "yyy",
					},
				},
			},
		},
	}

	result, err := edit.Apply(we)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result.FilesModified != 1 {
		t.Errorf("expected FilesModified=1, got %d", result.FilesModified)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "aaa xxx yyy\n"
	if string(got) != want {
		t.Errorf("file content mismatch\ngot:  %q\nwant: %q", string(got), want)
	}
}

func TestApply_MultiFileEdit(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "a.go")
	path2 := filepath.Join(dir, "b.go")
	if err := os.WriteFile(path1, []byte("type Foo struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path2, []byte("var f Foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: path1,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 8},
						},
						NewText: "Bar",
					},
				},
			},
			{
				Path: path2,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 6},
							End:   edit.Position{Line: 0, Character: 9},
						},
						NewText: "Bar",
					},
				},
			},
		},
	}

	result, err := edit.Apply(we)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result.FilesModified != 2 {
		t.Errorf("expected FilesModified=2, got %d", result.FilesModified)
	}

	got1, _ := os.ReadFile(path1)
	got2, _ := os.ReadFile(path2)
	if string(got1) != "type Bar struct{}\n" {
		t.Errorf("file1 content mismatch: %q", string(got1))
	}
	if string(got2) != "var f Bar\n" {
		t.Errorf("file2 content mismatch: %q", string(got2))
	}
}

func TestApply_RollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "real.go")
	original := "func real() {}\n"
	if err := os.WriteFile(realPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	nonExistent := filepath.Join(dir, "does_not_exist.go")

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: realPath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 9},
						},
						NewText: "modified",
					},
				},
			},
			{
				Path: nonExistent,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 0},
							End:   edit.Position{Line: 0, Character: 1},
						},
						NewText: "x",
					},
				},
			},
		},
	}

	_, err := edit.Apply(we)
	if err == nil {
		t.Fatal("expected Apply to return error for nonexistent file, got nil")
	}

	// The real file must be unchanged.
	got, err2 := os.ReadFile(realPath)
	if err2 != nil {
		t.Fatal(err2)
	}
	if string(got) != original {
		t.Errorf("rollback failed: file was modified\ngot:  %q\nwant: %q", string(got), original)
	}
}

func TestApply_DuplicateFileEditDoesNotPartiallyCommit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.go")
	original := "func oldName() {}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: path,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 12},
						},
						NewText: "first",
					},
				},
			},
			{
				Path: path,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 12},
						},
						NewText: "second",
					},
				},
			},
		},
	}

	_, err := edit.Apply(we)
	if err == nil {
		t.Fatal("expected Apply to reject duplicate file edits, got nil")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("duplicate file edit partially committed\ngot:  %q\nwant: %q", string(got), original)
	}
}
