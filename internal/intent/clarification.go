package intent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const ClarificationPrompt = `Classify the player's reply to a pending clarification.
Return only the requested JSON object. Candidates are numbered from one.
Use select for one named or numbered candidate, all for every candidate, confirm for an explicit confirmation, cancel for a rejection, and new_command for an unrelated command.
Never invent a candidate or identifier.`

type CandidateView struct {
	Ordinal int      `json:"ordinal"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

type ClarificationKind string

const (
	ClarificationSelect     ClarificationKind = "select"
	ClarificationAll        ClarificationKind = "all"
	ClarificationConfirm    ClarificationKind = "confirm"
	ClarificationCancel     ClarificationKind = "cancel"
	ClarificationNewCommand ClarificationKind = "new_command"
)

type ClarificationDecision struct {
	Kind    ClarificationKind
	Mention string
	Ordinal int
}

type clarificationDecisionJSON struct {
	Decision *ClarificationKind `json:"decision"`
	Mention  *string            `json:"mention"`
	Ordinal  *int               `json:"ordinal"`
}

func parseClarificationDecision(raw string, candidates []CandidateView) (ClarificationDecision, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire clarificationDecisionJSON
	if err := decoder.Decode(&wire); err != nil {
		return ClarificationDecision{}, fmt.Errorf("decode clarification decision: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return ClarificationDecision{}, fmt.Errorf("decode clarification decision: trailing JSON value")
		}
		return ClarificationDecision{}, fmt.Errorf("decode clarification decision trailing data: %w", err)
	}
	if wire.Decision == nil || wire.Mention == nil || wire.Ordinal == nil {
		return ClarificationDecision{}, fmt.Errorf("clarification decision requires decision, mention, and ordinal")
	}
	decision := ClarificationDecision{
		Kind:    *wire.Decision,
		Mention: strings.TrimSpace(*wire.Mention),
		Ordinal: *wire.Ordinal,
	}
	if err := validateClarificationDecision(decision, candidates); err != nil {
		return ClarificationDecision{}, err
	}
	return decision, nil
}

func validateClarificationDecision(decision ClarificationDecision, candidates []CandidateView) error {
	if decision.Ordinal < 0 {
		return fmt.Errorf("clarification ordinal must not be negative")
	}
	switch decision.Kind {
	case ClarificationSelect:
		if decision.Mention == "" && decision.Ordinal == 0 {
			return fmt.Errorf("select decision requires mention or ordinal")
		}
	case ClarificationConfirm:
		// Session state decides whether a bare confirmation identifies a candidate.
	case ClarificationAll, ClarificationCancel, ClarificationNewCommand:
		if decision.Mention != "" || decision.Ordinal != 0 {
			return fmt.Errorf("%s decision must not select a candidate", decision.Kind)
		}
	default:
		return fmt.Errorf("invalid clarification decision %q", decision.Kind)
	}
	if decision.Ordinal > 0 && !candidateOrdinalExists(candidates, decision.Ordinal) {
		return fmt.Errorf("clarification ordinal %d is not a candidate", decision.Ordinal)
	}
	return nil
}

func candidateOrdinalExists(candidates []CandidateView, ordinal int) bool {
	for _, candidate := range candidates {
		if candidate.Ordinal == ordinal {
			return true
		}
	}
	return false
}
