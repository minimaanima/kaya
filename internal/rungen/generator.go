package rungen

import (
	"fmt"
	"strings"
)

type GenerationError struct {
	Attempts int
	Reasons  []string
}

func (e GenerationError) Error() string {
	limit := len(e.Reasons)
	if limit > 10 {
		limit = 10
	}
	detail := strings.Join(e.Reasons[:limit], "; ")
	if detail == "" {
		return fmt.Sprintf("%s after %d attempts", ErrNoPlayableRun, e.Attempts)
	}
	return fmt.Sprintf("%s after %d attempts: %s", ErrNoPlayableRun, e.Attempts, detail)
}

func (e GenerationError) Unwrap() error {
	return ErrNoPlayableRun
}

func Generate(config RunConfig, def Definition) (GeneratedRun, error) {
	if config.GeneratorVersion != CurrentGeneratorVersion {
		return GeneratedRun{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, config.GeneratorVersion)
	}
	if err := ValidateDefinition(def); err != nil {
		return GeneratedRun{}, err
	}

	combinations, err := placementCombinations(def.ItemRules)
	if err != nil {
		return GeneratedRun{}, err
	}
	shufflePlacements(combinations, config.Seed)

	reasons := make([]string, 0, len(combinations))
	for attempt, placements := range combinations {
		proofState := def.Build()
		if err := ApplyPlacements(proofState, placements); err != nil {
			return GeneratedRun{}, fmt.Errorf("apply placement attempt %d: %w", attempt+1, err)
		}
		validation, err := Validate(def, proofState)
		if err != nil {
			return GeneratedRun{}, fmt.Errorf("validate placement attempt %d: %w", attempt+1, err)
		}
		if !validation.Valid {
			reasons = append(reasons, fmt.Sprintf("attempt %d: %s", attempt+1, validation.Reason))
			continue
		}
		if err := Replay(def, placements, validation.Witness); err != nil {
			reasons = append(reasons, fmt.Sprintf("attempt %d replay: %v", attempt+1, err))
			continue
		}

		playerState := def.Build()
		if err := ApplyPlacements(playerState, placements); err != nil {
			return GeneratedRun{}, fmt.Errorf("build player state: %w", err)
		}
		return GeneratedRun{
			Seed:             config.Seed,
			GeneratorVersion: config.GeneratorVersion,
			ScenarioID:       def.ScenarioID,
			ScenarioVersion:  def.ScenarioVersion,
			State:            playerState,
			Placements:       append([]Placement(nil), placements...),
			Validation:       validation,
		}, nil
	}

	return GeneratedRun{}, GenerationError{
		Attempts: len(combinations),
		Reasons:  reasons,
	}
}
