package telemetry

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Retention bounds keep on-disk telemetry from growing without limit. They are
// documented in the README "Telemetry retention & opt-out" section. The sweep
// runs at Finish and is always best-effort: any error is ignored so retention
// can never affect a refactoring operation.
const (
	// MaxSnapshotInvocations caps how many snapshot invocation directories are
	// retained under the snapshot root. The newest directories are kept.
	MaxSnapshotInvocations = 200

	// MaxSnapshotBytes caps the total bytes retained across snapshot
	// directories. Newest-first, directories are dropped once this budget would
	// be exceeded, even if the count is under MaxSnapshotInvocations.
	MaxSnapshotBytes int64 = 200 * 1024 * 1024 // 200 MiB

	// MaxTelemetryLogBytes caps telemetry.jsonl. When the file exceeds this
	// size it is rotated to telemetry.jsonl.1 (one generation retained).
	MaxTelemetryLogBytes int64 = 50 * 1024 * 1024 // 50 MiB
)

// sweepSnapshots enforces snapshot retention under root. It keeps the most
// recently modified invocation directories, removing the oldest ones once
// either maxDirs or maxBytes would be exceeded. It is best-effort: filesystem
// errors are ignored.
func sweepSnapshots(root string, maxDirs int, maxBytes int64) {
	if root == "" {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type dirInfo struct {
		path    string
		modUnix int64
		size    int64
	}
	dirs := make([]dirInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(root, e.Name())
		dirs = append(dirs, dirInfo{path: p, modUnix: info.ModTime().UnixNano(), size: dirSize(p)})
	}
	// Newest first so the budget is spent on the most recent invocations.
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].modUnix > dirs[j].modUnix })

	var kept int64
	for i, d := range dirs {
		remove := false
		switch {
		case maxDirs > 0 && i >= maxDirs:
			remove = true
		case maxBytes > 0 && kept+d.size > maxBytes:
			remove = true
		}
		if remove {
			_ = os.RemoveAll(d.path)
			continue
		}
		kept += d.size
	}
}

// dirSize returns the total size of regular files under path, best-effort.
func dirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// rotateTelemetryLog rotates path to path+".1" once it exceeds maxBytes, keeping
// a single previous generation. It is best-effort: errors are ignored.
func rotateTelemetryLog(path string, maxBytes int64) {
	if path == "" || maxBytes <= 0 {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() <= maxBytes {
		return
	}
	// os.Rename atomically replaces any existing .1 backup on POSIX systems.
	_ = os.Rename(path, path+".1")
}
