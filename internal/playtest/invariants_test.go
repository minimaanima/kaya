package playtest

import (
	"testing"

	"kaya/internal/game"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

func TestCheckStateRejectsDuplicatedItem(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	violations := CheckState(state)
	if !hasViolation(violations, "item_multiple_locations") {
		t.Fatalf("violations = %#v", violations)
	}
}

func TestCaptureSortsAndDeepCopiesWorldState(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.Inventory = map[game.ItemID]bool{
		scenario.ItemFlashlight: true,
		scenario.ItemBrassKey:   true,
	}
	state.DiscoveredItems = map[game.ItemID]bool{
		scenario.ItemFlashlight: true,
		scenario.ItemBrassKey:   true,
	}
	state.ScheduledEvents = []world.ScheduledEvent{
		{TriggerAtSeconds: 20},
		{TriggerAtSeconds: 10},
	}

	snapshot := Capture(state)
	if got := snapshot.Inventory; len(got) != 2 || got[0] != scenario.ItemBrassKey || got[1] != scenario.ItemFlashlight {
		t.Fatalf("inventory = %#v", got)
	}
	if got := snapshot.RemainingEventTimes; len(got) != 2 || got[0] != 10 || got[1] != 20 {
		t.Fatalf("event times = %#v", got)
	}

	state.Inventory = map[game.ItemID]bool{}
	object := state.Objects[scenario.ObjectReceptionDesk]
	object.ContainedItems = nil
	state.Objects[object.ID] = object
	if len(snapshot.Inventory) != 2 || len(snapshot.ObjectItems[scenario.ObjectReceptionDesk]) != 1 {
		t.Fatalf("snapshot changed after state mutation: %#v", snapshot)
	}
}

func TestSameWorldDetectsContainedItemReveal(t *testing.T) {
	before := Capture(scenario.NewPrototypeWorld())
	after := before
	after.ObjectRevealedItems = map[game.ObjectID][]game.ItemID{
		scenario.ObjectReceptionDesk: {scenario.ItemFlashlight},
	}
	if SameWorld(before, after) {
		t.Fatalf("SameWorld accepted a newly revealed contained item: before=%#v after=%#v", before, after)
	}
}

func TestCheckStateRejectsInvalidWorldRelationships(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	delete(state.Rooms, scenario.RoomReception)
	state.Items["fixed"] = world.Item{ID: "fixed", Portable: false}
	state.AddInventory("fixed")
	state.NowSeconds = 15
	state.ScheduledEvents = []world.ScheduledEvent{
		{TriggerAtSeconds: 20},
		{TriggerAtSeconds: 10},
	}

	violations := CheckState(state)
	for _, code := range []string{"current_room_missing", "inventory_item_not_portable", "event_before_current_time", "event_times_unsorted"} {
		if !hasViolation(violations, code) {
			t.Fatalf("missing %q in %#v", code, violations)
		}
	}
}
