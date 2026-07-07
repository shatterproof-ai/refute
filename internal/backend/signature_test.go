package backend

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDecodeSignatureParams_RejectsRemoveFromIndexMinusOne asserts the core
// invariant: a "remove" edit must carry the parameter's real 0-based original
// position. FromIndex -1 is the sentinel reserved for "add" (no original
// position), so it is not a valid origin for a removal and must be rejected at
// decode time rather than silently misread as a parameter reference.
func TestDecodeSignatureParams_RejectsRemoveFromIndexMinusOne(t *testing.T) {
	raw := mustMarshal(t, SignatureParams{
		Parameters: []ParamEdit{{Op: ParamOpRemove, FromIndex: -1, ToIndex: -1}},
	})
	_, err := DecodeSignatureParams(raw)
	if err == nil {
		t.Fatal("expected DecodeSignatureParams to reject a remove edit with fromIndex -1")
	}
	if !strings.Contains(err.Error(), "fromIndex") {
		t.Errorf("error should mention fromIndex, got: %v", err)
	}
}

func TestDecodeSignatureParams_Validation(t *testing.T) {
	tests := []struct {
		name    string
		edit    ParamEdit
		wantErr bool
	}{
		{"remove with real index", ParamEdit{Op: ParamOpRemove, FromIndex: 1, ToIndex: -1}, false},
		{"remove with fromIndex -1", ParamEdit{Op: ParamOpRemove, FromIndex: -1, ToIndex: -1}, true},
		{"remove with non -1 toIndex", ParamEdit{Op: ParamOpRemove, FromIndex: 1, ToIndex: 0}, true},
		{"add with fromIndex -1", ParamEdit{Op: ParamOpAdd, FromIndex: -1, ToIndex: 2}, false},
		{"add with real fromIndex", ParamEdit{Op: ParamOpAdd, FromIndex: 0, ToIndex: 2}, true},
		{"add with -1 toIndex", ParamEdit{Op: ParamOpAdd, FromIndex: -1, ToIndex: -1}, true},
		{"keep with both indices", ParamEdit{Op: ParamOpKeep, FromIndex: 0, ToIndex: 0}, false},
		{"keep with fromIndex -1", ParamEdit{Op: ParamOpKeep, FromIndex: -1, ToIndex: 0}, true},
		{"reorder with both indices", ParamEdit{Op: ParamOpReorder, FromIndex: 2, ToIndex: 0}, false},
		{"reorder with toIndex -1", ParamEdit{Op: ParamOpReorder, FromIndex: 2, ToIndex: -1}, true},
		{"unknown op", ParamEdit{Op: "swap", FromIndex: 0, ToIndex: 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := mustMarshal(t, SignatureParams{Parameters: []ParamEdit{tt.edit}})
			_, err := DecodeSignatureParams(raw)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %+v, got nil", tt.edit)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %+v: %v", tt.edit, err)
			}
		})
	}
}

func mustMarshal(t *testing.T, p SignatureParams) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshaling params: %v", err)
	}
	return raw
}
