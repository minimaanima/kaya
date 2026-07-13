package playtest

import (
	"context"
	"fmt"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/session"
	"kaya/internal/turn"
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

func TestCheckStateSortsItemLocationDiagnostics(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	chair := state.Objects[scenario.ObjectCollapsedChair]
	chair.ContainedItems = []game.ItemID{scenario.ItemFlashlight}
	state.Objects[chair.ID] = chair

	const want = "item \"flashlight\" has locations [inventory object collapsed_chair object reception_desk]"
	for attempt := 0; attempt < 100; attempt++ {
		violations := CheckState(state)
		for _, violation := range violations {
			if violation.Code == "item_multiple_locations" && violation.Detail != want {
				t.Fatalf("attempt %d detail = %q, want %q", attempt, violation.Detail, want)
			}
		}
		if !hasViolation(violations, "item_multiple_locations") {
			t.Fatalf("attempt %d violations = %s", attempt, fmt.Sprint(violations))
		}
	}
}

func TestCheckStateAllowsOrderedNegativeEventTimes(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.NowSeconds = -10
	state.ScheduledEvents = []world.ScheduledEvent{{TriggerAtSeconds: -5}}

	if violations := CheckState(state); hasViolation(violations, "event_times_unsorted") {
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

func TestCaptureDeepCopiesConversationAndObservationState(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.KnownExitDirections = map[game.RoomID]map[string]bool{
		scenario.RoomReception: {"east": true},
	}
	state.RecentReferents = []game.ReferentGroup{{
		ObjectIDs: []game.ObjectID{scenario.ObjectReceptionDesk},
		ItemIDs:   []game.ItemID{scenario.ItemFlashlight},
	}}
	state.ObservedObjectFacts = map[game.ObjectID]map[game.FactKind]game.Fact{
		scenario.ObjectReceptionDesk: {
			game.FactRoomDescription: {ID: "desk:description", Text: "before"},
		},
	}
	state.LastMentionedItemID = scenario.ItemFlashlight
	state.LastMentionedItemIDs = []game.ItemID{scenario.ItemFlashlight, scenario.ItemBrassKey}

	snapshot := Capture(state)

	state.KnownExitDirections[scenario.RoomReception]["east"] = false
	state.RecentReferents[0].ObjectIDs[0] = scenario.ObjectReceptionFloor
	state.RecentReferents[0].ItemIDs[0] = scenario.ItemBrassKey
	state.ObservedObjectFacts[scenario.ObjectReceptionDesk][game.FactRoomDescription] = game.Fact{ID: "desk:description", Text: "after"}
	state.LastMentionedItemID = scenario.ItemBrassKey
	state.LastMentionedItemIDs[0] = scenario.ItemBrassKey

	if !snapshot.KnownExitDirections[scenario.RoomReception]["east"] {
		t.Fatalf("known exits mutated: %#v", snapshot.KnownExitDirections)
	}
	if got := snapshot.RecentReferents[0]; got.ObjectIDs[0] != scenario.ObjectReceptionDesk || got.ItemIDs[0] != scenario.ItemFlashlight {
		t.Fatalf("referents mutated: %#v", snapshot.RecentReferents)
	}
	if got := snapshot.ObservedObjectFacts[scenario.ObjectReceptionDesk][game.FactRoomDescription].Text; got != "before" {
		t.Fatalf("observed fact = %q, want before", got)
	}
	if snapshot.LastMentionedItemID != scenario.ItemFlashlight || snapshot.LastMentionedItemIDs[0] != scenario.ItemFlashlight {
		t.Fatalf("last mentioned items mutated: %#v", snapshot)
	}
}

func TestSameWorldIgnoresConversationMemory(t *testing.T) {
	before := Capture(scenario.NewPrototypeWorld())
	after := before
	after.RecentReferents = []game.ReferentGroup{{ItemIDs: []game.ItemID{scenario.ItemFlashlight}}}
	after.LastMentionedItemID = scenario.ItemFlashlight
	after.LastMentionedItemIDs = []game.ItemID{scenario.ItemFlashlight}

	if !SameWorld(before, after) {
		t.Fatalf("SameWorld treats conversation memory as world state: before=%#v after=%#v", before, after)
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

func TestSameWorldDetectsKnownExitMutation(t *testing.T) {
	before := Capture(scenario.NewPrototypeWorld())
	after := cloneSnapshot(before)
	after.KnownExitDirections[scenario.RoomReception]["east"] = false

	if SameWorld(before, after) {
		t.Fatalf("SameWorld accepted changed known exits: before=%#v after=%#v", before.KnownExitDirections, after.KnownExitDirections)
	}
}

func TestSameWorldDetectsObservedFactMutation(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.ObservedObjectFacts[scenario.ObjectReceptionDesk] = map[game.FactKind]game.Fact{
		game.FactAction: {ID: "desk:observed", Kind: game.FactAction, Text: "before"},
	}
	before := Capture(state)
	after := cloneSnapshot(before)
	after.ObservedObjectFacts[scenario.ObjectReceptionDesk][game.FactAction] = game.Fact{ID: "desk:observed", Kind: game.FactAction, Text: "after"}

	if SameWorld(before, after) {
		t.Fatalf("SameWorld accepted changed observed facts: before=%#v after=%#v", before.ObservedObjectFacts, after.ObservedObjectFacts)
	}
}

func TestCheckTransitionRejectsUndiscoveredTakeMutation(t *testing.T) {
	beforeState := scenario.NewPrototypeWorld()
	before := Capture(beforeState)
	afterState := scenario.NewPrototypeWorld()
	afterState.DiscoverItems([]game.ItemID{scenario.ItemFlashlight})
	afterState.AddInventory(scenario.ItemFlashlight)
	desk := afterState.Objects[scenario.ObjectReceptionDesk]
	desk.ContainedItems = nil
	afterState.Objects[desk.ID] = desk
	after := Capture(afterState)
	step := Step{
		Before: before,
		Turn: session.ProcessedTurn{Result: turn.Result{Outcomes: []turn.ActionOutcome{{
			Intent: intent.Intent{Action: intent.ActionTakeItem, Target: "flashlight"},
			Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "item_taken"},
		}}}},
		After: after,
	}

	if violations := CheckTransition(runscenario.PrototypeDefinition(), step); !hasViolation(violations, "undiscovered_take_changed_item_state") {
		t.Fatalf("violations = %#v, want undiscovered_take_changed_item_state", violations)
	}
}

func TestCheckTransitionAllowsSearchThenTakeNewlyDiscoveredItem(t *testing.T) {
	generated := mustGeneratedRun(t, 1)
	removeItemFromStateObjects(generated.State, scenario.ItemFlashlight)
	desk := generated.State.Objects[scenario.ObjectReceptionDesk]
	desk.ContainedItems = append(desk.ContainedItems, scenario.ItemFlashlight)
	generated.State.Objects[desk.ID] = desk
	runner := NewRunner(runscenario.PrototypeDefinition(), generated, fallbackParser{}, fallbackComposer{})

	step, err := runner.Step(context.Background(), "search the reception desk and take the flashlight")
	if err != nil {
		t.Fatal(err)
	}
	if !runner.State().HasItem(scenario.ItemFlashlight) {
		t.Fatalf("compound command did not take the flashlight: %#v", step.Turn.Result.Outcomes)
	}
	if hasViolation(step.Violations, "undiscovered_take_changed_item_state") {
		t.Fatalf("search-then-take was treated as an undiscovered take: %#v", step.Violations)
	}
}

func TestCheckTransitionRejectsFlashlightPerceptionOrderMismatch(t *testing.T) {
	base := Snapshot{
		CurrentRoom: scenario.RoomStorage,
		RoomVisibility: map[game.RoomID]world.Visibility{
			scenario.RoomStorage: world.VisibilityPitchBlack,
		},
		RoomObjects: map[game.RoomID][]game.ObjectID{
			scenario.RoomStorage: {scenario.ObjectStorageCabinet},
		},
	}
	tests := []struct {
		name     string
		outcomes []turn.ActionOutcome
	}{
		{
			name: "dark awareness reports lit objects before activation",
			outcomes: []turn.ActionOutcome{
				roomAwarenessOutcome("Storage Cabinet"),
				{Intent: intent.Intent{Action: intent.ActionTurnOn, Item: "flashlight"}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "flashlight_on"}},
			},
		},
		{
			name: "lit awareness reports darkness after activation",
			outcomes: []turn.ActionOutcome{
				{Intent: intent.Intent{Action: intent.ActionTurnOn, Item: "flashlight"}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "flashlight_on"}},
				roomAwarenessOutcome("none"),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			after := cloneSnapshot(base)
			after.ActiveLight = true
			step := Step{Before: base, Turn: session.ProcessedTurn{Result: turn.Result{Outcomes: test.outcomes}}, After: after}
			if violations := CheckTransition(runscenario.PrototypeDefinition(), step); !hasViolation(violations, "flashlight_perception_order_mismatch") {
				t.Fatalf("violations = %#v, want flashlight_perception_order_mismatch", violations)
			}
		})
	}
}

func TestCheckTransitionTracksIntermediateRoomAcrossCompoundReturn(t *testing.T) {
	generated := mustGeneratedRun(t, 1)
	runner := NewRunner(runscenario.PrototypeDefinition(), generated, fallbackParser{}, fallbackComposer{})

	step, err := runner.Step(context.Background(), "go east then look around then go west")
	if err != nil {
		t.Fatal(err)
	}
	if len(step.Turn.Result.Outcomes) != 3 {
		t.Fatalf("outcomes = %#v, want move, awareness, move", step.Turn.Result.Outcomes)
	}
	if step.After.CurrentRoom != scenario.RoomReception {
		t.Fatalf("final room = %q, want starting room %q", step.After.CurrentRoom, scenario.RoomReception)
	}
	if hasViolation(step.Violations, "flashlight_perception_order_mismatch") {
		t.Fatalf("intermediate storage awareness was attributed to the final room: %#v", step.Violations)
	}
}

func TestCheckTransitionRejectsRediscoveredTakenItem(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.DiscoverItems([]game.ItemID{scenario.ItemFlashlight})
	state.AddInventory(scenario.ItemFlashlight)
	desk := state.Objects[scenario.ObjectReceptionDesk]
	desk.ContainedItems = nil
	state.Objects[desk.ID] = desk
	snapshot := Capture(state)
	step := Step{
		Before: snapshot,
		Turn: session.ProcessedTurn{Result: turn.Result{Outcomes: []turn.ActionOutcome{{
			Intent:         intent.Intent{Action: intent.ActionSearch, Target: "reception desk"},
			TargetObjectID: scenario.ObjectReceptionDesk,
			Result:         game.ActionResult{Status: game.ActionSucceeded, Outcome: "searched_found_items"},
		}}}},
		After: snapshot,
	}

	if violations := CheckTransition(runscenario.PrototypeDefinition(), step); !hasViolation(violations, "taken_item_rediscovered") {
		t.Fatalf("violations = %#v, want taken_item_rediscovered", violations)
	}
}

func TestCheckTransitionRejectsUnintendedDoorUnlock(t *testing.T) {
	const decoyDoor game.DoorID = "decoy_door"
	before := Snapshot{
		DoorStates: map[game.DoorID]world.DoorState{
			scenario.DoorStairwell: world.DoorLocked,
			decoyDoor:              world.DoorLocked,
		},
		DoorNames: map[game.DoorID]string{
			scenario.DoorStairwell: "Emergency Stairwell Door",
			decoyDoor:              "Decoy Door",
		},
		DoorAliases: map[game.DoorID][]string{
			scenario.DoorStairwell: {"stairwell door"},
			decoyDoor:              {"decoy"},
		},
	}
	after := cloneSnapshot(before)
	after.DoorStates[decoyDoor] = world.DoorClosed
	step := Step{
		Before: before,
		Turn: session.ProcessedTurn{Result: turn.Result{Outcomes: []turn.ActionOutcome{{
			Intent: intent.Intent{Action: intent.ActionUseItem, Item: "key", Target: "emergency stairwell door"},
			Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "door_unlocked"},
		}}}},
		After: after,
	}

	violations := CheckTransition(runscenario.PrototypeDefinition(), step)
	for _, code := range []string{"intended_door_not_unlocked", "unintended_door_changed"} {
		if !hasViolation(violations, code) {
			t.Fatalf("violations = %#v, want %s", violations, code)
		}
	}
}

func TestCheckTransitionAllowsUnambiguousInferredDoorUnlock(t *testing.T) {
	before := Capture(scenario.NewPrototypeWorld())
	after := cloneSnapshot(before)
	after.DoorStates[scenario.DoorStairwell] = world.DoorClosed
	step := Step{
		Before: before,
		Turn: session.ProcessedTurn{Result: turn.Result{Outcomes: []turn.ActionOutcome{{
			Intent: intent.Intent{Action: intent.ActionUseItem, Item: "key"},
			Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "door_unlocked"},
		}}}},
		After: after,
	}

	violations := CheckTransition(runscenario.PrototypeDefinition(), step)
	if hasViolation(violations, "intended_door_not_unlocked") || hasViolation(violations, "unintended_door_changed") {
		t.Fatalf("unambiguous inferred unlock was rejected: %#v", violations)
	}
}

