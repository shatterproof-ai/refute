package toolchain

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

const (
	LockfileName  = "refute.lock.json"
	ToolRoot      = ".refute"
	ActiveBinPath = ".refute/bin/refute"
)

// Lock is the committed project contract used by registryless package-manager adapters.
type Lock struct {
	Version     string     `json:"version"`
	ManifestURL string     `json:"manifest_url"`
	Artifacts   []Artifact `json:"artifacts"`
}

type Artifact struct {
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size,omitempty"`
	Filename     string `json:"filename,omitempty"`
}

func LoadLock(path string) (Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Lock{}, fmt.Errorf("read %s: %w", path, err)
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return Lock{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if lock.Version == "" {
		return Lock{}, fmt.Errorf("%s: version is required", path)
	}
	if lock.ManifestURL == "" {
		return Lock{}, fmt.Errorf("%s: manifest_url is required", path)
	}
	if len(lock.Artifacts) == 0 {
		return Lock{}, fmt.Errorf("%s: at least one artifact is required", path)
	}
	for i, artifact := range lock.Artifacts {
		if artifact.Platform == "" || artifact.Architecture == "" || artifact.URL == "" || artifact.SHA256 == "" {
			return Lock{}, fmt.Errorf("%s: artifact %d requires platform, architecture, url, and sha256", path, i)
		}
	}
	return lock, nil
}

func SelectArtifact(lock Lock, platform, architecture string) (Artifact, error) {
	for _, artifact := range lock.Artifacts {
		if artifact.Platform == platform && artifact.Architecture == architecture {
			return artifact, nil
		}
	}
	return Artifact{}, fmt.Errorf("unsupported platform %s/%s for refute %s", platform, architecture, lock.Version)
}

func CurrentPlatform() (string, string) {
	return runtime.GOOS, runtime.GOARCH
}
