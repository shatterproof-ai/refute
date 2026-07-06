package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// OperationChangeSignature is the capability string for the change-signature
// operation. It is shared by the profile registry, the support matrix, the
// selector, and the CLI so the operation name cannot drift between surfaces.
const OperationChangeSignature = "change-signature"

// Parameter-edit operation kinds for SignatureParams.ParamEdit.Op. The full set
// is defined so the declarative model generalizes to reorder and add work; the
// initial change-signature implementation supports only ParamOpRemove.
const (
	ParamOpKeep    = "keep"
	ParamOpAdd     = "add"
	ParamOpRemove  = "remove"
	ParamOpReorder = "reorder"
)

// SignatureParams is the backend-agnostic parameter object for change-signature,
// decoded from RefactorRequest.Params. It describes the desired final parameter
// list as a list of edits keyed to original positions plus an optional
// return-type change; the backend computes the declaration and call-site
// rewrites (docs/plans/refactoring-extension-model.md §3.3).
type SignatureParams struct {
	// Parameters is the list of per-parameter edits.
	Parameters []ParamEdit `json:"parameters"`
	// ReturnType, when set, requests a return-type change. Not yet supported by
	// any backend; a non-nil value is refused.
	ReturnType *string `json:"returnType,omitempty"`
}

// ParamEdit describes one parameter's edit within a signature change.
type ParamEdit struct {
	// Op is one of the ParamOp* kinds.
	Op string `json:"op"`
	// FromIndex is the parameter's original 0-based position ("keep", "remove",
	// "reorder"); -1 for "add".
	FromIndex int `json:"fromIndex"`
	// ToIndex is the parameter's final 0-based position ("keep", "add",
	// "reorder"); -1 for "remove".
	ToIndex int `json:"toIndex"`
	// Name / Type / Default describe an added parameter.
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Default string `json:"default,omitempty"`
}

// DecodeSignatureParams decodes and validates the Params of a change-signature
// request. It rejects malformed JSON and unknown fields at decode time (per the
// design's rationale for typed decoding over map[string]any), returning a
// precise error rather than surfacing a nil-map panic deep in a backend.
func DecodeSignatureParams(raw json.RawMessage) (SignatureParams, error) {
	var p SignatureParams
	if len(raw) == 0 {
		return p, fmt.Errorf("change-signature requires a params object")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, fmt.Errorf("decoding change-signature params: %w", err)
	}
	if len(p.Parameters) == 0 {
		return p, fmt.Errorf("change-signature requires at least one parameter edit")
	}
	return p, nil
}
