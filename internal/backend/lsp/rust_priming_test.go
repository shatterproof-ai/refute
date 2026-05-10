package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrimeRustWorkspace_SkipListAndCap(t *testing.T) {
	tmp := t.TempDir()
	// Valid Rust files.
	mkfile(t, tmp, "src/lib.rs")
	mkfile(t, tmp, "src/main.rs")
	// Should be skipped: target/, .git/, node_modules/, .cargo/.
	mkfile(t, tmp, "target/debug/build/foo-123/out/junk.rs")
	mkfile(t, tmp, ".git/hooks/post-commit.rs") // contrived
	mkfile(t, tmp, "node_modules/crate/src/x.rs")
	mkfile(t, tmp, ".cargo/registry/y.rs")
	// Additional real files to test the cap.
	for i := 0; i < maxPrimedFiles+5; i++ {
		mkfile(t, tmp, filepath.Join("extra", "f"+intToStr(i)+".rs"))
	}

	client := newFakeClient()
	err := PrimeRustWorkspace(client, tmp)
	if err != nil {
		t.Fatalf("PrimeRustWorkspace: %v", err)
	}
	if len(client.opened) > maxPrimedFiles {
		t.Errorf("opened %d files, want ≤%d", len(client.opened), maxPrimedFiles)
	}
	for _, path := range client.opened {
		for _, banned := range []string{"target/", ".git/", "node_modules/", ".cargo/"} {
			if contains(path, banned) {
				t.Errorf("opened banned path %q", path)
			}
		}
	}
}

type fakeClient struct{ opened []string }

func newFakeClient() *fakeClient { return &fakeClient{} }

func (c *fakeClient) DidOpen(path, langID string) error {
	c.opened = append(c.opened, path)
	return nil
}

func mkfile(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("// test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
