package rungen

import (
	"fmt"

	"kaya/internal/actions"
)

func Replay(def Definition, placements []Placement, witness []WitnessStep) error {
	if err := ValidateDefinition(def); err != nil {
		return err
	}
	state := def.Build()
	if err := ApplyPlacements(state, placements); err != nil {
		return fmt.Errorf("apply replay placements: %w", err)
	}

	resolver := actions.NewResolver(state)
	for index, step := range witness {
		result := resolver.Resolve(step.Intent)
		if result.NeedsClarification || result.Outcome != step.ExpectedOutcome {
			return fmt.Errorf(
				"replay step %d action %s: outcome %q, want %q",
				index+1,
				step.Intent.Action,
				result.Outcome,
				step.ExpectedOutcome,
			)
		}
	}
	if state.CurrentRoomID != def.WinRoom {
		return fmt.Errorf("replay ended in %q, want %q", state.CurrentRoomID, def.WinRoom)
	}
	return nil
}
