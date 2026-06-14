package openrewrite

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// nopWriteCloser adapts an io.Writer to io.WriteCloser for tests that only need
// to capture (or discard) the request written by callRename.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// TestCallRenameLargeResponse feeds a response whose newContent exceeds the
// 64 KiB default bufio.Scanner token cap through the driver and asserts the
// edit round-trips intact. A bufio.Scanner-based reader would have failed here.
func TestCallRenameLargeResponse(t *testing.T) {
	// 200 KiB of content, well past the old 64 KiB scanner limit.
	bigContent := strings.Repeat("abcdefghij \n", 20000)
	if len(bigContent) <= 64*1024 {
		t.Fatalf("test content too small: %d bytes", len(bigContent))
	}

	targetPath := filepath.Join(t.TempDir(), "Greeter.java")

	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"result": []map[string]any{
			{"path": targetPath, "newContent": bigContent},
		},
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	a := &Adapter{
		stdin:  nopWriteCloser{io.Discard},
		stdout: json.NewDecoder(strings.NewReader(string(respBytes) + "\n")),
	}

	fileEdits, err := a.callRename(map[string]any{"newName": "Hello"})
	if err != nil {
		t.Fatalf("callRename: %v", err)
	}
	if len(fileEdits) != 1 {
		t.Fatalf("got %d file edits, want 1", len(fileEdits))
	}
	fe := fileEdits[0]
	if fe.Path != targetPath {
		t.Errorf("path: got %q, want %q", fe.Path, targetPath)
	}
	if len(fe.Edits) != 1 {
		t.Fatalf("got %d text edits, want 1", len(fe.Edits))
	}
	if fe.Edits[0].NewText != bigContent {
		t.Errorf("NewText length: got %d, want %d (content not round-tripped)",
			len(fe.Edits[0].NewText), len(bigContent))
	}
}

// TestCallRenameSubprocessError surfaces a structured OpenRewrite error.
func TestCallRenameSubprocessError(t *testing.T) {
	resp := `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}` + "\n"
	a := &Adapter{
		stdin:  nopWriteCloser{io.Discard},
		stdout: json.NewDecoder(strings.NewReader(resp)),
	}
	_, err := a.callRename(map[string]any{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not contain subprocess message", err)
	}
}

// TestCallRenameNoResponseIncludesStderr verifies that when the subprocess
// produces no response (EOF), captured JVM stderr is folded into the error
// instead of being discarded.
func TestCallRenameNoResponseIncludesStderr(t *testing.T) {
	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	const diag = "java.lang.NoClassDefFoundError: kaboom"
	if _, err := stderrFile.WriteString(diag); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	a := &Adapter{
		stdin:      nopWriteCloser{io.Discard},
		stdout:     json.NewDecoder(strings.NewReader("")), // immediate EOF
		stderrFile: stderrFile,
		stderrPath: stderrFile.Name(),
	}

	_, err = a.callRename(map[string]any{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no response") {
		t.Errorf("error %q missing no-response context", err)
	}
	if !strings.Contains(err.Error(), diag) {
		t.Errorf("error %q does not include captured JVM stderr", err)
	}
}

// TestReadStderrTruncates caps captured stderr at maxJVMStderrBytes.
func TestReadStderrTruncates(t *testing.T) {
	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	big := strings.Repeat("x", maxJVMStderrBytes+1024)
	if _, err := stderrFile.WriteString(big); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	a := &Adapter{stderrFile: stderrFile, stderrPath: stderrFile.Name()}
	got := a.readStderr()
	if !strings.HasSuffix(got, "[stderr truncated]") {
		t.Errorf("expected truncation marker, got suffix %q", got[max(0, len(got)-40):])
	}
	if len(got) > maxJVMStderrBytes+len(" ... [stderr truncated]") {
		t.Errorf("stderr not bounded: got %d bytes", len(got))
	}
}

// TestShutdownTimesOutAndKills verifies Shutdown does not hang on a subprocess
// that ignores stdin closure: it times out and kills the process.
func TestShutdownTimesOutAndKills(t *testing.T) {
	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not on PATH")
	}
	cmd := exec.Command(sleepBin, "60")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}

	a := &Adapter{
		process:         cmd,
		stdin:           stdin,
		shutdownTimeout: 100 * time.Millisecond,
	}

	deadline := make(chan error, 1)
	go func() { deadline <- a.Shutdown() }()

	select {
	case err := <-deadline:
		if err == nil {
			t.Fatal("expected timeout error from Shutdown, got nil")
		}
		if !strings.Contains(err.Error(), "killed") {
			t.Errorf("error %q does not indicate the process was killed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown hung past kill fallback")
	}
}

// TestShutdownCleanExit returns the subprocess exit status for a process that
// exits promptly after stdin closes.
func TestShutdownCleanExit(t *testing.T) {
	catBin, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat not on PATH")
	}
	// `cat` with no args reads stdin until EOF, then exits cleanly.
	cmd := exec.Command(catBin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start cat: %v", err)
	}
	a := &Adapter{
		process:         cmd,
		stdin:           stdin,
		shutdownTimeout: 5 * time.Second,
	}
	if err := a.Shutdown(); err != nil {
		t.Fatalf("clean Shutdown returned error: %v", err)
	}
	// Shutdown must be idempotent (sync.Once): a second call is a no-op, not a
	// double-close panic or spurious error.
	if err := a.Shutdown(); err != nil {
		t.Fatalf("second Shutdown returned error: %v", err)
	}
}

// TestShutdownCleanExitNonZero verifies that a subprocess which exits with a
// non-zero status after stdin EOF (e.g. System.exit(1)) is treated as a clean
// shutdown — the process did exit, so Shutdown must not return the ExitError.
func TestShutdownCleanExitNonZero(t *testing.T) {
	shBin, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not on PATH")
	}
	// Drain stdin, then exit non-zero once stdin closes.
	cmd := exec.Command(shBin, "-c", "cat >/dev/null; exit 3")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sh: %v", err)
	}
	a := &Adapter{
		process:         cmd,
		stdin:           stdin,
		shutdownTimeout: 5 * time.Second,
	}
	if err := a.Shutdown(); err != nil {
		t.Fatalf("non-zero clean Shutdown should be treated as success, got: %v", err)
	}
}
