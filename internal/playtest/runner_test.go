package playtest

import (
	"context"
	"errors"
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
	before := Capture(scenario.NewPrototypeWorld())
	before.Time = 0
	before.RemainingEventTimes = []int{5, 5, 10}
	after := before
	after.Time = 5
	after.RemainingEventTimes = []int{10}
	step := Step{
		Before: before,
		Turn: session.ProcessedTurn{
			DurationSeconds: 5,
			Result: turn.Result{Outcomes: []turn.ActionOutcome{{Result: game.ActionResult{Events: []game.WorldEvent{
				{Type: game.EventSound, Description: "knock"},
				{Type: game.EventSound, Description: "knock"},
			}}}}},
		},
		After: after,
	}
	if violations := CheckTransition(runscenario.PrototypeDefinition(), step); len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
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
