package lsp

import "testing"

func TestMatchRustAction_ByKindAndTitle(t *testing.T) {
	actions := []CodeAction{
		{Kind: "refactor.extract.function", Title: "Extract into function"},
		{Kind: "refactor.extract.variable", Title: "Extract into variable"},
		{Kind: "refactor.inline.call", Title: "Inline"},
		{Kind: "refactor.inline.callers", Title: "Inline into all callers"},
		{Kind: "quickfix", Title: "Add `use std::fmt;`"},
	}
	cases := []struct {
		op       rustActionOp
		wantKind string
	}{
		{opExtractFunction, "refactor.extract.function"},
		{opExtractVariable, "refactor.extract.variable"},
		{opInlineCallSite, "refactor.inline.call"},
		{opInlineAllCallers, "refactor.inline.callers"},
	}
	for _, c := range cases {
		got, err := matchRustAction(actions, c.op)
		if err != nil {
			t.Errorf("op %v: %v", c.op, err)
			continue
		}
		if got.Kind != c.wantKind {
			t.Errorf("op %v: got Kind=%q, want %q", c.op, got.Kind, c.wantKind)
		}
	}
}

func TestMatchRustAction_NotOffered(t *testing.T) {
	actions := []CodeAction{
		{Kind: "quickfix", Title: "Add `use std::fmt;`"},
	}
	if _, err := matchRustAction(actions, opExtractFunction); err == nil {
		t.Error("expected error when no matching action present")
	}
}

func TestMatchRustAction_KindPrefix(t *testing.T) {
	// Some rust-analyzer versions emit kind="refactor.extract" without a suffix.
	actions := []CodeAction{
		{Kind: "refactor.extract", Title: "Extract into function"},
	}
	got, err := matchRustAction(actions, opExtractFunction)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Extract into function" {
		t.Errorf("got Title=%q", got.Title)
	}
}
