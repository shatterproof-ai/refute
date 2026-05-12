//go:build integration

package internal_test

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pmezard/go-difflib/difflib"
)

var updateGolden = flag.Bool("update", false, "update golden expected output")

func TestGolden(t *testing.T) {
	goldenRoot := filepath.Join("..", "testdata", "golden")
	entries, err := os.ReadDir(goldenRoot)
	if err != nil {
		t.Fatalf("read golden root: %v", err)
	}

	refuteBin := buildRefute(t)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			caseDir := filepath.Join(goldenRoot, name)
			for _, tool := range readGoldenRequirements(t, caseDir) {
				if _, err := exec.LookPath(tool); err != nil {
					t.Skipf("%s not found on PATH", tool)
				}
			}

			tmpDir := t.TempDir()
			copyDir(t, filepath.Join(caseDir, "input"), tmpDir)

			for _, line := range readGoldenCommands(t, filepath.Join(caseDir, "cmd.txt")) {
				args := strings.Fields(strings.ReplaceAll(line, "{{tmpdir}}", tmpDir))
				if len(args) == 0 {
					continue
				}
				if args[0] != "refute" {
					t.Fatalf("cmd.txt line must start with refute, got %q", args[0])
				}
				cmd := exec.Command(refuteBin, args[1:]...)
				cmd.Dir = tmpDir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s: %v\n%s", line, err, out)
				}
			}

			expectedDir := filepath.Join(caseDir, "expected")
			if *updateGolden {
				if err := replaceDir(expectedDir, tmpDir); err != nil {
					t.Fatalf("update expected: %v", err)
				}
				return
			}

			if diffs, err := diffDirs(expectedDir, tmpDir); err != nil {
				t.Fatal(err)
			} else if len(diffs) > 0 {
				t.Fatalf("golden output mismatch:\n%s", strings.Join(diffs, "\n"))
			}
		})
	}
}

func readGoldenRequirements(t *testing.T, caseDir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(caseDir, "requires.txt"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read requires.txt: %v", err)
	}
	return meaningfulLines(string(data))
}

func readGoldenCommands(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cmd.txt: %v", err)
	}
	lines := meaningfulLines(string(data))
	if len(lines) == 0 {
		t.Fatalf("cmd.txt has no commands")
	}
	return lines
}

func meaningfulLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func diffDirs(expectedDir, actualDir string) ([]string, error) {
	expected, err := readTree(expectedDir)
	if err != nil {
		return nil, fmt.Errorf("read expected tree: %w", err)
	}
	actual, err := readTree(actualDir)
	if err != nil {
		return nil, fmt.Errorf("read actual tree: %w", err)
	}

	paths := make([]string, 0, len(expected)+len(actual))
	seen := make(map[string]bool, len(expected)+len(actual))
	for path := range expected {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range actual {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)

	var diffs []string
	for _, path := range paths {
		want, hasWant := expected[path]
		got, hasGot := actual[path]
		switch {
		case !hasWant:
			diffs = append(diffs, fmt.Sprintf("unexpected file %s", path))
		case !hasGot:
			diffs = append(diffs, fmt.Sprintf("missing file %s", path))
		case string(want) != string(got):
			diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
				A:        difflib.SplitLines(string(want)),
				B:        difflib.SplitLines(string(got)),
				FromFile: filepath.Join("expected", path),
				ToFile:   filepath.Join("actual", path),
				Context:  3,
			})
			if err != nil {
				return nil, err
			}
			diffs = append(diffs, diff)
		}
	}
	return diffs, nil
}

func readTree(root string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	return files, err
}

func replaceDir(dst, src string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return copyTree(dst, src)
}

func copyTree(dst, src string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
