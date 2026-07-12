package playtest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/session"
	"kaya/internal/turn"
	"kaya/internal/world"
)

func TestRunnerStepRecordsProcessTurnFailure(t *testing.T) {
	want := errors.New("parser unavailable")
	generated := mustGeneratedRun(t, 1)
	runner := NewRunner(runscenario.PrototypeDefinition(), generated, errorParser{err: want}, fallbackComposer{})
	runner.State().ScheduledEvents = []world.ScheduledEvent{{TriggerAtSeconds: 0}}

	step, err := runner.Step(context.Background(), "go east")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
	if step.Error != want.Error() {
		t.Fatalf("step error = %q, want %q", step.Error, want.Error())
	}
	if !SameWorld(step.Before, step.After) || step.Before.Time != step.After.Time {
		t.Fatalf("process failure changed snapshot: before=%#v after=%#v", step.Before, step.After)
	}
	if !hasViolation(step.Violations, "event_before_current_time") {
		t.Fatalf("violations = %#v", step.Violations)
	}
	for _, violation := range step.Violations {
		if !strings.Contains(err.Error(), violation.Code) {
			t.Fatalf("error %q does not include violation %q", err, violation.Code)
		}
	}
	session := runner.Session()
	if len(session.Steps) != 1 || session.Steps[0].Error != want.Error() {
		t.Fatalf("session steps = %#v", session.Steps)
	}
}

func TestCaptureAndCheckTransitionAcceptValidMove(t *testing.T) {
	generated := mustGeneratedRun(t, 1)
	runner := NewRunner(runscenario.PrototypeDefinition(), generated, fallbackParser{}, fallbackComposer{})
	step, err := runner.Step(context.Background(), "go east")
	if err != nil {
		t.Fatal(err)
	}
	if violations := CheckTransition(runscenario.PrototypeDefinition(), step); len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
	}
}

func TestCheckTransitionRejectsInvalidOutcomeChanges(t *testing.T) {
	before := Capture(scenario.NewPrototypeWorld())
	after := before
	after.Time += 2
	after.CurrentRoom = scenario.RoomStairwell
	step := Step{
		Before: before,
		Turn: session.ProcessedTurn{
			DurationSeconds: 1,
			Result: turn.Result{Outcomes: []turn.ActionOutcome{
				{Intent: intent.Intent{Action: intent.ActionMove}, Result: game.ActionResult{Outcome: "door_blocked"}},
				{Intent: intent.Intent{Action: intent.ActionTakeItem, Target: "flashlight"}, Result: game.ActionResult{Outcome: "item_taken"}},
			}},
		},
		After: after,
	}
	violations := CheckTransition(runscenario.PrototypeDefinition(), step)
	for _, code := range []string{"time_duration_mismatch", "locked_move_changed_room", "taken_item_not_removed"} {
		if !hasViolation(violations, code) {
			t.Fatalf("missing %q in %#v", code, violations)
		}
	}
}

func TestCheckTransitionAllowsEqualPayloadEventsWhenDueTimesAreConsumed(t *testing.T) {
	knock := game.WorldEvent{Type: game.EventSound, Description: "knock"}
	later := game.WorldEvent{Type: game.EventSound, Description: "later"}
	before := Capture(scenario.NewPrototypeWorld())
	before.Time = 0
	before.RemainingEventTimes = []int{5, 5, 10}
	before.RemainingEvents = []world.ScheduledEvent{
		{TriggerAtSeconds: 5, Event: knock},
		{TriggerAtSeconds: 5, Event: knock},
		{TriggerAtSeconds: 10, Event: later},
	}
	after := before
	after.Time = 5
	after.RemainingEventTimes = []int{10}
	after.RemainingEvents = []world.ScheduledEvent{{TriggerAtSeconds: 10, Event: later}}
	step := Step{
		Before: before,
		Turn: session.ProcessedTurn{
			DurationSeconds: 5,
			Result: turn.Result{Outcomes: []turn.ActionOutcome{{Result: game.ActionResult{Events: []game.WorldEvent{
				knock,
				knock,
			}}}}},
		},
		After: after,
	}
	if violations := CheckTransition(runscenario.PrototypeDefinition(), step); len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
	}
}

func TestCheckTransitionRejectsConsumedDueEventWithoutEmission(t *testing.T) {
	scheduled := game.WorldEvent{Type: game.EventSound, Description: "knock"}
	before := Capture(scenario.NewPrototypeWorld())
	before.Time = 0
	before.RemainingEventTimes = []int{5}
	before.RemainingEvents = []world.ScheduledEvent{{TriggerAtSeconds: 5, Event: scheduled}}
	after := before
	after.Time = 5
	after.RemainingEventTimes = nil
	after.RemainingEvents = nil
	step := Step{
		Before: before,
		Turn:   session.ProcessedTurn{DurationSeconds: 5},
		After:  after,
	}
	if !hasViolation(CheckTransition(runscenario.PrototypeDefinition(), step), "scheduled_event_emission_mismatch") {
		t.Fatalf("violations = %#v", CheckTransition(runscenario.PrototypeDefinition(), step))
	}
}

