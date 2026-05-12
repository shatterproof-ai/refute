package edit_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	result, err := edit.ApplyWithin(we, dir)
	if err != nil {
		t.Fatalf("ApplyWithin failed: %v", err)
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

	result, err := edit.ApplyWithin(we, dir)
	if err != nil {
		t.Fatalf("ApplyWithin failed: %v", err)
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

	result, err := edit.ApplyWithin(we, dir)
	if err != nil {
		t.Fatalf("ApplyWithin failed: %v", err)
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

	_, err := edit.ApplyWithin(we, dir)
	if err == nil {
		t.Fatal("expected ApplyWithin to return error for nonexistent file, got nil")
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

	_, err := edit.ApplyWithin(we, dir)
	if err == nil {
		t.Fatal("expected ApplyWithin to reject duplicate file edits, got nil")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("duplicate file edit partially committed\ngot:  %q\nwant: %q", string(got), original)
	}
}

func TestApplyWithin_RejectsPathOutsideWorkspaceBeforeWrites(t *testing.T) {
	workspace := t.TempDir()
	insidePath := filepath.Join(workspace, "inside.go")
	insideOriginal := "func inside() {}\n"
	if err := os.WriteFile(insidePath, []byte(insideOriginal), 0o644); err != nil {
		t.Fatal(err)
	}

	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.go")
	outsideOriginal := "func outside() {}\n"
	if err := os.WriteFile(outsidePath, []byte(outsideOriginal), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: insidePath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 11},
						},
						NewText: "renamedInside",
					},
				},
			},
			{
				Path: outsidePath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 12},
						},
						NewText: "renamedOutside",
					},
				},
			},
		},
	}

	_, err := edit.ApplyWithin(we, workspace)
	if err == nil {
		t.Fatal("expected ApplyWithin to reject outside workspace path, got nil")
	}
	if !strings.Contains(err.Error(), "outside workspace") || !strings.Contains(err.Error(), outsidePath) || !strings.Contains(err.Error(), workspace) {
		t.Fatalf("expected actionable outside-workspace error, got %v", err)
	}

	gotInside, err := os.ReadFile(insidePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotInside) != insideOriginal {
		t.Errorf("inside file was modified before validation completed\ngot:  %q\nwant: %q", string(gotInside), insideOriginal)
	}

	gotOutside, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotOutside) != outsideOriginal {
		t.Errorf("outside file was modified\ngot:  %q\nwant: %q", string(gotOutside), outsideOriginal)
	}
}

func TestApplyWithin_RejectsSymlinkTargetOutsideWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated permissions on some Windows setups")
	}

	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "target.go")
	original := "func outside() {}\n"
	if err := os.WriteFile(outsidePath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(workspace, "link.go")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: linkPath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 12},
						},
						NewText: "renamedOutside",
					},
				},
			},
		},
	}

	_, err := edit.ApplyWithin(we, workspace)
	if err == nil {
		t.Fatal("expected ApplyWithin to reject symlink target outside workspace, got nil")
	}
	if !strings.Contains(err.Error(), "outside workspace") || !strings.Contains(err.Error(), linkPath) || !strings.Contains(err.Error(), outsidePath) {
		t.Fatalf("expected symlink target in outside-workspace error, got %v", err)
	}

	got, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("outside symlink target was modified\ngot:  %q\nwant: %q", string(got), original)
	}
}

func TestApplyWithin_ResolvesSymlinkTargetInsideWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated permissions on some Windows setups")
	}

	workspace := t.TempDir()
	targetPath := filepath.Join(workspace, "target.go")
	if err := os.WriteFile(targetPath, []byte("func target() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(workspace, "link.go")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: linkPath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 11},
						},
						NewText: "renamedTarget",
					},
				},
			},
		},
	}

	result, err := edit.ApplyWithin(we, workspace)
	if err != nil {
		t.Fatalf("ApplyWithin failed: %v", err)
	}
	if result.FilesModified != 1 {
		t.Errorf("expected FilesModified=1, got %d", result.FilesModified)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "func renamedTarget() {}\n"
	if string(got) != want {
		t.Errorf("symlink target content mismatch\ngot:  %q\nwant: %q", string(got), want)
	}
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected symlink path to remain present: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink path to remain a symlink, mode is %v", info.Mode())
	}
}

func TestApply_OverlappingEditsRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlap.go")
	original := "abcdef\n"
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
							Start: edit.Position{Line: 0, Character: 0},
							End:   edit.Position{Line: 0, Character: 3},
						},
						NewText: "X",
					},
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 2},
							End:   edit.Position{Line: 0, Character: 5},
						},
						NewText: "Y",
					},
				},
			},
		},
	}

	_, err := edit.ApplyWithin(we, dir)
	if err == nil {
		t.Fatal("expected ApplyWithin to reject overlapping edits, got nil")
	}
	if !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlap error, got %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("file was modified despite overlap rejection\ngot:  %q\nwant: %q", string(got), original)
	}
}

