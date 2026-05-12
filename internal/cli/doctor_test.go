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

func TestDoctor_BackendStatusesReflectSupportAndDependencies(t *testing.T) {
	origLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = origLookPath })

	presentBinaries := map[string]string{
		"gopls":                      "/fake/bin/gopls",
		"typescript-language-server": "/fake/bin/typescript-language-server",
		"rust-analyzer":              "/fake/bin/rust-analyzer",
		"pyright-langserver":         "/fake/bin/pyright-langserver",
	}

	tests := []struct {
		name              string
		availableBinaries map[string]string
		want              []DoctorBackendStatus
	}{
		{
			name:              "dependencies present",
			availableBinaries: presentBinaries,
			want: []DoctorBackendStatus{
				{
					Language:    "go",
					Status:      DoctorStatusOK,
					Binary:      "/fake/bin/gopls",
					InstallHint: "go install golang.org/x/tools/gopls@latest",
				},
				{
					Language:    "typescript",
					Status:      DoctorStatusExperimental,
					Binary:      "/fake/bin/typescript-language-server",
					InstallHint: "npm install -g typescript-language-server typescript",
				},
				{
					Language:    "javascript",
					Status:      DoctorStatusExperimental,
					Binary:      "/fake/bin/typescript-language-server",
					InstallHint: "npm install -g typescript-language-server typescript",
				},
				{
					Language:    "rust",
					Status:      DoctorStatusExperimental,
					Binary:      "/fake/bin/rust-analyzer",
					InstallHint: "rustup component add rust-analyzer",
				},
				{
					Language:    "python",
					Status:      DoctorStatusPlanned,
					Binary:      "/fake/bin/pyright-langserver",
					InstallHint: "npm install -g pyright",
				},
				{
					Language: "java",
					Status:   DoctorStatusNotClaimed,
				},
				{
					Language: "kotlin",
					Status:   DoctorStatusNotClaimed,
				},
			},
		},
		{
			name:              "dependencies missing",
			availableBinaries: map[string]string{},
			want: []DoctorBackendStatus{
				{
					Language:          "go",
					Status:            DoctorStatusMissing,
					MissingDependency: "gopls",
					InstallHint:       "go install golang.org/x/tools/gopls@latest",
				},
				{
					Language:          "typescript",
					Status:            DoctorStatusMissing,
					MissingDependency: "typescript-language-server",
					InstallHint:       "npm install -g typescript-language-server typescript",
				},
				{
					Language:          "javascript",
					Status:            DoctorStatusMissing,
					MissingDependency: "typescript-language-server",
					InstallHint:       "npm install -g typescript-language-server typescript",
				},
				{
					Language:          "rust",
					Status:            DoctorStatusMissing,
					MissingDependency: "rust-analyzer",
					InstallHint:       "rustup component add rust-analyzer",
				},
				{
					Language:          "python",
					Status:            DoctorStatusMissing,
					MissingDependency: "pyright-langserver",
					InstallHint:       "npm install -g pyright",
				},
				{
					Language: "java",
					Status:   DoctorStatusNotClaimed,
				},
				{
					Language: "kotlin",
					Status:   DoctorStatusNotClaimed,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookPathFn = func(name string) (string, error) {
				path, ok := tt.availableBinaries[name]
				if !ok {
					return "", errLookPathNotFound
				}
				return path, nil
			}

			report := buildDoctorReport()
			for _, want := range tt.want {
				got := doctorBackendByLanguage(t, report, want.Language)
				if got.Status != want.Status {
					t.Errorf("%s status = %q, want %q", want.Language, got.Status, want.Status)
				}
				if got.Binary != want.Binary {
					t.Errorf("%s binary = %q, want %q", want.Language, got.Binary, want.Binary)
				}
				if got.MissingDependency != want.MissingDependency {
					t.Errorf("%s missing dependency = %q, want %q", want.Language, got.MissingDependency, want.MissingDependency)
				}
				if got.InstallHint != want.InstallHint {
					t.Errorf("%s install hint = %q, want %q", want.Language, got.InstallHint, want.InstallHint)
				}
			}
		})
	}
}

func TestDoctor_RustOperationsMatchSupportMatrix(t *testing.T) {
	report := buildDoctorReport()
	rustEntry := doctorBackendByLanguage(t, report, "rust")

	got := strings.Join(rustEntry.Operations, ", ")
	want := strings.Join(supportMatrixOperations(t, "Rust"), ", ")
	if got != want {
		t.Errorf("rust operations = %q, want support matrix operations %q", got, want)
	}
}

func doctorBackendByLanguage(t *testing.T, report DoctorReport, language string) DoctorBackendStatus {
	t.Helper()

	for _, backend := range report.Backends {
		if backend.Language == language {
			return backend
		}
	}

	t.Fatalf("doctor report missing %s entry", language)
	return DoctorBackendStatus{}
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
