package playtest

import (
	"context"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/session"
	"kaya/internal/turn"
	"kaya/internal/world"
)

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
				{Result: game.ActionResult{Events: []game.WorldEvent{{Type: game.EventSound}, {Type: game.EventSound}}}},
			}},
		},
		After: after,
	}
	violations := CheckTransition(runscenario.PrototypeDefinition(), step)
	for _, code := range []string{"time_duration_mismatch", "locked_move_changed_room", "taken_item_not_removed", "scheduled_event_duplicated"} {
		if !hasViolation(violations, code) {
			t.Fatalf("missing %q in %#v", code, violations)
		}
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
