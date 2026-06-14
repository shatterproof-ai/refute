package telemetry

import (
	"os"
	"path/filepath"
	"strings"
)

// callerRules maps env-var prefixes to caller names, checked in priority order.
var callerRules = []struct {
	prefix string
	name   string
}{
	{"CLAUDE_", "claude"},
	{"CURSOR_", "cursor"},
	{"GOOSE_", "goose"},
	{"CODEX_", "codex"},
	{"GITHUB_ACTIONS", "github-actions"},
}

// envAllowPrefixes is the set of env-var prefixes captured in the env snapshot.
var envAllowPrefixes = []string{
	"CLAUDE_",
	"CURSOR_",
	"GOOSE_",
	"CODEX_",
	"GITHUB_",
	"CI",
	"TERM",
}

// secretPatterns is used to redact keys that look like credentials.
var secretPatterns = []string{"KEY", "SECRET", "TOKEN", "PASSWORD"}

func detectCaller(environ []string) string {
	for _, rule := range callerRules {
		for _, kv := range environ {
			key, _, _ := strings.Cut(kv, "=")
			if strings.HasPrefix(key, rule.prefix) {
				return rule.name
			}
		}
	}
	return ""
}

func filteredEnv(environ []string) map[string]string {
	env := make(map[string]string)
	for _, kv := range environ {
		key, val, _ := strings.Cut(kv, "=")
		if isSecret(key) {
			continue
		}
		for _, prefix := range envAllowPrefixes {
			if strings.HasPrefix(key, prefix) {
				env[key] = val
				break
			}
		}
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

func isSecret(key string) bool {
	upper := strings.ToUpper(key)
	for _, p := range secretPatterns {
		if strings.Contains(upper, p) {
			return true
		}
	}
	return false
}

// DefaultPath returns the default telemetry file path: ~/.local/share/refute/telemetry.jsonl.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "refute", "telemetry.jsonl")
}
