package playtest

import (
	"sort"

	"kaya/internal/game"
	"kaya/internal/world"
)

func Capture(state *world.State) Snapshot {
	if state == nil {
		return Snapshot{}
	}

	snapshot := Snapshot{
		CurrentRoom:         state.CurrentRoomID,
		PreviousRoom:        state.PreviousRoomID,
		Time:                state.NowSeconds,
		Inventory:           sortedPresentItems(state.Inventory),
		Discovered:          sortedPresentItems(state.DiscoveredItems),
		ItemNames:           make(map[game.ItemID]string, len(state.Items)),
		ItemAliases:         make(map[game.ItemID][]string, len(state.Items)),
		ObjectItems:         make(map[game.ObjectID][]game.ItemID, len(state.Objects)),
		ObjectRevealedItems: make(map[game.ObjectID][]game.ItemID, len(state.Objects)),
		DoorStates:          make(map[game.DoorID]world.DoorState, len(state.Doors)),
		RemainingEventTimes: make([]int, 0, len(state.ScheduledEvents)),
		RemainingEvents:     make([]world.ScheduledEvent, 0, len(state.ScheduledEvents)),
		ActiveLight:         state.ActiveLight,
		Kaya:                state.Kaya,
	}
	for itemID, item := range state.Items {
		snapshot.ItemNames[itemID] = item.Name
		snapshot.ItemAliases[itemID] = sortedStrings(item.Aliases)
	}
	for objectID, object := range state.Objects {
		snapshot.ObjectItems[objectID] = sortedItemIDs(object.ContainedItems)
		snapshot.ObjectRevealedItems[objectID] = sortedItemIDs(object.RevealedItemIDs)
	}
	for doorID, door := range state.Doors {
		snapshot.DoorStates[doorID] = door.State
	}
	for _, event := range state.ScheduledEvents {
		snapshot.RemainingEventTimes = append(snapshot.RemainingEventTimes, event.TriggerAtSeconds)
		snapshot.RemainingEvents = append(snapshot.RemainingEvents, event)
	}
	sort.Ints(snapshot.RemainingEventTimes)
	sort.Slice(snapshot.RemainingEvents, func(i, j int) bool {
		left, right := snapshot.RemainingEvents[i], snapshot.RemainingEvents[j]
		if left.TriggerAtSeconds != right.TriggerAtSeconds {
			return left.TriggerAtSeconds < right.TriggerAtSeconds
		}
		if left.Event.Type != right.Event.Type {
			return left.Event.Type < right.Event.Type
		}
		if left.Event.Description != right.Event.Description {
			return left.Event.Description < right.Event.Description
		}
		return left.Event.Danger < right.Event.Danger
	})
	return snapshot
}

func sortedStrings(values []string) []string {
	cloned := append([]string(nil), values...)
	sort.Strings(cloned)
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
		sameDoorStates(left.DoorStates, right.DoorStates)
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
