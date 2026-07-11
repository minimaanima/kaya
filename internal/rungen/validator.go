package rungen

import (
	"fmt"
	"sort"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/world"
)

type symbolicState struct {
	Room       game.RoomID
	Discovered uint64
	Inventory  uint64
	Unlocked   uint64
	LightOn    bool
}

type predecessor struct {
	Previous symbolicState
	Step     WitnessStep
}

type transition struct {
	Next symbolicState
	Step WitnessStep
	Key  string
}

type proofModel struct {
	definition    Definition
	state         *world.State
	itemBits      map[game.ItemID]uint
	doorBits      map[game.DoorID]uint
	itemObjects   map[game.ItemID]game.ObjectID
	objectRooms   map[game.ObjectID]game.RoomID
	requiredItems map[game.ItemID]bool
}

func Validate(def Definition, state *world.State) (ValidationResult, error) {
	if err := ValidateDefinition(def); err != nil {
		return ValidationResult{}, err
	}
	model, err := newProofModel(def, state)
	if err != nil {
		return ValidationResult{}, err
	}

	start := symbolicState{Room: def.StartRoom}
	queue := []symbolicState{start}
	visited := map[symbolicState]bool{start: true}
	parents := make(map[symbolicState]predecessor)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.Room == def.WinRoom {
			return ValidationResult{
				Valid:         true,
				VisitedStates: len(visited),
				Witness:       buildWitness(start, current, parents),
			}, nil
		}

		for _, move := range model.transitions(current) {
			if visited[move.Next] {
				continue
			}
			if len(visited) >= MaxValidationStates {
				return ValidationResult{}, fmt.Errorf("%w: exceeded %d states", ErrValidationLimit, MaxValidationStates)
			}
			visited[move.Next] = true
			parents[move.Next] = predecessor{Previous: current, Step: move.Step}
			queue = append(queue, move.Next)
		}
	}

	return ValidationResult{
		Valid:         false,
		Reason:        "win room unreachable",
		VisitedStates: len(visited),
	}, nil
}

func newProofModel(def Definition, state *world.State) (proofModel, error) {
	if state == nil {
		return proofModel{}, fmt.Errorf("%w: validation world is nil", ErrInvalidDefinition)
	}
	if _, ok := state.Rooms[def.StartRoom]; !ok {
		return proofModel{}, fmt.Errorf("%w: validation start room %q is missing", ErrInvalidDefinition, def.StartRoom)
	}
	if _, ok := state.Rooms[def.WinRoom]; !ok {
		return proofModel{}, fmt.Errorf("%w: validation win room %q is missing", ErrInvalidDefinition, def.WinRoom)
	}

	model := proofModel{
		definition:    def,
		state:         state,
		itemBits:      make(map[game.ItemID]uint, len(def.ItemRules)),
		doorBits:      make(map[game.DoorID]uint),
		itemObjects:   make(map[game.ItemID]game.ObjectID, len(def.ItemRules)),
		objectRooms:   make(map[game.ObjectID]game.RoomID),
		requiredItems: make(map[game.ItemID]bool, len(def.ItemRules)),
	}

	itemIDs := make([]game.ItemID, 0, len(def.ItemRules))
	for _, rule := range def.ItemRules {
		itemIDs = append(itemIDs, rule.ItemID)
		model.requiredItems[rule.ItemID] = true
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i] < itemIDs[j] })
	for index, itemID := range itemIDs {
		model.itemBits[itemID] = uint(index)
	}

	for roomID, room := range state.Rooms {
		for _, objectID := range room.Objects {
			if existingRoom, exists := model.objectRooms[objectID]; exists && existingRoom != roomID {
				return proofModel{}, fmt.Errorf("%w: object %q belongs to multiple rooms", ErrInvalidDefinition, objectID)
			}
			model.objectRooms[objectID] = roomID
		}
	}
	for objectID, object := range state.Objects {
		if _, ok := model.objectRooms[objectID]; !ok {
			continue
		}
		for _, itemID := range object.ContainedItems {
			if !model.requiredItems[itemID] {
				continue
			}
			if previous, exists := model.itemObjects[itemID]; exists {
				return proofModel{}, fmt.Errorf("%w: required item %q is placed in %q and %q", ErrInvalidDefinition, itemID, previous, objectID)
			}
			model.itemObjects[itemID] = objectID
		}
	}
	for _, itemID := range itemIDs {
		if _, ok := model.itemObjects[itemID]; !ok {
			return proofModel{}, fmt.Errorf("%w: required item %q is not placed", ErrInvalidDefinition, itemID)
		}
	}

	doorIDs := make([]game.DoorID, 0)
	seenDoors := make(map[game.DoorID]bool)
	for _, room := range state.Rooms {
		for _, exit := range room.Exits {
			if _, ok := state.Rooms[exit.To]; !ok {
				return proofModel{}, fmt.Errorf("%w: exit target room %q is missing", ErrInvalidDefinition, exit.To)
			}
			if exit.Door == "" || seenDoors[exit.Door] {
				continue
			}
			if _, ok := state.Doors[exit.Door]; !ok {
				return proofModel{}, fmt.Errorf("%w: exit door %q is missing", ErrInvalidDefinition, exit.Door)
			}
			seenDoors[exit.Door] = true
			doorIDs = append(doorIDs, exit.Door)
		}
	}
	if len(doorIDs) > 64 {
		return proofModel{}, fmt.Errorf("%w: relevant door count exceeds 64", ErrInvalidDefinition)
	}
	sort.Slice(doorIDs, func(i, j int) bool { return doorIDs[i] < doorIDs[j] })
	for index, doorID := range doorIDs {
		model.doorBits[doorID] = uint(index)
	}

	return model, nil
}

