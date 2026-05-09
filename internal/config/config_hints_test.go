package config

import "testing"

func TestInstallHint(t *testing.T) {
	cases := map[string]string{
		"rust":       "rustup component add rust-analyzer",
		"go":         "go install golang.org/x/tools/gopls@latest",
		"typescript": "npm install -g typescript-language-server typescript",
		"javascript": "npm install -g typescript-language-server typescript",
		"python":     "pip install pyright",
		"cobol":      "",
	}
	for lang, want := range cases {
		if got := InstallHint(lang); got != want {
			t.Errorf("InstallHint(%q) = %q, want %q", lang, got, want)
		}
	}
}
