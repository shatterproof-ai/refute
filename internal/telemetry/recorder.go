package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shatterproof-ai/refute/internal/edit"
)

const (
	SchemaVersion        = "2"
	EventInvocationStart = "invocation_start"
	EventInvocationEnd   = "invocation_end"

	envTelemetry          = "REFUTE_TELEMETRY"
	envTelemetrySnapshots = "REFUTE_TELEMETRY_SNAPSHOTS"
)

// Options controls where telemetry is written. Zero values use production
// defaults and the current process environment.
type Options struct {
	TelemetryPath string
	SnapshotRoot  string
	SessionRoot   string
	Environ       []string
	Verbose       bool
	Stderr        io.Writer
	Now           func() time.Time
}

// OperationContext is the operation metadata known after command parsing and
// backend selection.
type OperationContext struct {
	Operation     string
	Language      string
	Backend       string
	WorkspaceRoot string
	Status        string
	FilesModified int
	Warnings      []string
}

// FinishInfo contains the final process outcome.
type FinishInfo struct {
	ExitCode int
	Error    *ErrorInfo
}

// ErrorInfo is a compact telemetry-safe error summary.
type ErrorInfo struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// AgentInfo describes the detected agent process without credential fields.
type AgentInfo struct {
	Caller     string `json:"caller,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	Entrypoint string `json:"entrypoint,omitempty"`
	ExecPath   string `json:"execPath,omitempty"`
	Effort     string `json:"effort,omitempty"`
}

// PhaseTiming is one accumulated operation phase duration.
type PhaseTiming struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"durationMs"`
}

// EditStats summarizes the edit payload without requiring consumers to open
// snapshot files.
type EditStats struct {
	FilesModified int `json:"filesModified"`
}

// Recorder owns one refute invocation's telemetry lifecycle.
type Recorder struct {
	mu sync.Mutex

	enabled          bool
	snapshotsEnabled bool
	verbose          bool
	stderr           io.Writer
	now              func() time.Time

	telemetryPath string
	snapshotRoot  string
	sessionRoot   string

	invocationID string
	startedAt    time.Time
	args         []string
	cwd          string
	caller       string
	agent        AgentInfo
	env          map[string]string

	ctx      OperationContext
	project  ProjectInfo
	error    *ErrorInfo
	snapshot *SnapshotManifest

	phaseOrder []string
	phases     map[string]time.Duration
}

