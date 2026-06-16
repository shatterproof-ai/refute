package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend/tsmorph"
	"github.com/shatterproof-ai/refute/internal/config"
)

// doctorSchemaVersion identifies the `refute doctor --json` report shape. It is
// intentionally distinct from edit.SchemaVersion (the operation-envelope
// schema): the two documents evolve independently, so they must not share a
// version number.
const doctorSchemaVersion = "1"

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

// tsAdapterAvailableFn checks ts-morph adapter availability without a
// workspace root (doctor context). Overridable in tests.
var tsAdapterAvailableFn = func() bool { return tsmorph.Available() }

// doctorConfigFn resolves the effective configuration doctor probes against, so
// that a custom server in refute.config.json is reported as found rather than
// missing. Overridable in tests.
var doctorConfigFn = loadDoctorConfig

// loadDoctorConfig loads user/project configuration relative to the current
// working directory. Doctor runs without a target file, so the workspace root
// is discovered from the cwd. A load error degrades to built-in defaults rather
// than failing the report.
func loadDoctorConfig() *config.Config {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	root, err := FindWorkspaceRootFromDir(cwd)
	if err != nil {
		root = cwd
	}
	cfg, err := config.Load(flagConfig, root)
	if err != nil {
		return nil
	}
	return cfg
}

func buildDoctorReport() DoctorReport {
	report := DoctorReport{
		SchemaVersion: doctorSchemaVersion,
		Command:       "doctor",
	}
	cfg := doctorConfigFn()
	for _, entry := range config.SupportMatrix {
		// The ts-morph adapter is the preferred TypeScript backend; surface it
		// just before the TypeScript language-server fallback row.
		if entry.Language == "typescript" && entry.Backend == "lsp/typescript-language-server" {
			report.Backends = append(report.Backends, probeTSMorphAdapter())
		}
		report.Backends = append(report.Backends, probeSupportEntry(cfg, entry))
	}
	return report
}

// probeTSMorphAdapter checks whether the ts-morph adapter npm package is
// discoverable on the current machine (global install or repo-relative dev path).
func probeTSMorphAdapter() DoctorBackendStatus {
	row := DoctorBackendStatus{
		Language:    "typescript",
		Backend:     "tsmorph",
		Operations:  []string{"rename"},
		InstallHint: tsmorph.AdapterInstallHint(),
		Caveats:     "Preferred adapter for TypeScript/JavaScript rename; falls back to language server when missing.",
	}
	if tsAdapterAvailableFn() {
		row.Status = DoctorStatusOK
	} else {
		row.Status = DoctorStatusMissing
		row.MissingDependency = tsmorph.AdapterPackageName
	}
	return row
}

// probeSupportEntry turns one support-matrix row into a doctor row. Unsupported
// languages are reported without a host probe. For everything else, the server
// binary is looked up on PATH: a configured server command (user/project
// config) takes precedence over the matrix default so custom servers report as
// found.
func probeSupportEntry(cfg *config.Config, entry config.LanguageSupport) DoctorBackendStatus {
	row := DoctorBackendStatus{
		Language:    entry.Language,
		Backend:     entry.Backend,
		InstallHint: entry.InstallHint,
		Caveats:     entry.Caveats,
	}
	if entry.Level == config.LevelUnsupported {
		row.Status = DoctorStatusNotClaimed
		return row
	}
	row.Operations = entry.Operations

	// The matrix Binary is the default probe target. An explicit user/project
	// server override wins so a custom server reports as found. We read the
	// resolved Servers map directly (not cfg.Server, which falls back to the
	// builtin command) so doctor stays decoupled from builtinServers.
	binary := entry.Binary
	if cfg != nil {
		if srv, ok := cfg.Servers[entry.Language]; ok && srv.Command != "" {
			binary = srv.Command
		}
	}

	path, err := lookPathFn(binary)
	if err != nil {
		row.Status = DoctorStatusMissing
		row.MissingDependency = binary
		return row
	}
	row.Status = presentStatus(entry.Level)
	row.Binary = path
	return row
}

// presentStatus maps a release-support tier to the doctor status reported when
// the backend is present locally.
func presentStatus(level string) string {
	switch level {
	case config.LevelSupported:
		return DoctorStatusOK
	case config.LevelExperimental:
		return DoctorStatusExperimental
	case config.LevelPlanned:
		return DoctorStatusPlanned
	default:
		return DoctorStatusOK
	}
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

func renderDoctorHuman(w io.Writer, report DoctorReport) {
	for _, b := range report.Backends {
		fmt.Fprintf(w, "%-12s %-40s %s\n", b.Language, b.Backend, b.Status)
		if b.Binary != "" {
			fmt.Fprintf(w, "  binary: %s\n", b.Binary)
		}
		if len(b.Operations) > 0 && b.Status != DoctorStatusNotClaimed {
			fmt.Fprintf(w, "  operations: %s\n", strings.Join(b.Operations, ", "))
		}
		if b.Status == DoctorStatusMissing && b.InstallHint != "" {
			fmt.Fprintf(w, "  install: %s\n", b.InstallHint)
		}
		if b.Caveats != "" {
			fmt.Fprintf(w, "  note: %s\n", b.Caveats)
		}
	}
}
