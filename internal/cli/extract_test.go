package cli

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/selector"
	"github.com/shatterproof-ai/refute/internal/edit"
)

// resetInvocationFlagsForTest clears the process-global invocation flags so an
// error-path test for runExtract/runInline starts from a known state, and
// restores them afterward. It returns nothing; callers set the flags they care
// about after calling it. Shared by the extract and inline error-path tests.
func resetInvocationFlagsForTest(t *testing.T) {
	t.Helper()
	prevConfig := flagConfig
	prevJSON := flagJSON
	prevDryRun := flagDryRun
	flagJSON = false
	flagDryRun = false
	flagConfig = ""
	t.Cleanup(func() {
		flagJSON = prevJSON
		flagDryRun = prevDryRun
		flagConfig = prevConfig
	})
}

// TestRunExtract_HumanUnsupportedLanguage drives runExtract through the
// buildBackend selection failure for a language the support matrix gates as
// unsupported (Java). In human (non-JSON) mode the terminal error is returned
// verbatim for Run to render on stderr, and it maps to the generic exit code 1
// because ErrLanguageUnsupported carries no ExitCode of its own. This exercises
// the CLI-facing error path that feeds runExtract, not just parseCallSite.
func TestRunExtract_HumanUnsupportedLanguage(t *testing.T) {
	resetInvocationFlagsForTest(t)
	dir := t.TempDir()
	javaFile := writeJavaFixture(t, dir)

	flags := &extractFlags{
		File:      javaFile,
		StartLine: 2,
		StartCol:  10,
		EndLine:   2,
		EndCol:    15,
	}
	flagJSON = false

	err := runExtract("function", flags)
	if err == nil {
		t.Fatal("expected an error for an unsupported language, got nil")
	}
	var langUnsupported *selector.ErrLanguageUnsupported
	if !errors.As(err, &langUnsupported) {
		t.Fatalf("error = %v, want *selector.ErrLanguageUnsupported", err)
	}
	// Human-mode errors must not be the jsonEmitted sentinel: nothing was
	// written to stdout, so Run is responsible for printing to stderr.
	var emitted *jsonEmitted
	if errors.As(err, &emitted) {
		t.Fatal("human-mode error must not be jsonEmitted")
	}
	if got := exitCodeForError(err); got != 1 {
		t.Fatalf("exit code = %d, want 1", got)
	}
}

// TestRunExtract_JSONUnsupportedLanguage covers the JSON contract (#57) on
// runExtract's error path: with --json set, a single structured envelope is
// written to stdout and the returned error is the jsonEmitted sentinel wrapping
// an ExitCodeError, so Run does not print a second envelope.
func TestRunExtract_JSONUnsupportedLanguage(t *testing.T) {
	resetInvocationFlagsForTest(t)
	dir := t.TempDir()
	javaFile := writeJavaFixture(t, dir)

	flags := &extractFlags{
		File:      javaFile,
		StartLine: 2,
		StartCol:  10,
		EndLine:   2,
		EndCol:    15,
	}
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runExtract("variable", flags)
	})

	var emitted *jsonEmitted
	if !errors.As(runErr, &emitted) {
		t.Fatalf("error = %#v, want jsonEmitted", runErr)
	}
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	assertSingleJSONEnvelope(t, out)
	assertJSONErrorEnvelope(t, []byte(out), edit.StatusUnsupported, "unsupported-language")

	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Operation != "extract-variable" {
		t.Errorf("operation = %q, want extract-variable; envelope:\n%s", got.Operation, out)
	}
	if got.Language != "java" {
		t.Errorf("language = %q, want java; envelope:\n%s", got.Language, out)
	}
}

// TestRunExtract_HumanBackendMissingExitCode covers the failure exit-code
// contract (#58): a missing LSP server surfaces *ErrLSPServerMissing, which maps
// to exit code 3 ("install a backend"). It drives the full runExtract path,
// including buildBackend's PATH lookup, in human mode.
func TestRunExtract_HumanBackendMissingExitCode(t *testing.T) {
	resetInvocationFlagsForTest(t)
	dir := t.TempDir()
	goFile := writeGoFixture(t, dir)
	flagConfig = writeServerConfig(t, dir, "go", "refute-nonexistent-lsp-binary-xyz")

	flags := &extractFlags{
		File:      goFile,
		StartLine: 3,
		StartCol:  1,
		EndLine:   3,
		EndCol:    1,
	}
	flagJSON = false

	err := runExtract("function", flags)
	if err == nil {
		t.Fatal("expected an error for a missing backend, got nil")
	}
	var missing *ErrLSPServerMissing
	if !errors.As(err, &missing) {
		t.Fatalf("error = %v, want *ErrLSPServerMissing", err)
	}
	code, _ := exitDetails(err)
	if code != backendMissingExitCode {
		t.Fatalf("exit code = %d, want %d", code, backendMissingExitCode)
	}
}

// TestRunExtract_JSONBackendMissing covers the JSON contract for the
// backend-missing case on runExtract: one envelope on stdout with status
// backend-missing and exit code 3.
func TestRunExtract_JSONBackendMissing(t *testing.T) {
	resetInvocationFlagsForTest(t)
	dir := t.TempDir()
	goFile := writeGoFixture(t, dir)
	flagConfig = writeServerConfig(t, dir, "go", "refute-nonexistent-lsp-binary-xyz")

	flags := &extractFlags{
		File:      goFile,
		StartLine: 3,
		StartCol:  1,
		EndLine:   3,
		EndCol:    1,
	}
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runExtract("function", flags)
	})

	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code != backendMissingExitCode {
		t.Fatalf("expected ExitCodeError with code %d, got %#v", backendMissingExitCode, runErr)
	}
	assertSingleJSONEnvelope(t, out)
	assertJSONErrorEnvelope(t, []byte(out), edit.StatusBackendMissing, "backend-missing")
}

// assertSingleJSONEnvelope fails unless out is exactly one JSON value. The #57
// contract is that error paths emit exactly one envelope; a duplicate envelope
// or stray output alongside the structured result would leave trailing tokens
// after the first decode. The envelope itself is pretty-printed, so this decodes
// one value and then asserts the stream is at EOF rather than counting lines.
func assertSingleJSONEnvelope(t *testing.T, out string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var first json.RawMessage
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode first JSON value: %v\nraw:\n%s", err, out)
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		t.Fatalf("expected exactly one JSON envelope, found trailing data (err=%v):\n%s", err, out)
	}
}
