package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/config"
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

	if got.SchemaVersion != doctorSchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", got.SchemaVersion, doctorSchemaVersion)
	}
	if got.SchemaVersion == "" {
		t.Error("doctor report must populate schemaVersion")
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
	origTSAdapter := tsAdapterAvailableFn
	origConfig := doctorConfigFn
	origProbe := versionProbeFn
	t.Cleanup(func() {
		lookPathFn = origLookPath
		tsAdapterAvailableFn = origTSAdapter
		doctorConfigFn = origConfig
		versionProbeFn = origProbe
	})

	// Pin doctor to built-in defaults so the assertions are independent of any
	// user/project config on the host running the suite.
	doctorConfigFn = func() *config.Config {
		return &config.Config{Servers: map[string]config.ServerConfig{}}
	}
	// Stub version probing so the test never shells out to a real server.
	versionProbeFn = func(command string, args []string) string { return "" }

	presentBinaries := map[string]string{
		"gopls":                      "/fake/bin/gopls",
		"typescript-language-server": "/fake/bin/typescript-language-server",
		"rust-analyzer":              "/fake/bin/rust-analyzer",
		"pyright-langserver":         "/fake/bin/pyright-langserver",
	}

	tests := []struct {
		name              string
		tsAdapterPresent  bool
		availableBinaries map[string]string
		want              []DoctorBackendStatus
	}{
		{
			name:              "dependencies present",
			tsAdapterPresent:  true,
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
					Backend:     "tsmorph",
					Status:      DoctorStatusOK,
					InstallHint: "npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz",
				},
				{
					Language:    "typescript",
					Backend:     "lsp/typescript-language-server",
					Status:      DoctorStatusExperimental,
					Binary:      "/fake/bin/typescript-language-server",
					InstallHint: "npm install -g typescript-language-server typescript",
				},
				{
					Language:    "javascript",
					Backend:     "tsmorph",
					Status:      DoctorStatusOK,
					InstallHint: "npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz",
				},
				{
					Language:    "javascript",
					Backend:     "lsp/typescript-language-server",
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
			tsAdapterPresent:  false,
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
					Backend:           "tsmorph",
					Status:            DoctorStatusMissing,
					MissingDependency: "@shatterproof-ai/refute-ts-adapter",
					InstallHint:       "npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz",
				},
				{
					Language:          "typescript",
					Backend:           "lsp/typescript-language-server",
					Status:            DoctorStatusMissing,
					MissingDependency: "typescript-language-server",
					InstallHint:       "npm install -g typescript-language-server typescript",
				},
				{
					Language:          "javascript",
					Backend:           "tsmorph",
					Status:            DoctorStatusMissing,
					MissingDependency: "@shatterproof-ai/refute-ts-adapter",
					InstallHint:       "npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz",
				},
				{
					Language:          "javascript",
					Backend:           "lsp/typescript-language-server",
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
			adapterPresent := tt.tsAdapterPresent
			tsAdapterAvailableFn = func() bool { return adapterPresent }
			lookPathFn = func(name string) (string, error) {
				path, ok := tt.availableBinaries[name]
				if !ok {
					return "", errLookPathNotFound
				}
				return path, nil
			}

			report := buildDoctorReport()
			for _, want := range tt.want {
				var got DoctorBackendStatus
				if want.Backend != "" {
					got = doctorBackendByLangAndBackend(t, report, want.Language, want.Backend)
				} else {
					got = doctorBackendByLanguage(t, report, want.Language)
				}
				label := want.Language
				if want.Backend != "" {
					label += "/" + want.Backend
				}
				if got.Status != want.Status {
					t.Errorf("%s status = %q, want %q", label, got.Status, want.Status)
				}
				if got.Binary != want.Binary {
					t.Errorf("%s binary = %q, want %q", label, got.Binary, want.Binary)
				}
				if got.MissingDependency != want.MissingDependency {
					t.Errorf("%s missing dependency = %q, want %q", label, got.MissingDependency, want.MissingDependency)
				}
				if got.InstallHint != want.InstallHint {
					t.Errorf("%s install hint = %q, want %q", label, got.InstallHint, want.InstallHint)
				}
			}
		})
	}
}

