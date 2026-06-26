package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #119: drift guardrail. The CLI command inventory is a drift-sensitive
// surface (see docs/drift-control.md): the registered cobra commands are the
// normative source of truth for which subcommands exist, and several docs are
// required to mirror them. Historically a command would ship while a doc still
// called it future/missing, or a command would land with no doc entry at all.
//
// These tests fail when a registered command is absent from the documentation
// sections that are supposed to list every shipped command, so the doc can no
// longer silently fall behind the runtime. They run under `go test ./...`, so
// they are part of `make verify` via the unit-test gate without adding a new
// gate.

// docCommandException lists registered command names that are intentionally not
// expected to appear in the user-facing command inventories. Cobra injects
// `help` and `completion`; they are not refute operations and are not
// documented as such.
var docCommandException = map[string]bool{
	"help":       true,
	"completion": true,
}

// inventoryCommands returns the registered, user-facing command names that the
// docs are expected to enumerate. Hidden commands and cobra's injected helpers
// are excluded.
func inventoryCommands(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, c := range RootCmd.Commands() {
		if c.Hidden || docCommandException[c.Name()] {
			continue
		}
		names = append(names, c.Name())
	}
	if len(names) == 0 {
		t.Fatal("no registered commands found; RootCmd.Commands() returned nothing")
	}
	return names
}

// repoRoot walks up from the test's working directory to the module root (the
// directory containing go.mod) so the test can read repo docs regardless of the
// package directory it runs from.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %s", dir)
		}
		dir = parent
	}
}

// readDocSection returns the body of the Markdown section that begins with the
// given heading line, stopping at the next heading of the same or higher level.
// It fails the test if the heading is not present so a renamed/removed section
// is caught rather than silently treated as empty.
func readDocSection(t *testing.T, repo, relPath, heading string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repo, relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	level := countLeadingHashes(heading)
	if level == 0 {
		t.Fatalf("heading %q must start with one or more '#' followed by a space", heading)
	}

	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimRight(line, " \t") == heading {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("section %q not found in %s; drift-control expects it to enumerate the CLI inventory", heading, relPath)
	}

	var b strings.Builder
	for _, line := range lines[start+1:] {
		if hashes := countLeadingHashes(line); hashes > 0 && hashes <= level {
			break
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func countLeadingHashes(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n > 0 && n < len(line) && line[n] == ' ' {
		return n
	}
	return 0
}

// containsCommandName reports whether s documents name as a command in the
// backtick code-span form both doc inventories use: either `name` (the
// docs/current-state CLI Surface list form) or `refute name` (the README
// Operations table form). Requiring the backtick delimiters — rather than a
// bare word boundary — is what keeps the guard honest: a prose mention of a
// command's name cannot satisfy it after the command's real inventory entry is
// removed, and a prefix name like "rename" cannot match inside a longer name
// like "rename-function" (its right delimiter would be '-', not a backtick).
func containsCommandName(s, name string) bool {
	const refutePrefix = "`refute " // README Operations: `refute <name>`
	for i := 0; i+len(name) <= len(s); i++ {
		if s[i:i+len(name)] != name {
			continue
		}
		// Right boundary: the name must be closed by a backtick, so a name
		// flush against the end of s (no closing delimiter) does not match.
		if i+len(name) >= len(s) || s[i+len(name)] != '`' {
			continue
		}
		// Left boundary: an opening backtick (`name`) or the README
		// `refute <name>` form.
		if i > 0 && s[i-1] == '`' {
			return true
		}
		if i >= len(refutePrefix) && s[i-len(refutePrefix):i] == refutePrefix {
			return true
		}
	}
	return false
}

// TestContainsCommandNameBoundaries pins the boundary contract directly so the
// inventory guards below cannot silently regress to a loose substring match. A
// command name counts only as a backtick-delimited code span — the form both
// doc inventories use — so a bare prose mention or a prefix inside a longer
// command name does not satisfy the guard.
func TestContainsCommandNameBoundaries(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{"rename", "| `refute rename` | Rename a symbol. |", true},        // README Operations table form
		{"rename", "- `rename`;", true},                                   // current-state CLI Surface list form
		{"doctor", "Run `refute doctor` to check backends.", true},        // inline backtick reference
		{"rename-function", "- `rename-function`;", true},                 // hyphenated variant
		{"rename", "variants: `rename-function`, `rename-class`.", false}, // prefix inside a longer command name
		{"rename", "You can rename a symbol with the tool.", false},       // bare prose mention, no backticks
		{"rename", "the `refute rename-function` variant", false},         // refute-prefixed but a longer name
	}
	for _, tc := range cases {
		if got := containsCommandName(tc.s, tc.name); got != tc.want {
			t.Errorf("containsCommandName(%q, %q) = %v, want %v", tc.s, tc.name, got, tc.want)
		}
	}
}

// TestCommandInventoryDocumentedInReadme verifies every registered command
// appears in the README "## Operations" table. A command shipped without a
// README entry — or a README that still omits a renamed command — fails here.
func TestCommandInventoryDocumentedInReadme(t *testing.T) {
	repo := repoRoot(t)
	section := readDocSection(t, repo, "README.md", "## Operations")
	for _, name := range inventoryCommands(t) {
		if !containsCommandName(section, name) {
			t.Errorf("command %q is registered but absent from README.md \"## Operations\"; "+
				"update the command table (see docs/drift-control.md CLI-inventory rule)", name)
		}
	}
}

// TestCommandInventoryDocumentedInCurrentState verifies every registered
// command appears in the docs/current-state.md "### CLI Surface" section, which
// drift-control names as the normative shipped-command list. This is the guard
// against the historical drift where a shipped command was still described as
// planned/future: a command listed only in a roadmap/planned section, and not
// in the implemented CLI Surface, fails here.
func TestCommandInventoryDocumentedInCurrentState(t *testing.T) {
	repo := repoRoot(t)
	section := readDocSection(t, repo, "docs/current-state.md", "### CLI Surface")
	for _, name := range inventoryCommands(t) {
		if !containsCommandName(section, name) {
			t.Errorf("command %q is registered but absent from docs/current-state.md \"### CLI Surface\"; "+
				"a shipped command must be listed as implemented, not left in a planned/future section "+
				"(see docs/drift-control.md status-freshness rule)", name)
		}
	}
}
