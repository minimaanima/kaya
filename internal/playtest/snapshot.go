package playtest

import (
	"reflect"
	"sort"

	"kaya/internal/game"
	"kaya/internal/world"
)

func Capture(state *world.State) Snapshot {
	if state == nil {
		return Snapshot{}
	}

	snapshot := Snapshot{
		CurrentRoom:          state.CurrentRoomID,
		PreviousRoom:         state.PreviousRoomID,
		Time:                 state.NowSeconds,
		Inventory:            sortedPresentItems(state.Inventory),
		Discovered:           sortedPresentItems(state.DiscoveredItems),
		ItemNames:            make(map[game.ItemID]string, len(state.Items)),
		ItemAliases:          make(map[game.ItemID][]string, len(state.Items)),
		ObjectItems:          make(map[game.ObjectID][]game.ItemID, len(state.Objects)),
		ObjectRevealedItems:  make(map[game.ObjectID][]game.ItemID, len(state.Objects)),
		RoomVisibility:       make(map[game.RoomID]world.Visibility, len(state.Rooms)),
		RoomObjects:          make(map[game.RoomID][]game.ObjectID, len(state.Rooms)),
		DoorStates:           make(map[game.DoorID]world.DoorState, len(state.Doors)),
		DoorNames:            make(map[game.DoorID]string, len(state.Doors)),
		DoorAliases:          make(map[game.DoorID][]string, len(state.Doors)),
		KnownExitDirections:  cloneKnownExitDirections(state.KnownExitDirections),
		RecentReferents:      cloneReferentGroups(state.RecentReferents),
		ObservedObjectFacts:  cloneObservedObjectFacts(state.ObservedObjectFacts),
		LastMentionedItemID:  state.LastMentionedItemID,
		LastMentionedItemIDs: append([]game.ItemID(nil), state.LastMentionedItemIDs...),
		RemainingEventTimes:  make([]int, 0, len(state.ScheduledEvents)),
		RemainingEvents:      make([]world.ScheduledEvent, 0, len(state.ScheduledEvents)),
		ActiveLight:          state.ActiveLight,
		Kaya:                 state.Kaya,
	}
	for itemID, item := range state.Items {
		snapshot.ItemNames[itemID] = item.Name
		snapshot.ItemAliases[itemID] = sortedStrings(item.Aliases)
	}
	for objectID, object := range state.Objects {
		snapshot.ObjectItems[objectID] = sortedItemIDs(object.ContainedItems)
		snapshot.ObjectRevealedItems[objectID] = sortedItemIDs(object.RevealedItemIDs)
	}
	for roomID, room := range state.Rooms {
		snapshot.RoomVisibility[roomID] = room.Visibility
		snapshot.RoomObjects[roomID] = sortedObjectIDs(room.Objects)
	}
	for doorID, door := range state.Doors {
		snapshot.DoorStates[doorID] = door.State
		snapshot.DoorNames[doorID] = door.Name
		snapshot.DoorAliases[doorID] = sortedStrings(door.Aliases)
	}
	for _, event := range state.ScheduledEvents {
		snapshot.RemainingEventTimes = append(snapshot.RemainingEventTimes, event.TriggerAtSeconds)
		snapshot.RemainingEvents = append(snapshot.RemainingEvents, event)
	}
	sort.Ints(snapshot.RemainingEventTimes)
	return snapshot
}

func sortedStrings(values []string) []string {
	cloned := append([]string(nil), values...)
	sort.Strings(cloned)
	return cloned
}

func cloneKnownExitDirections(value map[game.RoomID]map[string]bool) map[game.RoomID]map[string]bool {
	if value == nil {
		return nil
	}
	cloned := make(map[game.RoomID]map[string]bool, len(value))
	for roomID, directions := range value {
		inner := make(map[string]bool, len(directions))
		for direction, known := range directions {
			inner[direction] = known
		}
		cloned[roomID] = inner
	}
	return cloned
}

func cloneReferentGroups(value []game.ReferentGroup) []game.ReferentGroup {
	if value == nil {
		return nil
	}
	cloned := make([]game.ReferentGroup, len(value))
	for index, group := range value {
		cloned[index] = game.ReferentGroup{
			ObjectIDs: append([]game.ObjectID(nil), group.ObjectIDs...),
			ItemIDs:   append([]game.ItemID(nil), group.ItemIDs...),
		}
	}
	return cloned
}

func cloneObservedObjectFacts(value map[game.ObjectID]map[game.FactKind]game.Fact) map[game.ObjectID]map[game.FactKind]game.Fact {
	if value == nil {
		return nil
	}
	cloned := make(map[game.ObjectID]map[game.FactKind]game.Fact, len(value))
	for objectID, facts := range value {
		inner := make(map[game.FactKind]game.Fact, len(facts))
		for kind, fact := range facts {
			inner[kind] = fact
		}
		cloned[objectID] = inner
	}
	return cloned
}

func SameWorld(left, right Snapshot) bool {
	return left.CurrentRoom == right.CurrentRoom &&
		left.PreviousRoom == right.PreviousRoom &&
		left.ActiveLight == right.ActiveLight &&
		sameItemIDs(left.Inventory, right.Inventory) &&
		sameItemIDs(left.Discovered, right.Discovered) &&
		sameObjectItems(left.ObjectItems, right.ObjectItems) &&
		sameObjectItems(left.ObjectRevealedItems, right.ObjectRevealedItems) &&
		sameDoorStates(left.DoorStates, right.DoorStates) &&
		reflect.DeepEqual(left.KnownExitDirections, right.KnownExitDirections) &&
		reflect.DeepEqual(left.ObservedObjectFacts, right.ObservedObjectFacts)
}

func sortedPresentItems(items map[game.ItemID]bool) []game.ItemID {
	ids := make([]game.ItemID, 0, len(items))
	for itemID, present := range items {
		if present {
			ids = append(ids, itemID)
		}
	}
	return sortedItemIDs(ids)
}

func sortedItemIDs(items []game.ItemID) []game.ItemID {
	cloned := append([]game.ItemID(nil), items...)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i] < cloned[j] })
	return cloned
}

func sortedObjectIDs(objects []game.ObjectID) []game.ObjectID {
	cloned := append([]game.ObjectID(nil), objects...)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i] < cloned[j] })
	return cloned
}

func sameItemIDs(left, right []game.ItemID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameObjectItems(left, right map[game.ObjectID][]game.ItemID) bool {
	if len(left) != len(right) {
		return false
	}
	for objectID, leftItems := range left {
		rightItems, ok := right[objectID]
		if !ok || !sameItemIDs(leftItems, rightItems) {
			return false
		}
	}
	return true
}

func sameDoorStates(left, right map[game.DoorID]world.DoorState) bool {
	if len(left) != len(right) {
		return false
	}
	for doorID, leftState := range left {
		if rightState, ok := right[doorID]; !ok || leftState != rightState {
			return false
		}
	}
	return true
}