// TestDoctor_JavaScriptTSMorphRow verifies doctor surfaces JavaScript rename
// support through the ts-morph adapter as its own row, distinct from the
// JavaScript language-server fallback row, in both the JSON report and the
// human-readable output.
func TestDoctor_JavaScriptTSMorphRow(t *testing.T) {
	origTSAdapter := tsAdapterAvailableFn
	t.Cleanup(func() { tsAdapterAvailableFn = origTSAdapter })
	tsAdapterAvailableFn = func() bool { return true }

	report := buildDoctorReport()
	jsAdapter := doctorBackendByLangAndBackend(t, report, "javascript", "tsmorph")
	if jsAdapter.Status != DoctorStatusOK {
		t.Errorf("javascript/tsmorph status = %q, want %q", jsAdapter.Status, DoctorStatusOK)
	}
	if len(jsAdapter.Operations) != 1 || jsAdapter.Operations[0] != "rename" {
		t.Errorf("javascript/tsmorph operations = %v, want [rename]", jsAdapter.Operations)
	}

	// The adapter row must precede the JavaScript language-server fallback row so
	// the preferred backend is reported first.
	var adapterIdx, lspIdx = -1, -1
	for i, b := range report.Backends {
		if b.Language != "javascript" {
			continue
		}
		switch b.Backend {
		case "tsmorph":
			adapterIdx = i
		case "lsp/typescript-language-server":
			lspIdx = i
		}
	}
	if adapterIdx == -1 || lspIdx == -1 {
		t.Fatalf("javascript rows missing: adapterIdx=%d lspIdx=%d", adapterIdx, lspIdx)
	}
	if adapterIdx > lspIdx {
		t.Errorf("javascript/tsmorph row at %d should precede lsp row at %d", adapterIdx, lspIdx)
	}

	var buf bytes.Buffer
	renderDoctorHuman(&buf, report)
	var foundJSRow bool
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if strings.Contains(line, "javascript") && strings.Contains(line, "tsmorph") {
			foundJSRow = true
			break
		}
	}
	if !foundJSRow {
		t.Errorf("human output missing javascript tsmorph row:\n%s", buf.String())
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

// TestDoctor_ReflectsCustomServerConfig verifies that a custom language server
// configured in user/project config is probed (and reported found) instead of
// the built-in default binary, so custom servers are no longer mislabelled as
// missing.
func TestDoctor_ReflectsCustomServerConfig(t *testing.T) {
	origLookPath := lookPathFn
	origConfig := doctorConfigFn
	origProbe := versionProbeFn
	t.Cleanup(func() {
		lookPathFn = origLookPath
		doctorConfigFn = origConfig
		versionProbeFn = origProbe
	})

	doctorConfigFn = func() *config.Config {
		return &config.Config{
			Servers: map[string]config.ServerConfig{
				"go": {Command: "custom-gopls"},
			},
		}
	}
	lookPathFn = func(name string) (string, error) {
		if name == "custom-gopls" {
			return "/fake/bin/custom-gopls", nil
		}
		return "", errLookPathNotFound
	}
	versionProbeFn = func(command string, args []string) string { return "" }

	report := buildDoctorReport()
	goEntry := doctorBackendByLanguage(t, report, "go")

	if goEntry.Status != DoctorStatusOK {
		t.Errorf("go status = %q, want %q (custom server present)", goEntry.Status, DoctorStatusOK)
	}
	if goEntry.Binary != "/fake/bin/custom-gopls" {
		t.Errorf("go binary = %q, want custom server path", goEntry.Binary)
	}
	if goEntry.MissingDependency != "" {
		t.Errorf("go missing dependency = %q, want empty for present custom server", goEntry.MissingDependency)
	}
}

// TestDoctor_ReportsBackendVersion verifies that doctor probes and surfaces each
// present backend's version in both the JSON rows and the human-readable output.
func TestDoctor_ReportsBackendVersion(t *testing.T) {
	origLookPath := lookPathFn
	origConfig := doctorConfigFn
	origProbe := versionProbeFn
	origTSAdapter := tsAdapterAvailableFn
	t.Cleanup(func() {
		lookPathFn = origLookPath
		doctorConfigFn = origConfig
		versionProbeFn = origProbe
		tsAdapterAvailableFn = origTSAdapter
	})

	doctorConfigFn = func() *config.Config {
		return &config.Config{Servers: map[string]config.ServerConfig{}}
	}
	lookPathFn = func(name string) (string, error) {
		if name == "gopls" {
			return "/fake/bin/gopls", nil
		}
		return "", errLookPathNotFound
	}
	tsAdapterAvailableFn = func() bool { return false }
	versionProbeFn = func(command string, args []string) string {
		if command == "/fake/bin/gopls" {
			return "golang.org/x/tools/gopls v1.2.3"
		}
		return ""
	}

	report := buildDoctorReport()
	goEntry := doctorBackendByLanguage(t, report, "go")
	if goEntry.Version != "golang.org/x/tools/gopls v1.2.3" {
		t.Errorf("go version = %q, want probed version", goEntry.Version)
	}

	var buf bytes.Buffer
	renderDoctorHuman(&buf, report)
	if !strings.Contains(buf.String(), "version: golang.org/x/tools/gopls v1.2.3") {
		t.Errorf("human output missing version line:\n%s", buf.String())
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

func doctorBackendByLangAndBackend(t *testing.T, report DoctorReport, language, backendName string) DoctorBackendStatus {
	t.Helper()

	for _, b := range report.Backends {
		if b.Language == language && b.Backend == backendName {
			return b
		}
	}

	t.Fatalf("doctor report missing %s/%s entry", language, backendName)
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
