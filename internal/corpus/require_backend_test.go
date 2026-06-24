//go:build corpus

package corpus_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBackendSkipReason verifies that backendSkipReason checks every declared
// backend (both backendEnv and backendTool) rather than stopping at the first
// one. A target that sets both fields must skip if either is unavailable, with a
// combined reason naming each missing backend (issue #125).
func TestBackendSkipReason(t *testing.T) {
	// A binary that exists on PATH in the test environment, and an env var we
	// can control. Use the test process's own executable dir to synthesize a
	// guaranteed-present tool and a guaranteed-absent one.
	presentTool := filepath.Base(os.Args[0]) // the test binary itself; on PATH via its dir
	tdir := filepath.Dir(os.Args[0])
	t.Setenv("PATH", tdir+string(os.PathListSeparator)+os.Getenv("PATH"))

	const absentTool = "refute-corpus-definitely-not-a-real-binary"
	const setEnv = "REFUTE_CORPUS_TEST_ENV_SET"
	const unsetEnv = "REFUTE_CORPUS_TEST_ENV_UNSET"
	t.Setenv(setEnv, "1")
	os.Unsetenv(unsetEnv)

	tests := []struct {
		name        string
		tgt         target
		wantSkip    bool
		wantSubstrs []string
	}{
		{
			name:     "both present",
			tgt:      target{Language: "java", Name: "t", BackendEnv: setEnv, BackendTool: presentTool},
			wantSkip: false,
		},
		{
			name:        "only env missing",
			tgt:         target{Language: "java", Name: "t", BackendEnv: unsetEnv, BackendTool: presentTool},
			wantSkip:    true,
			wantSubstrs: []string{unsetEnv},
		},
		{
			name:        "only tool missing",
			tgt:         target{Language: "java", Name: "t", BackendEnv: setEnv, BackendTool: absentTool},
			wantSkip:    true,
			wantSubstrs: []string{absentTool},
		},
		{
			name:        "both missing",
			tgt:         target{Language: "java", Name: "t", BackendEnv: unsetEnv, BackendTool: absentTool},
			wantSkip:    true,
			wantSubstrs: []string{unsetEnv, absentTool},
		},
		{
			name:        "single env field, missing",
			tgt:         target{Language: "java", Name: "t", BackendEnv: unsetEnv},
			wantSkip:    true,
			wantSubstrs: []string{unsetEnv},
		},
		{
			name:     "single env field, present",
			tgt:      target{Language: "java", Name: "t", BackendEnv: setEnv},
			wantSkip: false,
		},
		{
			name:        "single tool field, missing",
			tgt:         target{Language: "go", Name: "t", BackendTool: absentTool},
			wantSkip:    true,
			wantSubstrs: []string{absentTool},
		},
		{
			name:     "single tool field, present",
			tgt:      target{Language: "go", Name: "t", BackendTool: presentTool},
			wantSkip: false,
		},
		{
			name:     "no backend declared",
			tgt:      target{Language: "go", Name: "t"},
			wantSkip: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reason := backendSkipReason(tc.tgt)
			if tc.wantSkip && reason == "" {
				t.Fatalf("expected a skip reason, got none")
			}
			if !tc.wantSkip && reason != "" {
				t.Fatalf("expected no skip reason, got %q", reason)
			}
			for _, sub := range tc.wantSubstrs {
				if !strings.Contains(reason, sub) {
					t.Fatalf("skip reason %q does not mention %q", reason, sub)
				}
			}
		})
	}
}
