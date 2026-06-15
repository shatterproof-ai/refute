package openrewrite

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/capture"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// jarCacheSubPath is the relative path where the adapter JAR is built by Maven.
const jarCacheSubPath = "adapters/openrewrite/target/openrewrite-adapter.jar"

// defaultShutdownTimeout bounds how long Shutdown waits for the JVM to exit
// after stdin is closed before falling back to killing the process.
const defaultShutdownTimeout = 5 * time.Second

// Adapter wraps the OpenRewrite JVM subprocess to implement backend.RefactoringBackend.
type Adapter struct {
	jarPath       string
	workspaceRoot string
	process       *exec.Cmd
	stdin         io.WriteCloser
	stdout        *json.Decoder
	stderr        *capture.Stderr
	shutdownOnce  sync.Once
	// shutdownTimeout overrides defaultShutdownTimeout when non-zero (tests).
	shutdownTimeout time.Duration
	nextID          atomic.Int64
}

var _ backend.RefactoringBackend = (*Adapter)(nil)

// NewAdapter creates an Adapter using jarPath as the OpenRewrite fat JAR.
// If jarPath is empty, Initialize will search for the JAR relative to the
// workspace root.
func NewAdapter(jarPath string) *Adapter {
	return &Adapter{jarPath: jarPath}
}

// Initialize starts the JVM subprocess with the OpenRewrite adapter JAR.
func (a *Adapter) Initialize(workspaceRoot string) error {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("abs workspace root: %w", err)
	}
	a.workspaceRoot = absRoot

	jar, err := a.resolveJar(absRoot)
	if err != nil {
		return err
	}

	java, err := exec.LookPath("java")
	if err != nil {
		return fmt.Errorf("java not found on PATH: %w", err)
	}

	cmd := exec.Command(java, "-jar", jar)

	// Capture JVM stderr to a temp file so diagnostics (including the adapter's
	// own parse errors) can be surfaced in errors instead of being discarded.
	stderr, err := capture.New("refute-openrewrite-stderr-*")
	if err != nil {
		return fmt.Errorf("stderr temp file: %w", err)
	}
	cmd.Stderr = stderr.File()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		stderr.Cleanup()
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stderr.Cleanup()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stderr.Cleanup()
		return fmt.Errorf("start JVM: %w", err)
	}

	a.process = cmd
	a.stdin = stdin
	a.stdout = json.NewDecoder(stdout)
	a.stderr = stderr

	return nil
}

// Shutdown terminates the JVM subprocess. It closes stdin, waits for a clean
// exit within a bounded timeout, and falls back to killing the process so a
// hung JVM cannot block the CLI forever.
func (a *Adapter) Shutdown() error {
	var err error
	a.shutdownOnce.Do(func() {
		if a.stdin != nil {
			_ = a.stdin.Close()
		}
		if a.process != nil {
			err = a.waitWithTimeout()
		}
		a.cleanupStderr()
	})
	return err
}

// waitWithTimeout waits for the subprocess to exit, killing it if it does not
// exit within the shutdown timeout. It always reaps the process to avoid
// leaking a zombie.
func (a *Adapter) waitWithTimeout() error {
	timeout := a.shutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	done := make(chan error, 1)
	go func() { done <- a.process.Wait() }()
	select {
	case err := <-done:
		// The process exited on its own after stdin closed. A non-zero exit
		// status (e.g. System.exit(1)) is reported as *exec.ExitError; that
		// still means the JVM terminated, so treat it as a clean shutdown and
		// only surface genuine wait failures (I/O errors, lost child, etc.).
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return err
	case <-time.After(timeout):
		if a.process.Process != nil {
			_ = a.process.Process.Kill()
		}
		<-done // reap the killed process
		return fmt.Errorf("OpenRewrite subprocess did not exit within %s; killed", timeout)
	}
}

// stderrSuffix returns a "; JVM stderr: ..." fragment for folding captured JVM
// diagnostics into an error message, or "" when no stderr was captured.
func (a *Adapter) stderrSuffix() string {
	if a == nil {
		return ""
	}
	msg := a.stderr.Read(capture.DefaultMaxBytes)
	if msg == "" {
		return ""
	}
	return "; JVM stderr: " + msg
}

