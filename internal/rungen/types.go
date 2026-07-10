package rungen

import (
	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/world"
)

const (
	CurrentGeneratorVersion  = 1
	MaxPlacementCombinations = 4096
	MaxValidationStates      = 10_000
)

type RunConfig struct {
	Seed             int64
	GeneratorVersion int
}

type Definition struct {
	ScenarioID      string
	ScenarioVersion int
	Build           func() *world.State
	StartRoom       game.RoomID
	WinRoom         game.RoomID
	LightItem       game.ItemID
	ItemRules       []ItemRule
}

type ItemRule struct {
	ItemID     game.ItemID
	Candidates []PlacementCandidate
}

type PlacementCandidate struct {
	ObjectID game.ObjectID
}

type Placement struct {
	ItemID   game.ItemID
	ObjectID game.ObjectID
}

type WitnessStep struct {
	Intent          intent.Intent
	ExpectedOutcome string
}

type ValidationResult struct {
	Valid         bool
	Reason        string
	VisitedStates int
	Witness       []WitnessStep
}

type GeneratedRun struct {
	Seed             int64
	GeneratorVersion int
	ScenarioID       string
	ScenarioVersion  int
	State            *world.State
	Placements       []Placement
	Validation       ValidationResult
}
