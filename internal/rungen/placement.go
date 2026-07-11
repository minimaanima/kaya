package rungen

import (
	"fmt"
	"sort"

	"kaya/internal/game"
	"kaya/internal/world"
)

func ApplyPlacements(state *world.State, placements []Placement) error {
	if state == nil {
		return fmt.Errorf("%w: world is nil", ErrInvalidDefinition)
	}

	updates := make(map[game.ObjectID]world.Object)
	seenItems := make(map[game.ItemID]bool, len(placements))
	for _, placement := range placements {
		if placement.ItemID == "" || seenItems[placement.ItemID] {
			return fmt.Errorf("%w: duplicate or empty placement item %q", ErrInvalidDefinition, placement.ItemID)
		}
		seenItems[placement.ItemID] = true

		item, ok := state.Items[placement.ItemID]
		if !ok || !item.Portable {
			return fmt.Errorf("%w: item %q is missing or not portable", ErrInvalidDefinition, placement.ItemID)
		}
		if itemAlreadyPlaced(state, placement.ItemID) {
			return fmt.Errorf("%w: item %q is already placed", ErrInvalidDefinition, placement.ItemID)
		}

		object, ok := state.Objects[placement.ObjectID]
		if !ok || !object.Searchable || objectRoomCount(state, placement.ObjectID) != 1 {
			return fmt.Errorf("%w: candidate object %q is invalid", ErrInvalidDefinition, placement.ObjectID)
		}
		if updated, ok := updates[placement.ObjectID]; ok {
			object = updated
		}
		object.ContainedItems = append(append([]game.ItemID(nil), object.ContainedItems...), placement.ItemID)
		sort.Slice(object.ContainedItems, func(i, j int) bool {
			return object.ContainedItems[i] < object.ContainedItems[j]
		})
		updates[placement.ObjectID] = object
	}

	for objectID, object := range updates {
		state.Objects[objectID] = object
	}
	return nil
}
