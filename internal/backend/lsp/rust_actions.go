package lsp

import (
	"fmt"
	"regexp"
	"strings"
)

// rustActionOp names a logical refactoring operation. The same op can be
// satisfied by different rust-analyzer CodeAction kinds + titles across
// versions.
type rustActionOp int

const (
	opExtractFunction rustActionOp = iota
	opExtractVariable
	opInlineCallSite
	opInlineAllCallers
)

func (op rustActionOp) String() string {
	switch op {
	case opExtractFunction:
		return "opExtractFunction"
	case opExtractVariable:
		return "opExtractVariable"
	case opInlineCallSite:
		return "opInlineCallSite"
	case opInlineAllCallers:
		return "opInlineAllCallers"
	}
	return fmt.Sprintf("rustActionOp(%d)", int(op))
}

type rustActionPattern struct {
	kindPrefix  string
	titleRegexp *regexp.Regexp
}

var rustActionPatterns = map[rustActionOp]rustActionPattern{
	opExtractFunction:  {"refactor.extract", regexp.MustCompile(`(?i)extract .*function`)},
	opExtractVariable:  {"refactor.extract", regexp.MustCompile(`(?i)extract .*variable`)},
	opInlineCallSite:   {"refactor.inline", regexp.MustCompile(`(?i)^inline( call)?$`)},
	opInlineAllCallers: {"refactor.inline", regexp.MustCompile(`(?i)inline .*all callers`)},
}

// ErrActionNotOffered is returned when no code action matches the requested op.
type ErrActionNotOffered struct {
	Op            rustActionOp
	OfferedTitles []string
}

func (e *ErrActionNotOffered) Error() string {
	return fmt.Sprintf("rust-analyzer offered no action matching %s. Offered titles: %v",
		e.Op, e.OfferedTitles)
}

// matchRustAction finds the first action whose Kind starts with the expected
// prefix and whose Title matches the expected regex.
func matchRustAction(actions []CodeAction, op rustActionOp) (*CodeAction, error) {
	pat, ok := rustActionPatterns[op]
	if !ok {
		return nil, fmt.Errorf("no pattern registered for %s", op)
	}
	offered := make([]string, 0, len(actions))
	for i := range actions {
		offered = append(offered, actions[i].Title)
		if !strings.HasPrefix(actions[i].Kind, pat.kindPrefix) {
			continue
		}
		if pat.titleRegexp.MatchString(actions[i].Title) {
			return &actions[i], nil
		}
	}
	return nil, &ErrActionNotOffered{Op: op, OfferedTitles: offered}
}
