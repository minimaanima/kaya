package playtest

import (
	"fmt"
	"sort"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/rungen"
	"kaya/internal/turn"
	"kaya/internal/world"
)

func CheckState(state *world.State) []Violation {
	if state == nil {
		return []Violation{{Code: "state_nil", Detail: "world state is nil"}}
	}

	violations := make([]Violation, 0)
	if _, ok := state.Rooms[state.CurrentRoomID]; !ok {
		violations = append(violations, Violation{Code: "current_room_missing", Detail: fmt.Sprintf("current room %q is missing", state.CurrentRoomID)})
	}
	if state.PreviousRoomID != "" {
		if _, ok := state.Rooms[state.PreviousRoomID]; !ok {
			violations = append(violations, Violation{Code: "previous_room_missing", Detail: fmt.Sprintf("previous room %q is missing", state.PreviousRoomID)})
		}
	}

	locations := make(map[game.ItemID][]string)
	for itemID, present := range state.Inventory {
		if !present {
			continue
		}
		item, ok := state.Items[itemID]
		if !ok {
			violations = append(violations, Violation{Code: "inventory_item_missing", Detail: fmt.Sprintf("inventory item %q is undefined", itemID)})
		} else if !item.Portable {
			violations = append(violations, Violation{Code: "inventory_item_not_portable", Detail: fmt.Sprintf("inventory item %q is not portable", itemID)})
		}
		locations[itemID] = append(locations[itemID], "inventory")
	}
	for objectID, object := range state.Objects {
		for _, itemID := range object.ContainedItems {
			locations[itemID] = append(locations[itemID], "object "+string(objectID))
		}
	}
	for itemID, itemLocations := range locations {
		if len(itemLocations) > 1 {
			violations = append(violations, Violation{Code: "item_multiple_locations", Detail: fmt.Sprintf("item %q has locations %v", itemID, itemLocations)})
		}
	}

	previousTime := -1
	for _, event := range state.ScheduledEvents {
		if event.TriggerAtSeconds <= state.NowSeconds {
			violations = append(violations, Violation{Code: "event_before_current_time", Detail: fmt.Sprintf("event at %d is not after %d", event.TriggerAtSeconds, state.NowSeconds)})
		}
		if previousTime > event.TriggerAtSeconds {
			violations = append(violations, Violation{Code: "event_times_unsorted", Detail: "scheduled event times are not sorted"})
			break
		}
		previousTime = event.TriggerAtSeconds
	}
	return sortViolations(violations)
}

func CheckTransition(def rungen.Definition, step Step) []Violation {
	violations := make([]Violation, 0)
	expectedTime := step.Before.Time + step.Turn.DurationSeconds
	if step.After.Time != expectedTime {
		violations = append(violations, Violation{Code: "time_duration_mismatch", Detail: fmt.Sprintf("time is %d, want %d", step.After.Time, expectedTime)})
	}
	if clarificationOrRefusal(step.Turn.Result) && !SameWorld(step.Before, step.After) {
		violations = append(violations, Violation{Code: "nonexecuted_action_mutated_world", Detail: "clarification or refusal changed world state"})
	}
	if step.ObjectiveEmitted && step.After.CurrentRoom != def.WinRoom {
		violations = append(violations, Violation{Code: "objective_outside_win_room", Detail: fmt.Sprintf("objective emitted in %q", step.After.CurrentRoom)})
	}

	for _, outcome := range step.Turn.Result.Outcomes {
		if outcome.Intent.Action == intent.ActionTakeItem && outcome.Result.Outcome == "item_taken" && !takenItemRemoved(step.Before, step.After) {
			violations = append(violations, Violation{Code: "taken_item_not_removed", Detail: "taken item remains in a container or was not added to inventory"})
		}
		if outcome.Intent.Action == intent.ActionMove && outcome.Result.Outcome == "door_blocked" && step.After.CurrentRoom != step.Before.CurrentRoom {
			violations = append(violations, Violation{Code: "locked_move_changed_room", Detail: "blocked door movement changed current room"})
		}
	}
	if scheduledEventDuplicated(step.Turn.Result.Outcomes) {
		violations = append(violations, Violation{Code: "scheduled_event_duplicated", Detail: "the same scheduled event was emitted more than once"})
	}
	return sortViolations(violations)
}

func clarificationOrRefusal(result turn.Result) bool {
	if len(result.Outcomes) == 0 {
		return result.StopReason == "clarification"
	}
	for _, outcome := range result.Outcomes {
		if outcome.Result.Status != game.ActionRefused && outcome.Result.Status != game.ActionClarification && !outcome.Result.NeedsClarification {
			return false
		}
	}
	return true
}

func takenItemRemoved(before, after Snapshot) bool {
	if len(after.Inventory) <= len(before.Inventory) {
		return false
	}
	for _, itemID := range after.Inventory {
		if containsItem(before.Inventory, itemID) {
			continue
		}
		if !objectContainsItem(after.ObjectItems, itemID) {
			return true
		}
	}
	return false
}

func containsItem(items []game.ItemID, target game.ItemID) bool {
	for _, itemID := range items {
		if itemID == target {
			return true
		}
	}
	return false
}

func objectContainsItem(objects map[game.ObjectID][]game.ItemID, target game.ItemID) bool {
	for _, items := range objects {
		if containsItem(items, target) {
			return true
		}
	}
	return false
}

func scheduledEventDuplicated(outcomes []turn.ActionOutcome) bool {
	seen := make(map[string]bool)
	for _, outcome := range outcomes {
		for _, event := range outcome.Result.Events {
			key := string(event.Type) + "\x00" + event.Description + "\x00" + string(event.Danger)
			if seen[key] {
				return true
			}
			seen[key] = true
		}
	}
	return false
}

func sortViolations(violations []Violation) []Violation {
	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].Code == violations[j].Code {
			return violations[i].Detail < violations[j].Detail
		}
		return violations[i].Code < violations[j].Code
	})
	return violations
}