func TestCheckTransitionRejectsDueScheduledTimeThatRemains(t *testing.T) {
	before := Capture(scenario.NewPrototypeWorld())
	before.Time = 0
	before.RemainingEventTimes = []int{5}
	after := before
	after.Time = 5
	step := Step{
		Before: before,
		Turn:   session.ProcessedTurn{DurationSeconds: 5},
		After:  after,
	}
	if !hasViolation(CheckTransition(runscenario.PrototypeDefinition(), step), "scheduled_event_not_consumed") {
		t.Fatalf("violations = %#v", CheckTransition(runscenario.PrototypeDefinition(), step))
	}
}

func TestCheckTransitionRequiresTargetedItemToMove(t *testing.T) {
	beforeState := scenario.NewPrototypeWorld()
	before := Capture(beforeState)
	afterState := scenario.NewPrototypeWorld()
	afterState.AddInventory(scenario.ItemBrassKey)
	keyContainer := afterState.Objects[scenario.ObjectBodyCabinet]
	keyContainer.ContainedItems = nil
	afterState.Objects[keyContainer.ID] = keyContainer
	after := Capture(afterState)

	step := Step{
		Before: before,
		Turn: session.ProcessedTurn{Result: turn.Result{Outcomes: []turn.ActionOutcome{{
			Intent: intent.Intent{Action: intent.ActionTakeItem, Target: "flashlight"},
			Result: game.ActionResult{Outcome: "item_taken"},
		}}}},
		After: after,
	}
	if !hasViolation(CheckTransition(runscenario.PrototypeDefinition(), step), "taken_item_not_removed") {
		t.Fatalf("violations = %#v", CheckTransition(runscenario.PrototypeDefinition(), step))
	}
}

func TestCheckTransitionAcceptsTakeItemAliases(t *testing.T) {
	tests := []struct {
		name     string
		itemID   game.ItemID
		objectID game.ObjectID
		target   string
	}{
		{name: "flashlight torch", itemID: scenario.ItemFlashlight, objectID: scenario.ObjectReceptionDesk, target: "torch"},
		{name: "brass key small key", itemID: scenario.ItemBrassKey, objectID: scenario.ObjectBodyCabinet, target: "small key"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beforeState := scenario.NewPrototypeWorld()
			before := Capture(beforeState)
			afterState := scenario.NewPrototypeWorld()
			afterState.AddInventory(test.itemID)
			container := afterState.Objects[test.objectID]
			container.ContainedItems = nil
			afterState.Objects[container.ID] = container
			after := Capture(afterState)

			step := Step{
				Before: before,
				Turn: session.ProcessedTurn{Result: turn.Result{Outcomes: []turn.ActionOutcome{{
					Intent: intent.Intent{Action: intent.ActionTakeItem, Target: test.target},
					Result: game.ActionResult{Outcome: "item_taken"},
				}}}},
				After: after,
			}
			if violations := CheckTransition(runscenario.PrototypeDefinition(), step); len(violations) != 0 {
				t.Fatalf("violations = %#v", violations)
			}
		})
	}
}

func TestRunnerEmitsObjectiveOnlyOnceOnWinRoomTransition(t *testing.T) {
	generated := mustGeneratedRun(t, 3)
	runner := NewRunner(runscenario.PrototypeDefinition(), generated, fallbackParser{}, fallbackComposer{})
	state := runner.State()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil {
		t.Fatal(err)
	}
	state.Doors[scenario.DoorStairwell] = world.Door{
		ID: scenario.DoorStairwell, State: world.DoorOpen,
	}

	first, err := runner.Step(context.Background(), "go north")
	if err != nil {
		t.Fatal(err)
	}
	if !first.ObjectiveEmitted {
		t.Fatalf("first win transition did not emit objective: %#v", first)
	}
	second, err := runner.Step(context.Background(), "wait")
	if err != nil {
		t.Fatal(err)
	}
	if second.ObjectiveEmitted || runner.Session().ObjectiveEmissions != 1 {
		t.Fatalf("objective emissions = %#v, session = %#v", second, runner.Session())
	}
}

func TestClarificationCannotAdvanceTimeOrMutateWorld(t *testing.T) {
	generated := mustGeneratedRun(t, 2)
	runner := NewRunner(runscenario.PrototypeDefinition(), generated, fallbackParser{}, fallbackComposer{})
	step, err := runner.Step(context.Background(), "do it")
	if err != nil {
		t.Fatal(err)
	}
	if violations := CheckTransition(runscenario.PrototypeDefinition(), step); len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
	}
	if step.After.Time != step.Before.Time {
		t.Fatalf("time changed: %#v", step)
	}
}
