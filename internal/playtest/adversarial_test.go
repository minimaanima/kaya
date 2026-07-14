package playtest

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

type adversarialState struct {
	time       int
	room       game.RoomID
	inventory  []game.ItemID
	discovered []game.ItemID
	light      bool
	door       world.DoorState
}

func TestAdversarialPrototypeSessions(t *testing.T) {
	adversarialCases := []struct {
		name         string
		placements   []rungen.Placement
		prepare      func(*Runner)
		messages     []string
		wantOutcomes []string
		wantStates   []adversarialState
		check        func(*testing.T, []Step)
	}{
		{
			name:         "take-before-discovery",
			placements:   prototypePlacementsFor(scenario.ObjectReceptionDesk, scenario.ObjectBodyCabinet),
			messages:     []string{"take the flashlight"},
			wantOutcomes: []string{"unresolved_reference"},
			wantStates: []adversarialState{{
				time: 0, room: scenario.RoomReception, door: world.DoorLocked,
			}},
		},
		{
			name:         "locked-door-does-not-move",
			placements:   prototypePlacementsFor(scenario.ObjectReceptionDesk, scenario.ObjectStorageCabinet),
			prepare:      storageWithLight,
			messages:     []string{"go north"},
			wantOutcomes: []string{"door_blocked"},
			wantStates: []adversarialState{{
				time: 2, room: scenario.RoomStorage, inventory: []game.ItemID{scenario.ItemFlashlight}, discovered: []game.ItemID{scenario.ItemFlashlight}, light: true, door: world.DoorLocked,
			}},
		},
		{
			name:         "dark-inspection-hides-objects",
			placements:   prototypePlacementsFor(scenario.ObjectReceptionFloor, scenario.ObjectStorageCabinet),
			messages:     []string{"go east", "look around"},
			wantOutcomes: []string{"moved", "inspected_room"},
			wantStates: []adversarialState{
				{time: 20, room: scenario.RoomStorage, door: world.DoorLocked},
				{time: 25, room: scenario.RoomStorage, door: world.DoorLocked},
			},
			check: func(t *testing.T, steps []Step) {
				t.Helper()
				if responseMentionsHiddenStorageDetail(steps[1].Turn.Response.Text) {
					t.Fatalf("dark inspection leaked hidden storage detail: %q", steps[1].Turn.Response.Text)
				}
			},
		},
		{
			name:         "ambiguous-doctor-remembers-both",
			placements:   prototypePlacementsFor(scenario.ObjectReceptionFloor, scenario.ObjectStorageCabinet),
			prepare:      storageWithLight,
			messages:     []string{"search the doctors", "both"},
			wantOutcomes: []string{"clarification", "searched_empty"},
			wantStates: []adversarialState{
				{time: 0, room: scenario.RoomStorage, inventory: []game.ItemID{scenario.ItemFlashlight}, discovered: []game.ItemID{scenario.ItemFlashlight}, light: true, door: world.DoorLocked},
				{time: 60, room: scenario.RoomStorage, inventory: []game.ItemID{scenario.ItemFlashlight}, discovered: []game.ItemID{scenario.ItemFlashlight}, light: true, door: world.DoorLocked},
			},
			check: func(t *testing.T, steps []Step) {
				t.Helper()
				if steps[0].Turn.Pending == nil || steps[1].Turn.ClarificationDecision == nil || steps[1].Turn.ClarificationDecision.Kind != intent.ClarificationAll {
					t.Fatalf("clarification evidence was not retained: first=%#v second=%#v", steps[0].Turn.Pending, steps[1].Turn.ClarificationDecision)
				}
				if len(steps[1].Turn.Result.Outcomes) != 2 {
					t.Fatalf("remembered follow-up outcomes = %#v, want both doctors searched", steps[1].Turn.Result.Outcomes)
				}
				gotTargets := []game.ObjectID{steps[1].Turn.Result.Outcomes[0].TargetObjectID, steps[1].Turn.Result.Outcomes[1].TargetObjectID}
				wantTargets := []game.ObjectID{scenario.ObjectBodyCabinet, scenario.ObjectBodyDoor}
				if !reflect.DeepEqual(gotTargets, wantTargets) {
					t.Fatalf("remembered follow-up targets = %#v, want %#v", gotTargets, wantTargets)
				}
			},
		},
		{
			name:         "failed-first-action-stops-compound",
			placements:   prototypePlacementsFor(scenario.ObjectCollapsedChair, scenario.ObjectStorageCabinet),
			messages:     []string{"take the key and go east"},
			wantOutcomes: []string{"unresolved_reference"},
			wantStates: []adversarialState{{
				time: 0, room: scenario.RoomReception, door: world.DoorLocked,
			}},
			check: func(t *testing.T, steps []Step) {
				t.Helper()
				if len(steps[0].Turn.SemanticPlan.Actions) != 2 || len(steps[0].Turn.Result.Outcomes) != 1 {
					t.Fatalf("compound failure executed unexpected actions: plan=%#v outcomes=%#v", steps[0].Turn.SemanticPlan.Actions, steps[0].Turn.Result.Outcomes)
				}
			},
		},
		{
			name:         "repeated-search-after-take",
			placements:   prototypePlacementsFor(scenario.ObjectReceptionDesk, scenario.ObjectStorageCabinet),
			messages:     []string{"search the reception desk", "take the flashlight", "search the reception desk"},
			wantOutcomes: []string{"searched_found_items", "item_taken", "searched_empty"},
			wantStates: []adversarialState{
				{time: 35, room: scenario.RoomReception, discovered: []game.ItemID{scenario.ItemFlashlight}, door: world.DoorLocked},
				{time: 40, room: scenario.RoomReception, inventory: []game.ItemID{scenario.ItemFlashlight}, discovered: []game.ItemID{scenario.ItemFlashlight}, door: world.DoorLocked},
				{time: 70, room: scenario.RoomReception, inventory: []game.ItemID{scenario.ItemFlashlight}, discovered: []game.ItemID{scenario.ItemFlashlight}, door: world.DoorLocked},
			},
		},
	}

	for _, tc := range adversarialCases {
		t.Run(tc.name, func(t *testing.T) {
			run := fixedPrototypeRun(t, tc.placements)
			runner := NewRunner(runscenario.PrototypeDefinition(), run, fallbackParser{}, fallbackComposer{})
			if tc.prepare != nil {
				tc.prepare(runner)
			}

			steps := make([]Step, 0, len(tc.messages))
			for index, message := range tc.messages {
				step, err := runner.Step(context.Background(), message)
				if err != nil {
					t.Fatalf("step %d message %q: %v\nsession=%#v", index+1, message, err, runner.Session())
				}
				steps = append(steps, step)
				assertAdversarialState(t, step.After, tc.wantStates[index])
				if got := terminalOutcome(step); got != tc.wantOutcomes[index] {
					t.Fatalf("step %d outcome = %q, want %q\nstep=%#v", index+1, got, tc.wantOutcomes[index], step)
				}
			}
			if tc.check != nil {
				tc.check(t, steps)
			}
		})
	}
}

