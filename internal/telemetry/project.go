package telemetry

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProjectInfo captures best-effort local project identity for audit records.
type ProjectInfo struct {
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	GitRoot       string `json:"gitRoot,omitempty"`
	GitWorktree   string `json:"gitWorktree,omitempty"`
	GitRemote     string `json:"gitRemote,omitempty"`
	GitSlug       string `json:"gitSlug,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Head          string `json:"head,omitempty"`
	Dirty         *bool  `json:"dirty,omitempty"`
	Module        string `json:"module,omitempty"`
}

// DetectProject infers a stable project identity from workspaceRoot, falling
// back to cwd. It never returns errors because telemetry must not affect CLI
// behavior.
func DetectProject(workspaceRoot, cwd string) ProjectInfo {
	base := workspaceRoot
	if base == "" {
		base = cwd
	}
	if base == "" {
		return ProjectInfo{}
	}
	abs, err := filepath.Abs(base)
	if err == nil {
		base = filepath.Clean(abs)
	}

	info := ProjectInfo{WorkspaceRoot: cleanAbs(workspaceRoot)}
	if info.WorkspaceRoot == "" {
		info.WorkspaceRoot = base
	}
	info.Module = detectModule(base)

	if gitRoot := gitOutput(base, "rev-parse", "--show-toplevel"); gitRoot != "" {
		info.GitRoot = gitRoot
		info.GitWorktree = gitRoot
		if remote := gitOutput(gitRoot, "remote", "get-url", "origin"); remote != "" {
			info.GitRemote = remote
			info.GitSlug = remoteSlug(remote)
		}
		info.Branch = gitOutput(gitRoot, "branch", "--show-current")
		info.Head = gitOutput(gitRoot, "rev-parse", "HEAD")
		dirty := gitOutput(gitRoot, "status", "--porcelain") != ""
		info.Dirty = &dirty
		if info.Module == "" {
			info.Module = detectModule(gitRoot)
		}
	}
	return info
}

// DisplayName returns the most concise useful project label.
func (p ProjectInfo) DisplayName() string {
	switch {
	case p.GitSlug != "":
		return p.GitSlug
	case p.Module != "":
		return p.Module
	case p.WorkspaceRoot != "":
		return p.WorkspaceRoot
	default:
		return ""
	}
}

func gitOutput(dir string, args ...string) string {
	allArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", allArgs...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectModule(start string) string {
	dir := start
	info, err := os.Stat(dir)
	if err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		path := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(path); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[0] == "module" {
					return fields[1]
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func cleanAbs(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func remoteSlug(remote string) string {
	trimmed := strings.TrimSuffix(remote, ".git")
	if strings.HasPrefix(trimmed, "git@") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			return lastTwoPathParts(parts[1])
		}
	}
	if strings.Contains(trimmed, "://") {
		if i := strings.Index(trimmed, "://"); i >= 0 {
			trimmed = trimmed[i+3:]
		}
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) == 2 {
			return lastTwoPathParts(parts[1])
		}
	}
	return lastTwoPathParts(trimmed)
}

func lastTwoPathParts(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return ""
}