func TestApply_AdjacentNonOverlappingEditsAllowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adjacent.go")
	if err := os.WriteFile(path, []byte("abcdef\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: path,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 0},
							End:   edit.Position{Line: 0, Character: 3},
						},
						NewText: "X",
					},
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 3},
							End:   edit.Position{Line: 0, Character: 6},
						},
						NewText: "Y",
					},
				},
			},
		},
	}

	if _, err := edit.ApplyWithin(we, dir); err != nil {
		t.Fatalf("ApplyWithin rejected adjacent non-overlapping edits: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "XY\n" {
		t.Errorf("adjacent edits result mismatch\ngot:  %q\nwant: %q", string(got), "XY\n")
	}
}

func TestApply_PreservesCRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf.txt")
	original := "first line\r\nsecond line\r\nthird line\r\n"
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
							Start: edit.Position{Line: 1, Character: 0},
							End:   edit.Position{Line: 1, Character: 6},
						},
						NewText: "SECOND",
					},
				},
			},
		},
	}

	if _, err := edit.ApplyWithin(we, dir); err != nil {
		t.Fatalf("ApplyWithin failed on CRLF content: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "first line\r\nSECOND line\r\nthird line\r\n"
	if string(got) != want {
		t.Errorf("CRLF content mismatch\ngot:  %q\nwant: %q", string(got), want)
	}
}

func TestApply_RollbackCleansUpTempFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory write semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}

	workspace := t.TempDir()
	writableDir := filepath.Join(workspace, "writable")
	if err := os.Mkdir(writableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writablePath := filepath.Join(writableDir, "writable.go")
	writableOriginal := "func writable() {}\n"
	if err := os.WriteFile(writablePath, []byte(writableOriginal), 0o644); err != nil {
		t.Fatal(err)
	}

	readonlyDir := filepath.Join(workspace, "readonly")
	if err := os.Mkdir(readonlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	readonlyPath := filepath.Join(readonlyDir, "readonly.go")
	readonlyOriginal := "func readonly() {}\n"
	if err := os.WriteFile(readonlyPath, []byte(readonlyOriginal), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readonlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonlyDir, 0o755) })

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: writablePath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 13},
						},
						NewText: "renamed",
					},
				},
			},
			{
				Path: readonlyPath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 13},
						},
						NewText: "renamed",
					},
				},
			},
		},
	}

	if _, err := edit.ApplyWithin(we, workspace); err == nil {
		t.Fatal("expected ApplyWithin to fail when a target dir is read-only")
	}

	gotWritable, err := os.ReadFile(writablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotWritable) != writableOriginal {
		t.Errorf("writable file was modified despite later failure\ngot:  %q\nwant: %q", string(gotWritable), writableOriginal)
	}

	entries, err := os.ReadDir(writableDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".refute.tmp") || strings.Contains(e.Name(), ".refute.bak") {
			t.Errorf("leftover scratch file in writable dir: %s", e.Name())
		}
	}
}

func TestApplyWithin_SymlinkInsideWorkspaceLeavesLinkIntact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated permissions on some Windows setups")
	}

	workspace := t.TempDir()
	targetPath := filepath.Join(workspace, "target.go")
	if err := os.WriteFile(targetPath, []byte("func target() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(workspace, "link.go")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: linkPath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 11},
						},
						NewText: "renamed",
					},
				},
			},
		},
	}

	if _, err := edit.ApplyWithin(we, workspace); err != nil {
		t.Fatalf("ApplyWithin on symlinked path failed: %v", err)
	}

	gotTarget, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotTarget) != "func renamed() {}\n" {
		t.Errorf("symlink target content mismatch: %q", string(gotTarget))
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected link path to remain present: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected link path to remain a symlink, mode is %v", info.Mode())
	}
}

func TestApplyWithin_AcceptsPathsUnderSymlinkedWorkspaceRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated permissions on some Windows setups")
	}

	realWorkspace := t.TempDir()
	parent := t.TempDir()
	workspaceLink := filepath.Join(parent, "workspace-link")
	if err := os.Symlink(realWorkspace, workspaceLink); err != nil {
		t.Fatal(err)
	}

	pathViaLink := filepath.Join(workspaceLink, "target.go")
	if err := os.WriteFile(pathViaLink, []byte("func target() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: pathViaLink,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 11},
						},
						NewText: "renamedTarget",
					},
				},
			},
		},
	}

	_, err := edit.ApplyWithin(we, workspaceLink)
	if err != nil {
		t.Fatalf("ApplyWithin rejected path under symlinked workspace root: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(realWorkspace, "target.go"))
	if err != nil {
		t.Fatal(err)
	}
	want := "func renamedTarget() {}\n"
	if string(got) != want {
		t.Errorf("content mismatch through symlinked root\ngot:  %q\nwant: %q", string(got), want)
	}
}
