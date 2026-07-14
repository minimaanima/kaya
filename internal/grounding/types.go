package grounding

import (
	"errors"

	"kaya/internal/intent"
)

var (
	ErrMissingWorld      = errors.New("missing world state")
	ErrUnsupportedAction = errors.New("unsupported semantic action")
)

type CandidateKind string

const (
	CandidateObject CandidateKind = "object"
	CandidateItem   CandidateKind = "item"
	CandidateDoor   CandidateKind = "door"
	CandidateExit   CandidateKind = "exit"
)

type Role string

const (
	RoleObject Role = "object"
	RoleItem   Role = "item"
	RoleDoor   Role = "door"
	RoleExit   Role = "exit"
)

type Candidate struct {
	Kind    CandidateKind
	ID      string
	Name    string
	Aliases []string
}

// Binding is an ID selection made against candidates from an earlier
// clarification. IDs are still checked against the current eligible view.
type Binding struct {
	Role         Role
	CandidateIDs []string
}

type GroundedReference struct {
	Role       Role
	Mention    string
	Quantity   intent.TargetMode
	Candidates []Candidate
}

type Clarification struct {
	Role       Role
	Mention    string
	Candidates []Candidate
}

type MissingReference struct {
	Role    Role
	Mention string
}

type Result struct {
	Action        intent.SemanticAction
	References    []GroundedReference
	Clarification *Clarification
	Missing       *MissingReference
	Err           error
}

func (r Result) Ready() bool {
	return r.Err == nil && r.Clarification == nil && r.Missing == nil
}

func (r Result) Reference(role Role) (GroundedReference, bool) {
	for _, reference := range r.References {
		if reference.Role == role {
			return reference, true
		}
	}
	return GroundedReference{}, false
}