type invocationStartEvent struct {
	SchemaVersion string            `json:"schemaVersion"`
	Event         string            `json:"event"`
	Ts            string            `json:"ts"`
	InvocationID  string            `json:"invocationId"`
	StartedAt     string            `json:"startedAt"`
	Args          []string          `json:"args"`
	Cwd           string            `json:"cwd"`
	Caller        string            `json:"caller,omitempty"`
	Agent         AgentInfo         `json:"agent,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Project       ProjectInfo       `json:"project,omitempty"`
}

type invocationEndEvent struct {
	SchemaVersion    string        `json:"schemaVersion"`
	Event            string        `json:"event"`
	Ts               string        `json:"ts"`
	InvocationID     string        `json:"invocationId"`
	StartedAt        string        `json:"startedAt"`
	EndedAt          string        `json:"endedAt"`
	DurationMS       int64         `json:"durationMs"`
	ExitCode         int           `json:"exitCode"`
	Status           string        `json:"status"`
	Args             []string      `json:"args"`
	Cwd              string        `json:"cwd"`
	Caller           string        `json:"caller,omitempty"`
	Agent            AgentInfo     `json:"agent,omitempty"`
	Operation        string        `json:"operation,omitempty"`
	Language         string        `json:"language,omitempty"`
	Backend          string        `json:"backend,omitempty"`
	WorkspaceRoot    string        `json:"workspaceRoot,omitempty"`
	Project          ProjectInfo   `json:"project,omitempty"`
	PhaseTimings     []PhaseTiming `json:"phaseTimings,omitempty"`
	EditStats        EditStats     `json:"editStats,omitempty"`
	FilesModified    int           `json:"filesModified,omitempty"`
	SnapshotManifest string        `json:"snapshotManifest,omitempty"`
	Warnings         []string      `json:"warnings,omitempty"`
	Error            *ErrorInfo    `json:"error,omitempty"`
}

// Start begins recording a refute invocation and writes its start event.
func Start(args []string, cwd string, opts Options) *Recorder {
	environ := opts.Environ
	if environ == nil {
		environ = os.Environ()
	}
	if envValue(environ, envTelemetry) == "0" {
		return &Recorder{}
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	telemetryPath := opts.TelemetryPath
	if telemetryPath == "" {
		telemetryPath = DefaultPath()
	}
	snapshotRoot := opts.SnapshotRoot
	if snapshotRoot == "" {
		snapshotRoot = DefaultSnapshotRoot()
	}
	sessionRoot := opts.SessionRoot
	if sessionRoot == "" {
		sessionRoot = DefaultSessionRoot()
	}
	started := now().UTC()
	caller := detectCaller(environ)
	r := &Recorder{
		enabled:          telemetryPath != "",
		snapshotsEnabled: envValue(environ, envTelemetrySnapshots) != "0",
		verbose:          opts.Verbose,
		stderr:           stderr,
		now:              now,
		telemetryPath:    telemetryPath,
		snapshotRoot:     snapshotRoot,
		sessionRoot:      sessionRoot,
		invocationID:     newInvocationID(),
		startedAt:        started,
		args:             append([]string(nil), args...),
		cwd:              cwd,
		caller:           caller,
		agent:            detectAgent(environ, caller),
		env:              filteredEnv(environ),
		project:          DetectProject("", cwd),
		phases:           make(map[string]time.Duration),
	}
	if !r.enabled {
		return r
	}
	r.appendJSON(invocationStartEvent{
		SchemaVersion: SchemaVersion,
		Event:         EventInvocationStart,
		Ts:            formatTime(started),
		InvocationID:  r.invocationID,
		StartedAt:     formatTime(started),
		Args:          r.args,
		Cwd:           cwd,
		Caller:        caller,
		Agent:         r.agent,
		Env:           r.env,
		Project:       r.project,
	})
	if r.verbose {
		fmt.Fprintf(r.stderr, "refute start: %s\n", r.commandString())
	}
	return r
}

// Enabled reports whether this recorder writes telemetry.
func (r *Recorder) Enabled() bool {
	return r != nil && r.enabled
}

// SetOperation merges newly discovered operation metadata into the recorder.
func (r *Recorder) SetOperation(ctx OperationContext) {
	if !r.Enabled() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Operation != "" {
		r.ctx.Operation = ctx.Operation
	}
	if ctx.Language != "" {
		r.ctx.Language = ctx.Language
	}
	if ctx.Backend != "" {
		r.ctx.Backend = ctx.Backend
	}
	if ctx.WorkspaceRoot != "" {
		r.ctx.WorkspaceRoot = ctx.WorkspaceRoot
		r.project = DetectProject(ctx.WorkspaceRoot, r.cwd)
	}
	if ctx.Status != "" {
		r.ctx.Status = ctx.Status
	}
	if ctx.FilesModified != 0 {
		r.ctx.FilesModified = ctx.FilesModified
	}
	if len(ctx.Warnings) > 0 {
		r.ctx.Warnings = append(r.ctx.Warnings, ctx.Warnings...)
	}
}

// SetStatus records the operation status without replacing other context.
func (r *Recorder) SetStatus(status string) {
	r.SetOperation(OperationContext{Status: status})
}

// SetDefaultStatus records a status only when the command path has not already
// supplied a more specific result status.
func (r *Recorder) SetDefaultStatus(status string) {
	if !r.Enabled() || status == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Status == "" {
		r.ctx.Status = status
	}
}

// SetError records a compact error summary for the end event.
func (r *Recorder) SetError(code, message string) {
	if !r.Enabled() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.error = &ErrorInfo{Code: code, Message: message}
}

// SetDefaultError records an error only when no more specific error was
// already supplied by the command path.
func (r *Recorder) SetDefaultError(code, message string) {
	if !r.Enabled() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.error == nil {
		r.error = &ErrorInfo{Code: code, Message: message}
	}
}

// StartPhase starts timing a named phase. The returned function is safe to call
// once when the phase completes.
func (r *Recorder) StartPhase(name string) func() {
	if !r.Enabled() || name == "" {
		return func() {}
	}
	start := r.now().UTC()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.addPhase(name, r.now().UTC().Sub(start))
		})
	}
}

// CaptureSnapshot stores compressed before/after snapshots for a workspace edit.
func (r *Recorder) CaptureSnapshot(we *edit.WorkspaceEdit) (*SnapshotManifest, error) {
	if !r.Enabled() || !r.snapshotsEnabled || we == nil {
		return nil, nil
	}
	r.mu.Lock()
	workspaceRoot := r.ctx.WorkspaceRoot
	r.mu.Unlock()
	if workspaceRoot == "" {
		return nil, nil
	}

	done := r.StartPhase("snapshot")
	defer done()
	manifest, err := writeSnapshot(r.snapshotRoot, r.invocationID, workspaceRoot, we)
	if err != nil {
		r.SetOperation(OperationContext{Warnings: []string{"snapshot capture failed: " + err.Error()}})
		return nil, err
	}
	r.mu.Lock()
	r.snapshot = manifest
	r.ctx.FilesModified = len(manifest.Files)
	r.mu.Unlock()
	return manifest, nil
}

// MarkSnapshotApplied records actual post-apply hashes in the snapshot manifest.
func (r *Recorder) MarkSnapshotApplied() {
	if !r.Enabled() {
		return
	}
	r.mu.Lock()
	manifest := r.snapshot
	r.mu.Unlock()
	if manifest == nil {
		return
	}
	if err := finalizeSnapshotActual(manifest); err != nil {
		r.SetOperation(OperationContext{Warnings: []string{"snapshot finalize failed: " + err.Error()}})
	}
}

// Finish writes the invocation end event and any human-readable transcript.
func (r *Recorder) Finish(info FinishInfo) {
	if !r.Enabled() {
		return
	}
	ended := r.now().UTC()

	r.mu.Lock()
	ctx := r.ctx
	project := r.project
	errInfo := info.Error
	if errInfo == nil {
		errInfo = r.error
	}
	status := ctx.Status
	if status == "" {
		if info.ExitCode == 0 {
			status = "succeeded"
		} else {
			status = "failed"
		}
	}
	snapshotPath := ""
	if r.snapshot != nil {
		snapshotPath = r.snapshot.Path
	}
	warnings := append([]string(nil), ctx.Warnings...)
	phases := r.phaseTimingsLocked()
	r.mu.Unlock()

	event := invocationEndEvent{
		SchemaVersion:    SchemaVersion,
		Event:            EventInvocationEnd,
		Ts:               formatTime(ended),
		InvocationID:     r.invocationID,
		StartedAt:        formatTime(r.startedAt),
		EndedAt:          formatTime(ended),
		DurationMS:       durationMS(ended.Sub(r.startedAt)),
		ExitCode:         info.ExitCode,
		Status:           status,
		Args:             r.args,
		Cwd:              r.cwd,
		Caller:           r.caller,
		Agent:            r.agent,
		Operation:        ctx.Operation,
		Language:         ctx.Language,
		Backend:          ctx.Backend,
		WorkspaceRoot:    ctx.WorkspaceRoot,
		Project:          project,
		PhaseTimings:     phases,
		EditStats:        EditStats{FilesModified: ctx.FilesModified},
		FilesModified:    ctx.FilesModified,
		SnapshotManifest: snapshotPath,
		Warnings:         warnings,
		Error:            errInfo,
	}
	r.appendJSON(event)
	summary := r.summary(status, info.ExitCode, ended.Sub(r.startedAt), snapshotPath)
	r.writeTranscript(summary)
	if r.verbose {
		fmt.Fprint(r.stderr, summary)
	}
}

func (r *Recorder) addPhase(name string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.phases[name]; !ok {
		r.phaseOrder = append(r.phaseOrder, name)
	}
	r.phases[name] += d
}

func (r *Recorder) phaseTimingsLocked() []PhaseTiming {
	names := append([]string(nil), r.phaseOrder...)
	if len(names) == 0 && len(r.phases) > 0 {
		for name := range r.phases {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	out := make([]PhaseTiming, 0, len(names))
	for _, name := range names {
		out = append(out, PhaseTiming{Name: name, DurationMS: durationMS(r.phases[name])})
	}
	return out
}

func (r *Recorder) appendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	appendLine(r.telemetryPath, data)
}

func (r *Recorder) writeTranscript(summary string) {
	if r.sessionRoot == "" || r.agent.SessionID == "" {
		return
	}
	caller := r.caller
	if caller == "" {
		caller = "unknown"
	}
	path := filepath.Join(r.sessionRoot, safePathPart(caller), safePathPart(r.agent.SessionID)+".log")
	appendLine(path, []byte(summary))
}

func (r *Recorder) summary(status string, exitCode int, duration time.Duration, snapshotPath string) string {
	project := r.project.DisplayName()
	if project == "" {
		project = r.cwd
	}
	lines := []string{
		fmt.Sprintf("[%s] %s", formatTime(r.now().UTC()), r.commandString()),
		fmt.Sprintf("project: %s", project),
		fmt.Sprintf("status: %s exit=%d durationMs=%d", status, exitCode, durationMS(duration)),
	}
	if r.ctx.Operation != "" || r.ctx.Language != "" || r.ctx.Backend != "" {
		lines = append(lines, fmt.Sprintf("operation: %s language=%s backend=%s", r.ctx.Operation, r.ctx.Language, r.ctx.Backend))
	}
	lines = append(lines, fmt.Sprintf("files: %d", r.ctx.FilesModified))
	if snapshotPath != "" {
		lines = append(lines, fmt.Sprintf("snapshot: %s", snapshotPath))
	}
	if r.error != nil {
		lines = append(lines, fmt.Sprintf("error: %s %s", r.error.Code, r.error.Message))
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func (r *Recorder) commandString() string {
	if len(r.args) == 0 {
		return "refute"
	}
	return "refute " + strings.Join(r.args, " ")
}

func appendLine(path string, data []byte) {
	if path == "" {
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

func newInvocationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func detectAgent(environ []string, caller string) AgentInfo {
	return AgentInfo{
		Caller:     caller,
		SessionID:  firstEnv(environ, "CLAUDE_CODE_SESSION_ID", "CLAUDE_SESSION_ID", "CODEX_SESSION_ID", "CURSOR_SESSION_ID", "GOOSE_SESSION_ID", "GITHUB_RUN_ID"),
		Entrypoint: firstEnv(environ, "CLAUDE_CODE_ENTRYPOINT", "CODEX_ENTRYPOINT"),
		ExecPath:   firstEnv(environ, "CLAUDE_CODE_EXECPATH", "CODEX_EXECPATH"),
		Effort:     firstEnv(environ, "CLAUDE_EFFORT", "CODEX_EFFORT"),
	}
}

func envValue(environ []string, key string) string {
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if ok && k == key {
			return v
		}
	}
	return ""
}

func firstEnv(environ []string, keys ...string) string {
	for _, key := range keys {
		if isSecret(key) {
			continue
		}
		if v := envValue(environ, key); v != "" {
			return v
		}
	}
	return ""
}

func safePathPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out == "" || out == "." || out == ".." {
		return "unknown"
	}
	return out
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func durationMS(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}
