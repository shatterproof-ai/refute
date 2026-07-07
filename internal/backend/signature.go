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
//
// The FromIndex/ToIndex invariants below are enforced by DecodeSignatureParams
// so a contradictory edit (e.g. a "remove" carrying FromIndex -1, which would
// collide with the "add" sentinel) is rejected at decode time rather than being
// silently misread by a backend or caller that trusts these fields.
type ParamEdit struct {
	// Op is one of the ParamOp* kinds.
	Op string `json:"op"`
	// FromIndex is the parameter's original 0-based position; required for
	// "keep", "remove", and "reorder". It is -1 for "add", which has no original
	// position.
	FromIndex int `json:"fromIndex"`
	// ToIndex is the parameter's final 0-based position; required for "keep",
	// "add", and "reorder". It is -1 for "remove", which has no final position.
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
	if err := p.validate(); err != nil {
		return p, fmt.Errorf("invalid change-signature params: %w", err)
	}
	return p, nil
}

// validate enforces the per-op FromIndex/ToIndex invariants documented on
// ParamEdit. It rejects an edit whose indices contradict its Op — most notably a
// "remove" (or "keep"/"reorder") that supplies FromIndex -1, which is reserved
// as the "no original position" sentinel for "add". Without this check such an
// edit decodes cleanly and a backend that consults FromIndex silently acts on
// the wrong (or a nonexistent) parameter.
func (p SignatureParams) validate() error {
	for i, e := range p.Parameters {
		switch e.Op {
		case ParamOpAdd:
			if e.FromIndex != -1 {
				return fmt.Errorf("parameter edit %d: op %q requires fromIndex -1 (an added parameter has no original position), got %d", i, e.Op, e.FromIndex)
			}
			if e.ToIndex < 0 {
				return fmt.Errorf("parameter edit %d: op %q requires a final 0-based toIndex, got %d", i, e.Op, e.ToIndex)
			}
		case ParamOpRemove:
			if e.FromIndex < 0 {
				return fmt.Errorf("parameter edit %d: op %q requires the original 0-based fromIndex, got %d", i, e.Op, e.FromIndex)
			}
			if e.ToIndex != -1 {
				return fmt.Errorf("parameter edit %d: op %q requires toIndex -1 (a removed parameter has no final position), got %d", i, e.Op, e.ToIndex)
			}
		case ParamOpKeep, ParamOpReorder:
			if e.FromIndex < 0 {
				return fmt.Errorf("parameter edit %d: op %q requires the original 0-based fromIndex, got %d", i, e.Op, e.FromIndex)
			}
			if e.ToIndex < 0 {
				return fmt.Errorf("parameter edit %d: op %q requires the final 0-based toIndex, got %d", i, e.Op, e.ToIndex)
			}
		default:
			return fmt.Errorf("parameter edit %d: unknown op %q (want one of %q, %q, %q, %q)", i, e.Op, ParamOpKeep, ParamOpAdd, ParamOpRemove, ParamOpReorder)
		}
	}
	return nil
}
