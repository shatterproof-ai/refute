package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

const (
	defaultTimeout           = 30000
	defaultDaemonIdleTimeout = 600000
	projectConfigFilename    = "refute.config.json"
	userConfigRelPath        = ".config/refute/config.json"
)

// ServerConfig holds the command and arguments for a language server.
type ServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// DaemonConfig holds settings for the refute daemon.
type DaemonConfig struct {
	AutoStart   bool `json:"autoStart"`
	IdleTimeout int  `json:"idleTimeout"`
}

// Config is the resolved configuration for refute.
type Config struct {
	Servers map[string]ServerConfig `json:"servers"`
	Timeout int                     `json:"timeout"`
	Daemon  DaemonConfig            `json:"daemon"`
}

// builtinServers defines the default language-server configurations shipped
// with refute.
var builtinServers = map[string]ServerConfig{
	"go": {
		Command: "gopls",
		Args:    []string{"serve"},
	},
	"rust": {
		Command: "rust-analyzer",
	},
	"typescript": {
		Command: "typescript-language-server",
		Args:    []string{"--stdio"},
	},
	"javascript": {
		Command: "typescript-language-server",
		Args:    []string{"--stdio"},
	},
	"python": {
		Command: "pyright-langserver",
		Args:    []string{"--stdio"},
	},
	"java": {
		Command: "jdtls",
		Args:    []string{},
	},
	"kotlin": {
		Command: "kotlin-language-server",
		Args:    []string{},
	},
}

// defaults returns a Config populated entirely with built-in defaults.
func defaults() *Config {
	return &Config{
		Servers: make(map[string]ServerConfig),
		Timeout: defaultTimeout,
		Daemon: DaemonConfig{
			AutoStart:   false,
			IdleTimeout: defaultDaemonIdleTimeout,
		},
	}
}

// fileLayer is the JSON shape of a config file.  Fields are pointers or maps
// so we can distinguish "not present" from "zero value" during merge.
type fileLayer struct {
	Servers map[string]ServerConfig `json:"servers"`
	Timeout *int                    `json:"timeout"`
	Daemon  *daemonLayer            `json:"daemon"`
}

type daemonLayer struct {
	AutoStart   *bool `json:"autoStart"`
	IdleTimeout *int  `json:"idleTimeout"`
}

// mergeLayer applies the non-zero values from a file layer onto dst.
func mergeLayer(dst *Config, layer fileLayer) {
	for lang, srv := range layer.Servers {
		dst.Servers[lang] = srv
	}
	if layer.Timeout != nil {
		dst.Timeout = *layer.Timeout
	}
	if layer.Daemon != nil {
		if layer.Daemon.AutoStart != nil {
			dst.Daemon.AutoStart = *layer.Daemon.AutoStart
		}
		if layer.Daemon.IdleTimeout != nil {
			dst.Daemon.IdleTimeout = *layer.Daemon.IdleTimeout
		}
	}
}

// loadFile reads and parses one config file layer.  If the file does not
// exist the returned layer is empty and err is nil (silently skipped).
func loadFile(path string) (fileLayer, error) {
	var layer fileLayer
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return layer, nil
	}
	if err != nil {
		return layer, err
	}
	if err := json.Unmarshal(data, &layer); err != nil {
		return layer, err
	}
	return layer, nil
}

// Server returns the ServerConfig for a language, falling back to the
// built-in default when the language is not present in the resolved config.
func (c *Config) Server(language string) ServerConfig {
	if srv, ok := c.Servers[language]; ok {
		return srv
	}
	if srv, ok := builtinServers[language]; ok {
		return srv
	}
	return ServerConfig{}
}

// ResolvedServer returns the effective ServerConfig for a language in a
// workspace. Explicit config wins. For TypeScript/JavaScript, a local
// node_modules/.bin/typescript-language-server takes precedence over the
// built-in default when no explicit config is set.
func (c *Config) ResolvedServer(language string, workspaceRoot string) ServerConfig {
	if srv, ok := c.Servers[language]; ok {
		return srv
	}
	if local, ok := localTypeScriptServer(language, workspaceRoot); ok {
		return local
	}
	return c.Server(language)
}

func localTypeScriptServer(language string, workspaceRoot string) (ServerConfig, bool) {
	if workspaceRoot == "" {
		return ServerConfig{}, false
	}
	switch language {
	case "typescript", "javascript":
	default:
		return ServerConfig{}, false
	}
	name := "typescript-language-server"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	path := filepath.Join(workspaceRoot, "node_modules", ".bin", name)
	if _, err := os.Stat(path); err != nil {
		return ServerConfig{}, false
	}
	return ServerConfig{
		Command: path,
		Args:    []string{"--stdio"},
	}, true
}

// Load builds a Config by applying layers in ascending priority:
//
//  1. Built-in defaults
//  2. ~/.config/refute/config.json  (user global)
//  3. <workspaceRoot>/refute.config.json  (project)
//  4. explicitPath  (flag / env override)
//
// Missing files are silently skipped.  An error is returned only for files
// that exist but cannot be read or parsed.
func Load(explicitPath string, workspaceRoot string) (*Config, error) {
	cfg := defaults()

	// User-level config: ~/.config/refute/config.json
	home, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(home, userConfigRelPath)
		layer, err := loadFile(userPath)
		if err != nil {
			return nil, err
		}
		mergeLayer(cfg, layer)
	}

	// Project-level config: <workspaceRoot>/refute.config.json
	if workspaceRoot != "" {
		projectPath := filepath.Join(workspaceRoot, projectConfigFilename)
		layer, err := loadFile(projectPath)
		if err != nil {
			return nil, err
		}
		mergeLayer(cfg, layer)
	}

	// Explicit path (highest priority).
	if explicitPath != "" {
		layer, err := loadFile(explicitPath)
		if err != nil {
			return nil, err
		}
		mergeLayer(cfg, layer)
	}

	return cfg, nil
}