func removeItemFromStateObjects(state *world.State, itemID game.ItemID) {
	for objectID, object := range state.Objects {
		filtered := object.ContainedItems[:0]
		for _, containedID := range object.ContainedItems {
			if containedID != itemID {
				filtered = append(filtered, containedID)
			}
		}
		object.ContainedItems = filtered
		state.Objects[objectID] = object
	}
}

func TestCheckTransitionRejectsScheduledEventReordering(t *testing.T) {
	first := game.WorldEvent{Type: game.EventSound, Description: "first"}
	second := game.WorldEvent{Type: game.EventSound, Description: "second"}
	before := Snapshot{
		RemainingEventTimes: []int{5, 10},
		RemainingEvents: []world.ScheduledEvent{
			{TriggerAtSeconds: 5, Event: first},
			{TriggerAtSeconds: 10, Event: second},
		},
	}
	after := Snapshot{Time: 10}
	step := Step{
		Before: before,
		Turn: session.ProcessedTurn{
			DurationSeconds: 10,
			Result: turn.Result{Outcomes: []turn.ActionOutcome{{Result: game.ActionResult{
				Events: []game.WorldEvent{second, first},
			}}}},
		},
		After: after,
	}

	if violations := CheckTransition(runscenario.PrototypeDefinition(), step); !hasViolation(violations, "scheduled_event_emission_mismatch") {
		t.Fatalf("violations = %#v, want scheduled_event_emission_mismatch", violations)
	}
}

func TestCapturePreservesScheduledEventInsertionOrder(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.ScheduledEvents = []world.ScheduledEvent{
		{TriggerAtSeconds: 5, Event: game.WorldEvent{Description: "inserted first"}},
		{TriggerAtSeconds: 5, Event: game.WorldEvent{Description: "inserted second"}},
	}

	got := Capture(state).RemainingEvents
	if len(got) != 2 || got[0].Event.Description != "inserted first" || got[1].Event.Description != "inserted second" {
		t.Fatalf("remaining events = %#v, want insertion order", got)
	}
}

func roomAwarenessOutcome(visibleObjects string) turn.ActionOutcome {
	return turn.ActionOutcome{
		Intent: intent.Intent{Action: intent.ActionInspect},
		Result: game.ActionResult{
			Status:  game.ActionSucceeded,
			Outcome: "inspected_room",
			VisibleFacts: []game.Fact{{
				Kind: game.FactVisibleObjects, Value: visibleObjects, Required: true,
			}},
		},
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