func (m proofModel) transitions(current symbolicState) []transition {
	room, ok := m.state.Rooms[current.Room]
	if !ok {
		return nil
	}

	var moves []transition
	for _, objectID := range room.Objects {
		object, ok := m.state.Objects[objectID]
		if !ok || !object.Searchable || !world.CanSeeObject(room, object, current.LightOn) {
			continue
		}

		discovered := current.Discovered
		for _, itemID := range object.ContainedItems {
			bit, required := m.itemBits[itemID]
			if required {
				discovered |= uint64(1) << bit
			}
		}
		if discovered != current.Discovered {
			next := current
			next.Discovered = discovered
			moves = append(moves, newTransition(next, intent.Intent{
				Action: intent.ActionSearch,
				Target: object.Name,
			}, "searched_found_items"))
		}
	}

	for itemID, bit := range m.itemBits {
		mask := uint64(1) << bit
		if current.Discovered&mask == 0 || current.Inventory&mask != 0 {
			continue
		}
		objectID := m.itemObjects[itemID]
		if m.objectRooms[objectID] != current.Room {
			continue
		}
		object := m.state.Objects[objectID]
		item := m.state.Items[itemID]
		if !item.Portable || !world.CanSeeObject(room, object, current.LightOn) {
			continue
		}
		next := current
		next.Inventory |= mask
		moves = append(moves, newTransition(next, intent.Intent{
			Action: intent.ActionTakeItem,
			Item:   item.Name,
		}, "item_taken"))
	}

	if !current.LightOn && m.definition.LightItem != "" {
		if bit, ok := m.itemBits[m.definition.LightItem]; ok && current.Inventory&(uint64(1)<<bit) != 0 {
			next := current
			next.LightOn = true
			moves = append(moves, newTransition(next, intent.Intent{
				Action: intent.ActionTurnOn,
				Item:   m.state.Items[m.definition.LightItem].Name,
			}, "flashlight_on"))
		}
	}

	seenUnlocks := make(map[game.DoorID]bool)
	for _, exit := range room.Exits {
		if exit.Door == "" || seenUnlocks[exit.Door] {
			continue
		}
		door := m.state.Doors[exit.Door]
		doorBit, tracked := m.doorBits[door.ID]
		if !tracked || door.IsPassable() || current.Unlocked&(uint64(1)<<doorBit) != 0 {
			continue
		}
		itemBit, required := m.itemBits[door.RequiredKey]
		if !required || current.Inventory&(uint64(1)<<itemBit) == 0 || !door.CanUnlockWith(door.RequiredKey) {
			continue
		}
		seenUnlocks[door.ID] = true
		next := current
		next.Unlocked |= uint64(1) << doorBit
		moves = append(moves, newTransition(next, intent.Intent{
			Action: intent.ActionUseItem,
			Item:   m.state.Items[door.RequiredKey].Name,
			Target: door.Name,
		}, "door_unlocked"))
	}

	for _, exit := range room.Exits {
		if !m.exitPassable(current, exit) {
			continue
		}
		next := current
		next.Room = exit.To
		moves = append(moves, newTransition(next, intent.Intent{
			Action:    intent.ActionMove,
			Direction: exit.Direction,
		}, "moved"))
	}

	sort.Slice(moves, func(i, j int) bool { return moves[i].Key < moves[j].Key })
	return moves
}

func (m proofModel) exitPassable(current symbolicState, exit world.Exit) bool {
	if exit.Door == "" {
		return true
	}
	door, ok := m.state.Doors[exit.Door]
	if !ok {
		return false
	}
	if door.IsPassable() {
		return true
	}
	bit, tracked := m.doorBits[door.ID]
	return tracked && current.Unlocked&(uint64(1)<<bit) != 0
}

func newTransition(next symbolicState, in intent.Intent, outcome string) transition {
	return transition{
		Next: next,
		Step: WitnessStep{Intent: in, ExpectedOutcome: outcome},
		Key:  fmt.Sprintf("%s|%s|%s|%s", in.Action, in.Direction, in.Item, in.Target),
	}
}

func buildWitness(start, current symbolicState, parents map[symbolicState]predecessor) []WitnessStep {
	var reversed []WitnessStep
	for current != start {
		parent := parents[current]
		reversed = append(reversed, parent.Step)
		current = parent.Previous
	}
	witness := make([]WitnessStep, len(reversed))
	for i := range reversed {
		witness[len(reversed)-1-i] = reversed[i]
	}
	return witness
}
