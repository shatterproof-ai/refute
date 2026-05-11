package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestDoctorCommand_JSONShape(t *testing.T) {
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"doctor", "--json"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("doctor --json: %v", err)
	}

	var got DoctorReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, buf.String())
	}

	if got.SchemaVersion != edit.SchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", got.SchemaVersion, edit.SchemaVersion)
	}
	if got.Command != "doctor" {
		t.Errorf("command = %q, want \"doctor\"", got.Command)
	}

	wantLangs := map[string]bool{
		"go":         false,
		"typescript": false,
		"javascript": false,
		"rust":       false,
		"python":     false,
		"java":       false,
		"kotlin":     false,
	}
	for _, b := range got.Backends {
		if _, ok := wantLangs[b.Language]; ok {
			wantLangs[b.Language] = true
		}
	}
	for lang, seen := range wantLangs {
		if !seen {
			t.Errorf("doctor report missing language %q", lang)
		}
	}
}

func TestDoctorCommand_HumanShape(t *testing.T) {
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"doctor"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("doctor: %v", err)
	}

	out := buf.String()
	for _, lang := range []string{"go", "typescript", "rust", "java", "kotlin"} {
		if !strings.Contains(out, lang) {
			t.Errorf("human-readable doctor output missing language %q\n%s", lang, out)
		}
	}
}

func TestDoctor_GoStatusReflectsLookPath(t *testing.T) {
	origLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = origLookPath })

	lookPathFn = func(name string) (string, error) {
		if name == "gopls" {
			return "/fake/path/to/gopls", nil
		}
		return "", errLookPathNotFound
	}

	report := buildDoctorReport()
	var goEntry *DoctorBackendStatus
	for i := range report.Backends {
		if report.Backends[i].Language == "go" {
			goEntry = &report.Backends[i]
			break
		}
	}
	if goEntry == nil {
		t.Fatal("doctor report missing go entry")
	}
	if goEntry.Status != DoctorStatusOK {
		t.Errorf("go status = %q with gopls on PATH, want %q", goEntry.Status, DoctorStatusOK)
	}
	if goEntry.Binary != "/fake/path/to/gopls" {
		t.Errorf("go binary = %q, want /fake/path/to/gopls", goEntry.Binary)
	}
}

func TestDoctor_RustOperationsMatchSupportMatrix(t *testing.T) {
	report := buildDoctorReport()
	var rustEntry *DoctorBackendStatus
	for i := range report.Backends {
		if report.Backends[i].Language == "rust" {
			rustEntry = &report.Backends[i]
			break
		}
	}
	if rustEntry == nil {
		t.Fatal("doctor report missing rust entry")
	}

	got := strings.Join(rustEntry.Operations, ", ")
	want := strings.Join(supportMatrixOperations(t, "Rust"), ", ")
	if got != want {
		t.Errorf("rust operations = %q, want support matrix operations %q", got, want)
	}
}

func supportMatrixOperations(t *testing.T, language string) []string {
	t.Helper()

	data, err := os.ReadFile("../../docs/support-matrix.md")
	if err != nil {
		t.Fatalf("read support matrix: %v", err)
	}

	prefix := "| " + language + " |"
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 6 {
			t.Fatalf("support matrix row for %s has too few columns: %q", language, line)
		}
		rawOps := strings.Split(strings.TrimSpace(cells[5]), ",")
		ops := make([]string, 0, len(rawOps))
		for _, op := range rawOps {
			ops = append(ops, strings.TrimSpace(op))
		}
		return ops
	}

	t.Fatalf("support matrix missing language %s", language)
	return nil
}
