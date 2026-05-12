package language

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		language   string
		languageID string
		cliKey     string
	}{
		{
			name:       "typescript react",
			path:       "src/component.tsx",
			language:   "typescript",
			languageID: "typescriptreact",
			cliKey:     "typescript",
		},
		{
			name:       "javascript react",
			path:       "src/component.jsx",
			language:   "javascript",
			languageID: "javascriptreact",
			cliKey:     "typescript",
		},
		{
			name:       "go",
			path:       "main.go",
			language:   "go",
			languageID: "go",
			cliKey:     "go",
		},
		{
			name: "unknown",
			path: "README.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect(tt.path)
			if got.Language != tt.language {
				t.Fatalf("Language = %q, want %q", got.Language, tt.language)
			}
			if got.LanguageID != tt.languageID {
				t.Fatalf("LanguageID = %q, want %q", got.LanguageID, tt.languageID)
			}
			if got.CLIConfigKey != tt.cliKey {
				t.Fatalf("CLIConfigKey = %q, want %q", got.CLIConfigKey, tt.cliKey)
			}
		})
	}
}
