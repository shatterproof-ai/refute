package cli

import (
	"strings"
	"testing"
)

// TestDoctor_IncludesTSMorphAdapterEntry verifies that the doctor report
// includes a backend entry for the ts-morph adapter under the typescript
// language so users know how to install it.
func TestDoctor_IncludesTSMorphAdapterEntry(t *testing.T) {
	report := buildDoctorReport()

	var found *DoctorBackendStatus
	for i := range report.Backends {
		b := &report.Backends[i]
		if b.Language == "typescript" && b.Backend == "tsmorph" {
			found = b
			break
		}
	}
	if found == nil {
		t.Fatal("doctor report missing entry with language=typescript backend=tsmorph")
	}

	const wantHint = "npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz"
	if found.InstallHint != wantHint {
		t.Errorf("InstallHint = %q, want %q", found.InstallHint, wantHint)
	}
	if strings.Contains(found.InstallHint, "npm install -g @shatterproof-ai/refute-ts-adapter") {
		t.Errorf("InstallHint still references nonexistent npm registry package: %q", found.InstallHint)
	}
}

// TestDoctor_TSMorphAdapterReflectsAvailability verifies that the ts-morph
// doctor entry reports ok when the adapter is available and missing otherwise.
func TestDoctor_TSMorphAdapterReflectsAvailability(t *testing.T) {
	oldFn := tsAdapterAvailableFn
	t.Cleanup(func() { tsAdapterAvailableFn = oldFn })

	tsAdapterAvailableFn = func() bool { return true }
	report := buildDoctorReport()
	entry := doctorTSMorphEntry(t, report)
	if entry.Status != DoctorStatusOK {
		t.Errorf("status with adapter present = %q, want %q", entry.Status, DoctorStatusOK)
	}

	tsAdapterAvailableFn = func() bool { return false }
	report = buildDoctorReport()
	entry = doctorTSMorphEntry(t, report)
	if entry.Status != DoctorStatusMissing {
		t.Errorf("status with adapter absent = %q, want %q", entry.Status, DoctorStatusMissing)
	}
	if entry.MissingDependency != "@shatterproof-ai/refute-ts-adapter" {
		t.Errorf("MissingDependency = %q, want npm package name", entry.MissingDependency)
	}
}

func doctorTSMorphEntry(t *testing.T, report DoctorReport) DoctorBackendStatus {
	t.Helper()
	for _, b := range report.Backends {
		if b.Language == "typescript" && b.Backend == "tsmorph" {
			return b
		}
	}
	t.Fatal("doctor report missing typescript/tsmorph entry")
	return DoctorBackendStatus{}
}
