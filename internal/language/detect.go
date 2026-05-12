package language

import "path/filepath"

// Detection describes the language metadata inferred from a source file path.
type Detection struct {
	// Language is the backend/config language key used by backend selection.
	Language string
	// LanguageID is the LSP languageId used in textDocument/didOpen.
	LanguageID string
	// CLIConfigKey is the legacy config key reported by CLI-only helpers.
	CLIConfigKey string
}

// Detect returns language metadata for filePath based on its extension.
func Detect(filePath string) Detection {
	switch filepath.Ext(filePath) {
	case ".go":
		return Detection{Language: "go", LanguageID: "go", CLIConfigKey: "go"}
	case ".ts":
		return Detection{Language: "typescript", LanguageID: "typescript", CLIConfigKey: "typescript"}
	case ".tsx":
		return Detection{Language: "typescript", LanguageID: "typescriptreact", CLIConfigKey: "typescript"}
	case ".js":
		return Detection{Language: "javascript", LanguageID: "javascript", CLIConfigKey: "typescript"}
	case ".jsx":
		return Detection{Language: "javascript", LanguageID: "javascriptreact", CLIConfigKey: "typescript"}
	case ".py":
		return Detection{Language: "python", LanguageID: "python", CLIConfigKey: "python"}
	case ".java":
		return Detection{Language: "java", LanguageID: "java", CLIConfigKey: "java"}
	case ".kt":
		return Detection{Language: "kotlin", LanguageID: "kotlin", CLIConfigKey: "kotlin"}
	case ".rs":
		return Detection{Language: "rust", LanguageID: "rust", CLIConfigKey: "rust"}
	case ".cs":
		return Detection{Language: "csharp", LanguageID: "csharp", CLIConfigKey: "csharp"}
	default:
		return Detection{}
	}
}
