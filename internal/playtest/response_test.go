package playtest

import (
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/response"
	"kaya/internal/scenario"
	"kaya/internal/session"
	"kaya/internal/turn"
	"kaya/internal/world"
)

func TestCheckResponseRejectsKayaPrefix(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	violations := CheckResponse(responseStep(state, normalResult(), response.Response{Text: "Kaya: I search the desk."}), state)
	assertResponseViolation(t, violations, "response_kaya_prefix")
}

func TestCheckResponseRejectsUngroundedNonFallbackFactID(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	violations := CheckResponse(responseStep(state, normalResult(), response.Response{Text: "I search the desk.", UsedFactIDs: []game.FactID{"f999"}}), state)
	assertResponseViolation(t, violations, "response_fact_id_ungrounded")
}

func TestCheckResponseRejectsClarificationThatAdvancedTime(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	step := responseStep(state, turn.Result{StopReason: "clarification", ClarificationQuestion: "Which way?"}, response.Response{Text: "Which way?"})
	step.After.Time = step.Before.Time + 1
	violations := CheckResponse(step, state)
	assertResponseViolation(t, violations, "response_clarification_advanced_time")
}

func TestCheckResponseRejectsPitchBlackRoomAwarenessLeak(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	result := turn.Result{Outcomes: []turn.ActionOutcome{{
		Intent: intent.Intent{Action: intent.ActionInspect},
		Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "inspected_room"},
	}}}
	violations := CheckResponse(responseStep(state, result, response.Response{Text: "I can see the Storage Cabinet to the north."}), state)
	assertResponseViolation(t, violations, "response_darkness_leak")
}

func TestCheckResponseRejectsCompoundMoveThenInspectPitchBlackLeaks(t *testing.T) {
	for _, text := range []string{
		"I can see the Storage Cabinet.",
		"I can go north.",
	} {
		t.Run(text, func(t *testing.T) {
			state, step := compoundResponseStep(t, []turn.ActionOutcome{
				{Intent: intent.Intent{Action: intent.ActionMove, Direction: "east"}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "moved"}},
				{Intent: intent.Intent{Action: intent.ActionInspect}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "inspected_room"}},
			}, response.Response{Text: text})
			assertResponseViolation(t, CheckResponse(step, state), "response_darkness_leak")
		})
	}
}

func TestCheckResponseDoesNotApplyPostMoveDarknessToEarlierReceptionAwareness(t *testing.T) {
	state, step := compoundResponseStep(t, []turn.ActionOutcome{
		{Intent: intent.Intent{Action: intent.ActionInspect}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "inspected_room"}},
		{Intent: intent.Intent{Action: intent.ActionMove, Direction: "east"}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "moved"}},
	}, response.Response{Text: "I can see the Reception Desk, then move east into Storage Room."})
	if violations := CheckResponse(step, state); hasViolation(violations, "response_darkness_leak") {
		t.Fatalf("earlier reception awareness inherited post-move darkness: %#v", violations)
	}
}

func TestCheckResponseDoesNotTreatOrdinaryProseAsHiddenDirection(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	result := turn.Result{Outcomes: []turn.ActionOutcome{{
		Intent: intent.Intent{Action: intent.ActionInspect},
		Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "inspected_room"},
	}}}
	violations := CheckResponse(responseStep(state, result, response.Response{Text: "The northbound sign is useless in this darkness."}), state)
	if hasViolation(violations, "response_darkness_leak") {
		t.Fatalf("ordinary prose was flagged as a hidden direction: %#v", violations)
	}
}

func TestCheckResponseAllowsKnownPitchBlackReturnDirection(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil {
		t.Fatal(err)
	}
	result := turn.Result{Outcomes: []turn.ActionOutcome{{
		Intent: intent.Intent{Action: intent.ActionInspect},
		Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "inspected_room"},
	}}}
	violations := CheckResponse(responseStep(state, result, response.Response{Text: "I can go: west."}), state)
	if hasViolation(violations, "response_darkness_leak") {
		t.Fatalf("known return route was flagged as hidden: %#v", violations)
	}
}

func TestCheckResponseRejectsDebugMarker(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	violations := CheckResponse(responseStep(state, normalResult(), response.Response{Text: "debug: I search the desk."}), state)
	assertResponseViolation(t, violations, "response_debug_marker")
}

func TestCheckResponseAllowsFallbackWithUngroundedFactIDWhenOtherInvariantsHold(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	violations := CheckResponse(responseStep(state, normalResult(), response.Response{
		Text:         "I search the desk.",
		UsedFallback: true,
		UsedFactIDs:  []game.FactID{"f999"},
	}), state)
	if len(violations) != 0 {
		t.Fatalf("fallback response violations = %#v, want none", violations)
	}
}

func responseStep(state *world.State, result turn.Result, reply response.Response) Step {
	snapshot := Capture(state)
	return Step{
		Player: "look around",
		Before: snapshot,
		After:  snapshot,
		Turn: session.ProcessedTurn{
			Result:   result,
			Response: reply,
		},
	}
}

func compoundResponseStep(t *testing.T, outcomes []turn.ActionOutcome, reply response.Response) (*world.State, Step) {
	t.Helper()
	beforeState := scenario.NewPrototypeWorld()
	afterState := scenario.NewPrototypeWorld()
	afterState.CurrentRoomID = scenario.RoomStorage
	afterState.PreviousRoomID = scenario.RoomReception
	if err := afterState.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil {
		t.Fatal(err)
	}
	return afterState, Step{
		Player: "go east and look around",
		Before: Capture(beforeState),
		After:  Capture(afterState),
		Turn: session.ProcessedTurn{
			Result:   turn.Result{Outcomes: outcomes},
			Response: reply,
		},
	}
}

func normalResult() turn.Result {
	return turn.Result{Outcomes: []turn.ActionOutcome{{
		Result: game.ActionResult{VisibleFacts: []game.Fact{{Text: "I search the desk.", Required: true}}},
	}}}
}

func assertResponseViolation(t *testing.T, violations []Violation, code string) {
	t.Helper()
	if !hasViolation(violations, code) {
		t.Fatalf("violations = %#v, want %q", violations, code)
	}
}
