package lsp

import (
	"strings"
	"testing"
)

func TestByteColumnToUTF16Character(t *testing.T) {
	line := `const label = "é𝄞"; target := 1`
	byteColumn := strings.Index(line, "target") + 1
	got, err := byteColumnToUTF16Character(line, byteColumn)
	if err != nil {
		t.Fatalf("byteColumnToUTF16Character: %v", err)
	}
	want := 21
	if got != want {
		t.Fatalf("expected UTF-16 character %d, got %d", want, got)
	}
}

func TestReplaceWholeIdent_respectsIdentifierBoundaries(t *testing.T) {
	got := replaceWholeIdent("newFunction()\nnewFunctionCall()\n_ = newFunction", "newFunction", "sum")
	want := "sum()\nnewFunctionCall()\n_ = sum"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