// cleanupStderr closes and removes the captured-stderr temp file.
func (a *Adapter) cleanupStderr() {
	a.stderr.Cleanup()
}

// FindSymbol returns ErrUnsupported — OpenRewrite does not expose symbol search.
func (a *Adapter) FindSymbol(_ symbol.Query) ([]symbol.Location, error) {
	return nil, backend.ErrUnsupported
}

// Rename renames a symbol in a Java project using OpenRewrite.
//
// For method rename: loc.Name must identify the method; a wildcard parameter
// pattern (..) is used so the recipe matches any overload.
// For type rename: the symbol kind must be KindClass or KindType.
func (a *Adapter) Rename(loc symbol.Location, newName string) (*edit.WorkspaceEdit, error) {
	if a.process == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}

	params, err := a.buildRenameParams(loc, newName)
	if err != nil {
		return nil, err
	}

	fileEdits, err := a.callRename(params)
	if err != nil {
		return nil, err
	}
	return &edit.WorkspaceEdit{FileEdits: fileEdits}, nil
}

// ExtractFunction returns ErrUnsupported.
func (a *Adapter) ExtractFunction(_ symbol.SourceRange, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

// ExtractVariable returns ErrUnsupported.
func (a *Adapter) ExtractVariable(_ symbol.SourceRange, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

// InlineSymbol returns ErrUnsupported.
func (a *Adapter) InlineSymbol(_ symbol.Location) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

// MoveToFile returns ErrUnsupported.
func (a *Adapter) MoveToFile(_ symbol.Location, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

// Capabilities returns the list of operations this adapter supports.
func (a *Adapter) Capabilities() []backend.Capability {
	return []backend.Capability{
		{Operation: "rename"},
	}
}

// buildRenameParams constructs the JSON-RPC params for a rename call.
func (a *Adapter) buildRenameParams(loc symbol.Location, newName string) (map[string]any, error) {
	params := map[string]any{
		"workspaceRoot": a.workspaceRoot,
		"newName":       newName,
	}

	switch loc.Kind {
	case symbol.KindClass, symbol.KindType:
		// ChangeType expects the fully-qualified class name only: "com.example.Greeter".
		fqn, err := javaTypeFQN(loc.File)
		if err != nil {
			return nil, fmt.Errorf("resolving FQN for type rename: %w", err)
		}
		params["oldFullyQualifiedName"] = fqn

	default:
		// ChangeMethodName expects "<class> <method>(..)": "com.example.Greeter greet(..)".
		prefix, err := javaMethodPatternPrefix(loc.File, loc.Name)
		if err != nil {
			return nil, fmt.Errorf("resolving FQN for method rename: %w", err)
		}
		params["methodPattern"] = prefix + "(..)"
	}

	return params, nil
}

// callRename sends the rename JSON-RPC request and parses the response.
func (a *Adapter) callRename(params map[string]any) ([]edit.FileEdit, error) {
	id := int(a.nextID.Add(1))
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "rename",
		"params":  params,
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if _, err := fmt.Fprintf(a.stdin, "%s\n", reqBytes); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Decode one JSON response value. A json.Decoder streams arbitrarily large
	// values, unlike a default bufio.Scanner whose 64 KiB token cap silently
	// truncates responses that embed full file contents.
	var resp struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result []struct {
			Path       string `json:"path"`
			NewContent string `json:"newContent"`
		} `json:"result"`
	}
	if err := a.stdout.Decode(&resp); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("no response from OpenRewrite subprocess%s", a.stderrSuffix())
		}
		return nil, fmt.Errorf("reading response from OpenRewrite subprocess: %w%s", err, a.stderrSuffix())
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("OpenRewrite error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	fileEdits := make([]edit.FileEdit, 0, len(resp.Result))
	for _, r := range resp.Result {
		fe, err := newContentToFileEdit(r.Path, r.NewContent)
		if err != nil {
			return nil, fmt.Errorf("converting result for %s: %w", r.Path, err)
		}
		fileEdits = append(fileEdits, fe)
	}
	return fileEdits, nil
}

// resolveJar finds the OpenRewrite adapter JAR. If a.jarPath is set it is
// used directly; otherwise the conventional build output path under the
// checkout root is tried.
func (a *Adapter) resolveJar(workspaceRoot string) (string, error) {
	if a.jarPath != "" {
		if _, err := os.Stat(a.jarPath); err == nil {
			return a.jarPath, nil
		}
		return "", fmt.Errorf("openrewrite JAR not found at %s", a.jarPath)
	}

	// Walk up from workspaceRoot to find the checkout root (where go.mod lives).
	checkoutRoot, err := findCheckoutRoot(workspaceRoot)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(checkoutRoot, jarCacheSubPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf(
		"OpenRewrite adapter JAR not found at %s; build it with: mvn package -f %s/adapters/openrewrite/pom.xml -q",
		candidate, checkoutRoot,
	)
}

// findCheckoutRoot walks up from dir looking for go.mod (the refute checkout root).
func findCheckoutRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// javaTypeFQN reads a Java source file and returns the fully-qualified class
// name, e.g. "com.example.Greeter". Used as oldFullyQualifiedName for ChangeType.
func javaTypeFQN(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", filePath, err)
	}
	src := string(data)

	pkg := parseJavaPackage(src)
	cls := parseJavaClass(src)
	if cls == "" {
		return "", fmt.Errorf("could not find class declaration in %s", filePath)
	}
	if pkg != "" {
		return pkg + "." + cls, nil
	}
	return cls, nil
}

// javaMethodPatternPrefix returns "<package>.<ClassName> <methodName>", the
// prefix of an OpenRewrite ChangeMethodName pattern before the parameter list.
func javaMethodPatternPrefix(filePath, methodName string) (string, error) {
	fqn, err := javaTypeFQN(filePath)
	if err != nil {
		return "", err
	}
	return fqn + " " + methodName, nil
}

// parseJavaPackage extracts the package name from Java source text.
func parseJavaPackage(src string) string {
	for _, line := range strings.SplitAfter(src, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "package ") && strings.HasSuffix(t, ";") {
			return strings.TrimSuffix(strings.TrimPrefix(t, "package "), ";")
		}
	}
	return ""
}

// parseJavaClass extracts the first public class (or interface/enum) name from
// Java source text.
func parseJavaClass(src string) string {
	for _, line := range strings.SplitAfter(src, "\n") {
		t := strings.TrimSpace(line)
		for _, keyword := range []string{"class ", "interface ", "enum ", "@interface "} {
			if idx := strings.Index(t, keyword); idx >= 0 {
				rest := t[idx+len(keyword):]
				// Take the identifier before any whitespace, '{', or '<'.
				end := strings.IndexAny(rest, " \t{<")
				if end < 0 {
					end = len(rest)
				}
				name := rest[:end]
				if name != "" && !strings.ContainsAny(name, "(){};") {
					return name
				}
			}
		}
	}
	return ""
}

// newContentToFileEdit converts an OpenRewrite result (full new file content)
// into an edit.FileEdit that replaces the entire file with the new content.
// The TextEdit range uses 0-indexed LSP positions.
func newContentToFileEdit(path, newContent string) (edit.FileEdit, error) {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return edit.FileEdit{}, fmt.Errorf("reading %s: %w", path, err)
	}
	oldLines := strings.Split(string(old), "\n")
	lastLineIdx := len(oldLines) - 1
	lastLineLen := len(oldLines[lastLineIdx])

	return edit.FileEdit{
		Path: path,
		Edits: []edit.TextEdit{
			{
				Range: edit.Range{
					Start: edit.Position{Line: 0, Character: 0},
					End:   edit.Position{Line: lastLineIdx, Character: lastLineLen},
				},
				NewText: newContent,
			},
		},
	}, nil
}