func fixedPrototypeRun(t *testing.T, placements []rungen.Placement) rungen.GeneratedRun {
	t.Helper()
	state := scenario.NewPrototypeTemplate()
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatalf("apply placements %#v: %v", placements, err)
	}
	return rungen.GeneratedRun{
		Seed:             4,
		GeneratorVersion: rungen.CurrentGeneratorVersion,
		ScenarioID:       scenario.PrototypeScenarioID,
		ScenarioVersion:  scenario.PrototypeScenarioVersion,
		State:            state,
		Placements:       append([]rungen.Placement(nil), placements...),
	}
}

func prototypePlacementsFor(flashlightObject, keyObject game.ObjectID) []rungen.Placement {
	return []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: flashlightObject},
		{ItemID: scenario.ItemBrassKey, ObjectID: keyObject},
	}
}

func storageWithLight(runner *Runner) {
	state := runner.State()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	for objectID, object := range state.Objects {
		for index, itemID := range object.ContainedItems {
			if itemID != scenario.ItemFlashlight {
				continue
			}
			object.ContainedItems = append(object.ContainedItems[:index], object.ContainedItems[index+1:]...)
			state.Objects[objectID] = object
			break
		}
	}
	state.AddInventory(scenario.ItemFlashlight)
	state.ActiveLight = true
	_ = state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception)
}

func assertAdversarialState(t *testing.T, got Snapshot, want adversarialState) {
	t.Helper()
	if got.Time != want.time || got.CurrentRoom != want.room || got.ActiveLight != want.light || got.DoorStates[scenario.DoorStairwell] != want.door ||
		!reflect.DeepEqual(got.Inventory, want.inventory) || !reflect.DeepEqual(got.Discovered, want.discovered) {
		t.Fatalf("state = time=%d room=%q inventory=%#v discovered=%#v light=%t door=%q, want time=%d room=%q inventory=%#v discovered=%#v light=%t door=%q",
			got.Time, got.CurrentRoom, got.Inventory, got.Discovered, got.ActiveLight, got.DoorStates[scenario.DoorStairwell],
			want.time, want.room, want.inventory, want.discovered, want.light, want.door)
	}
}

func terminalOutcome(step Step) string {
	if len(step.Turn.Result.Outcomes) == 0 {
		return step.Turn.Result.StopReason
	}
	return step.Turn.Result.Outcomes[len(step.Turn.Result.Outcomes)-1].Result.Outcome
}

func responseMentionsHiddenStorageDetail(text string) bool {
	for _, hidden := range []string{"doctor near cabinet", "doctor near door", "storage cabinet", "north"} {
		if containsNormalizedNameForTest(text, hidden) {
			return true
		}
	}
	return false
}

func containsNormalizedNameForTest(text, name string) bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	needle := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	for start := 0; start+len(needle) <= len(words); start++ {
		if reflect.DeepEqual(words[start:start+len(needle)], needle) {
			return true
		}
	}
	return false
}
