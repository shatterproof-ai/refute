package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one telemetry record written before a refute command executes.
type Entry struct {
	Ts     string            `json:"ts"`
	Args   []string          `json:"args"`
	Cwd    string            `json:"cwd"`
	Caller string            `json:"caller,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}

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

// Capture builds an Entry from the given args and cwd, reading caller identity
// and the env snapshot from the current process environment.
func Capture(args []string, cwd string) Entry {
	return CaptureFrom(args, cwd, os.Environ())
}

// CaptureFrom is like Capture but uses the provided environ slice instead of
// os.Environ(). Useful for tests.
func CaptureFrom(args []string, cwd string, environ []string) Entry {
	e := Entry{
		Ts:   time.Now().UTC().Format(time.RFC3339),
		Args: args,
		Cwd:  cwd,
	}
	e.Caller = detectCaller(environ)
	e.Env = filteredEnv(environ)
	return e
}

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

// Append serializes e as a JSON line and appends it to path, creating parent
// directories as needed. Errors are silently discarded — telemetry must never
// disrupt normal CLI operation.
func Append(path string, e Entry) {
	if path == "" {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}
