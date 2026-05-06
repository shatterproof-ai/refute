package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// DoctorStatus values describe the readiness of a backend on the current host.
const (
	DoctorStatusOK           = "ok"
	DoctorStatusMissing      = "missing"
	DoctorStatusExperimental = "experimental"
	DoctorStatusPlanned      = "planned"
	DoctorStatusNotClaimed   = "not-claimed"
)

// DoctorBackendStatus is one row of the doctor report.
type DoctorBackendStatus struct {
	Language          string   `json:"language"`
	Backend           string   `json:"backend"`
	Status            string   `json:"status"`
	Binary            string   `json:"binary,omitempty"`
	Operations        []string `json:"operations,omitempty"`
	MissingDependency string   `json:"missingDependency,omitempty"`
	InstallHint       string   `json:"installHint,omitempty"`
	Caveats           string   `json:"caveats,omitempty"`
}

// DoctorReport is the JSON shape of `refute doctor --json`.
type DoctorReport struct {
	SchemaVersion string                `json:"schemaVersion"`
	Command       string                `json:"command"`
	Backends      []DoctorBackendStatus `json:"backends"`
}

// errLookPathNotFound is returned by the test fake when a binary is intentionally
// reported as missing.
var errLookPathNotFound = errors.New("not found")

// lookPathFn is exec.LookPath in production and is overridden in tests.
var lookPathFn = exec.LookPath

var goOperations = []string{"rename", "extract-function", "extract-variable", "inline"}

func buildDoctorReport() DoctorReport {
	report := DoctorReport{
		SchemaVersion: edit.SchemaVersion,
		Command:       "doctor",
	}
	report.Backends = []DoctorBackendStatus{
		probeLSP("go", "lsp/gopls", "gopls", DoctorStatusOK,
			"go install golang.org/x/tools/gopls@latest", goOperations,
			"Primary v0.1 target."),
		probeLSP("typescript", "lsp/typescript-language-server", "typescript-language-server", DoctorStatusExperimental,
			"npm install -g typescript-language-server typescript", []string{"rename"},
			"TypeScript support is experimental for v0.1; ts-morph adapter is not packaged."),
		probeLSP("javascript", "lsp/typescript-language-server", "typescript-language-server", DoctorStatusExperimental,
			"npm install -g typescript-language-server typescript", []string{"rename"},
			"JavaScript support is experimental for v0.1."),
		probeLSP("rust", "lsp/rust-analyzer", "rust-analyzer", DoctorStatusExperimental,
			"rustup component add rust-analyzer", []string{"rename"},
			"Rust support is experimental for v0.1; CI exercises rename coverage when rust-analyzer is installed."),
		probeLSP("python", "lsp/pyright", "pyright-langserver", DoctorStatusPlanned,
			"npm install -g pyright", []string{"rename"},
			"Python support is planned, not yet covered by integration tests."),
		{
			Language: "java",
			Backend:  "openrewrite",
			Status:   DoctorStatusNotClaimed,
			Caveats:  "Java/OpenRewrite support is not claimed for v0.1.",
		},
		{
			Language: "kotlin",
			Backend:  "openrewrite",
			Status:   DoctorStatusNotClaimed,
			Caveats:  "Kotlin/OpenRewrite support is not claimed for v0.1.",
		},
	}
	return report
}

// probeLSP looks up the named binary on PATH. okStatus is the status returned
// when the binary is present (callers pick "ok" or "experimental" to reflect
// the matrix). On miss, the row carries DoctorStatusMissing and the install
// hint.
func probeLSP(language, backend, binary, okStatus, installHint string, ops []string, caveats string) DoctorBackendStatus {
	row := DoctorBackendStatus{
		Language:    language,
		Backend:     backend,
		Operations:  ops,
		InstallHint: installHint,
		Caveats:     caveats,
	}
	path, err := lookPathFn(binary)
	if err != nil {
		row.Status = DoctorStatusMissing
		row.MissingDependency = binary
		return row
	}
	row.Status = okStatus
	row.Binary = path
	return row
}

func init() {
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report which language backends are installed and ready to use",
		Long: `doctor inspects the local environment for language servers and adapter
assets that refute relies on. It reports which languages are ready, which
need a missing dependency installed, and which are not claimed for the
current release.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := buildDoctorReport()
			out := cmd.OutOrStdout()
			if flagJSON {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("marshalling doctor report: %w", err)
				}
				fmt.Fprintln(out, string(data))
				return nil
			}
			renderDoctorHuman(out, report)
			return nil
		},
	}
	doctorCmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	RootCmd.AddCommand(doctorCmd)
}

func renderDoctorHuman(w interface{ Write(p []byte) (int, error) }, report DoctorReport) {
	for _, b := range report.Backends {
		line := fmt.Sprintf("%-12s %-40s %s\n", b.Language, b.Backend, b.Status)
		_, _ = w.Write([]byte(line))
		if b.Binary != "" {
			_, _ = w.Write([]byte(fmt.Sprintf("  binary: %s\n", b.Binary)))
		}
		if len(b.Operations) > 0 && b.Status != DoctorStatusNotClaimed {
			_, _ = w.Write([]byte(fmt.Sprintf("  operations: %s\n", joinOps(b.Operations))))
		}
		if b.Status == DoctorStatusMissing && b.InstallHint != "" {
			_, _ = w.Write([]byte(fmt.Sprintf("  install: %s\n", b.InstallHint)))
		}
		if b.Caveats != "" {
			_, _ = w.Write([]byte(fmt.Sprintf("  note: %s\n", b.Caveats)))
		}
	}
}

func joinOps(ops []string) string {
	out := ""
	for i, op := range ops {
		if i > 0 {
			out += ", "
		}
		out += op
	}
	return out
}
