package rungen

import (
	"fmt"
	"strings"

	"kaya/internal/game"
	"kaya/internal/world"
)

func ValidateDefinition(def Definition) error {
	if strings.TrimSpace(def.ScenarioID) == "" {
		return fmt.Errorf("%w: scenario ID is required", ErrInvalidDefinition)
	}
	if def.ScenarioVersion <= 0 {
		return fmt.Errorf("%w: scenario version must be positive", ErrInvalidDefinition)
	}
	if def.Build == nil {
		return fmt.Errorf("%w: world builder is required", ErrInvalidDefinition)
	}

	state := def.Build()
	if state == nil {
		return fmt.Errorf("%w: world builder returned nil", ErrInvalidDefinition)
	}
	if _, ok := state.Rooms[def.StartRoom]; !ok {
		return fmt.Errorf("%w: start room %q is missing", ErrInvalidDefinition, def.StartRoom)
	}
	if _, ok := state.Rooms[def.WinRoom]; !ok {
		return fmt.Errorf("%w: win room %q is missing", ErrInvalidDefinition, def.WinRoom)
	}
	if len(def.ItemRules) == 0 || len(def.ItemRules) > 64 {
		return fmt.Errorf("%w: required item count must be between 1 and 64", ErrInvalidDefinition)
	}

	seenItems := make(map[game.ItemID]bool, len(def.ItemRules))
	for _, rule := range def.ItemRules {
		item, ok := state.Items[rule.ItemID]
		if rule.ItemID == "" || !ok || !item.Portable || seenItems[rule.ItemID] {
			return fmt.Errorf("%w: invalid required item %q", ErrInvalidDefinition, rule.ItemID)
		}
		if itemAlreadyPlaced(state, rule.ItemID) {
			return fmt.Errorf("%w: template already contains required item %q", ErrInvalidDefinition, rule.ItemID)
		}
		seenItems[rule.ItemID] = true
		if len(rule.Candidates) == 0 {
			return fmt.Errorf("%w: item %q has no candidates", ErrInvalidDefinition, rule.ItemID)
		}

		seenCandidates := make(map[game.ObjectID]bool, len(rule.Candidates))
		for _, candidate := range rule.Candidates {
			object, ok := state.Objects[candidate.ObjectID]
			if candidate.ObjectID == "" || !ok || !object.Searchable || seenCandidates[candidate.ObjectID] {
				return fmt.Errorf("%w: invalid candidate %q for item %q", ErrInvalidDefinition, candidate.ObjectID, rule.ItemID)
			}
			if objectRoomCount(state, candidate.ObjectID) != 1 {
				return fmt.Errorf("%w: candidate %q must belong to exactly one room", ErrInvalidDefinition, candidate.ObjectID)
			}
			seenCandidates[candidate.ObjectID] = true
		}
	}

	if def.LightItem != "" && !seenItems[def.LightItem] {
		return fmt.Errorf("%w: light item %q is not a required item", ErrInvalidDefinition, def.LightItem)
	}

	relevantDoors := 0
	for _, door := range state.Doors {
		if door.RequiredKey != "" && seenItems[door.RequiredKey] {
			relevantDoors++
		}
	}
	if relevantDoors > 64 {
		return fmt.Errorf("%w: relevant door count exceeds 64", ErrInvalidDefinition)
	}

	_, err := placementCombinations(def.ItemRules)
	return err
}

func itemAlreadyPlaced(state *world.State, itemID game.ItemID) bool {
	for _, object := range state.Objects {
		for _, containedItemID := range object.ContainedItems {
			if containedItemID == itemID {
				return true
			}
		}
	}
	return false
}

func objectRoomCount(state *world.State, objectID game.ObjectID) int {
	count := 0
	for _, room := range state.Rooms {
		for _, roomObjectID := range room.Objects {
			if roomObjectID == objectID {
				count++
			}
		}
	}
	return count
}
