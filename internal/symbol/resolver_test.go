package symbol_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestResolve_Tier3_ExactPosition(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc hello() {}\n"), 0644)

	query := symbol.Query{
		File:   filePath,
		Line:   3,
		Column: 6,
	}

	loc, err := symbol.Resolve(query)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if loc.Line != 3 || loc.Column != 6 {
		t.Errorf("expected line=3 col=6, got line=%d col=%d", loc.Line, loc.Column)
	}
	if loc.File != filePath {
		t.Errorf("expected file %s, got %s", filePath, loc.File)
	}
}

func TestResolve_Tier2_FindNameOnLine(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc helloWorld() {}\n"), 0644)

	query := symbol.Query{
		File: filePath,
		Line: 3,
		Name: "helloWorld",
	}

	loc, err := symbol.Resolve(query)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if loc.Column != 6 { // "func " = 5 chars, "helloWorld" starts at col 6 (1-indexed)
		t.Errorf("expected column 6, got %d", loc.Column)
	}
	if loc.Name != "helloWorld" {
		t.Errorf("expected name helloWorld, got %s", loc.Name)
	}
}

func TestResolve_Tier2_NameNotFound(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc hello() {}\n"), 0644)

	query := symbol.Query{
		File: filePath,
		Line: 3,
		Name: "nonexistent",
	}

	_, err := symbol.Resolve(query)
	if err == nil {
		t.Fatal("expected error for name not found on line")
	}
}

func TestResolve_Tier2_MultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	// "x" appears twice on this line: as parameter and in body.
	os.WriteFile(filePath, []byte("package main\n\nfunc f(x int) int { return x }\n"), 0644)

	query := symbol.Query{
		File: filePath,
		Line: 3,
		Name: "x",
	}

	_, err := symbol.Resolve(query)
	if err == nil {
		t.Fatal("expected error for multiple same-line occurrences")
	}
}

func TestResolve_Tier2_RejectsPartialIdentifier(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc oldNameExtra() {}\n"), 0644)

	query := symbol.Query{
		File: filePath,
		Line: 3,
		Name: "oldName",
	}

	_, err := symbol.Resolve(query)
	if err == nil {
		t.Fatal("expected error for partial identifier match")
	}
}

func TestResolve_Tier2_IgnoresCommentsAndStrings(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc f() { println(\"target\") /* target */ }\n"), 0644)

	query := symbol.Query{
		File: filePath,
		Line: 3,
		Name: "target",
	}

	_, err := symbol.Resolve(query)
	if err == nil {
		t.Fatal("expected error when name appears only in comments and strings")
	}
}

func TestResolve_InvalidTier(t *testing.T) {
	query := symbol.Query{} // No fields set.
	_, err := symbol.Resolve(query)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}
