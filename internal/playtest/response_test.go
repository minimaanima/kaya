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

func TestCheckResponseAttributesDarkThenLitAwarenessBySentenceEvidence(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	before := Capture(state)
	state.ActiveLight = true
	after := Capture(state)
	result := turn.Result{Outcomes: []turn.ActionOutcome{
		roomAwarenessWithVisibleFact("none", "I cannot make out any distinct objects."),
		{Intent: intent.Intent{Action: intent.ActionTurnOn, Item: "flashlight"}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "flashlight_on", VisibleFacts: []game.Fact{{Kind: game.FactAction, Text: "I turn on the flashlight.", Required: true}}}},
		roomAwarenessWithVisibleFact("Storage Cabinet", "I can see: Storage Cabinet."),
	}}
	reply := response.Response{
		Text: "I cannot make out any distinct objects. I turn on the flashlight. I can see: Storage Cabinet.",
		Sentences: []response.ResponseSentence{
			{Text: "I cannot make out any distinct objects.", FactIDs: []game.FactID{"f001"}},
			{Text: "I turn on the flashlight.", FactIDs: []game.FactID{"f002"}},
			{Text: "I can see: Storage Cabinet.", FactIDs: []game.FactID{"f003"}},
		},
		UsedFactIDs: []game.FactID{"f001", "f002", "f003"},
	}
	step := Step{Player: "look around then turn on the flashlight then look around", Before: before, After: after, Turn: session.ProcessedTurn{Result: result, Response: reply}}

	if violations := CheckResponse(step, state); hasViolation(violations, "response_darkness_leak") {
		t.Fatalf("lit sentence was attributed to earlier dark awareness: %#v", violations)
	}
}

func TestCheckResponseAttributesLitThenDarkAwarenessBySentenceEvidence(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	before := Capture(state)
	state.ActiveLight = false
	after := Capture(state)
	result := turn.Result{Outcomes: []turn.ActionOutcome{
		roomAwarenessWithVisibleFact("Storage Cabinet", "I can see: Storage Cabinet."),
		{Intent: intent.Intent{Action: intent.ActionTurnOff, Item: "flashlight"}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "flashlight_off", VisibleFacts: []game.Fact{{Kind: game.FactAction, Text: "I turn off the flashlight.", Required: true}}}},
		roomAwarenessWithVisibleFact("none", "I cannot make out any distinct objects."),
	}}
	reply := response.Response{
		Text: "I can see: Storage Cabinet. I turn off the flashlight. I cannot make out any distinct objects.",
		Sentences: []response.ResponseSentence{
			{Text: "I can see: Storage Cabinet.", FactIDs: []game.FactID{"f001"}},
			{Text: "I turn off the flashlight.", FactIDs: []game.FactID{"f002"}},
			{Text: "I cannot make out any distinct objects.", FactIDs: []game.FactID{"f003"}},
		},
		UsedFactIDs: []game.FactID{"f001", "f002", "f003"},
	}
	step := Step{Player: "look around then turn off the flashlight then look around", Before: before, After: after, Turn: session.ProcessedTurn{Result: result, Response: reply}}

	if violations := CheckResponse(step, state); hasViolation(violations, "response_darkness_leak") {
		t.Fatalf("lit sentence was attributed to later dark awareness: %#v", violations)
	}
}

func TestCheckResponseRejectsDarkDirectionLeakBeforeLaterLitAwareness(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil {
		t.Fatal(err)
	}
	before := Capture(state)
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, ""); err != nil {
		t.Fatal(err)
	}
	after := Capture(state)
	result := turn.Result{Outcomes: []turn.ActionOutcome{
		roomAwarenessWithVisibleAndExitFacts("none", "west"),
		{Intent: intent.Intent{Action: intent.ActionTurnOn, Item: "flashlight"}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "flashlight_on", VisibleFacts: []game.Fact{{Kind: game.FactAction, Text: "I turn on the flashlight.", Required: true}}}},
		roomAwarenessWithVisibleAndExitFacts("Storage Cabinet", "west, north"),
	}}
	reply := response.Response{
		Text: "I can go north. I turn on the flashlight. I can go north.",
		Sentences: []response.ResponseSentence{
			{Text: "I can go north.", FactIDs: []game.FactID{"f002"}},
			{Text: "I turn on the flashlight.", FactIDs: []game.FactID{"f003"}},
			{Text: "I can go north.", FactIDs: []game.FactID{"f005"}},
		},
		UsedFactIDs: []game.FactID{"f002", "f003", "f005"},
	}
	step := Step{Player: "look around then turn on the flashlight then look around", Before: before, After: after, Turn: session.ProcessedTurn{Result: result, Response: reply}}

	assertResponseViolation(t, CheckResponse(step, state), "response_darkness_leak")
}

func TestCheckResponseAllowsKnownLitDirectionAfterLightTurnsOff(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, ""); err != nil {
		t.Fatal(err)
	}
	before := Capture(state)
	state.ActiveLight = false
	after := Capture(state)
	result := turn.Result{Outcomes: []turn.ActionOutcome{
		roomAwarenessWithVisibleAndExitFacts("Storage Cabinet", "west, north"),
		{Intent: intent.Intent{Action: intent.ActionTurnOff, Item: "flashlight"}, Result: game.ActionResult{Status: game.ActionSucceeded, Outcome: "flashlight_off", VisibleFacts: []game.Fact{{Kind: game.FactAction, Text: "I turn off the flashlight.", Required: true}}}},
		roomAwarenessWithVisibleAndExitFacts("none", "west, north"),
	}}
	reply := response.Response{
		Text: "I can go north. I turn off the flashlight. I can still go north.",
		Sentences: []response.ResponseSentence{
			{Text: "I can go north.", FactIDs: []game.FactID{"f002"}},
			{Text: "I turn off the flashlight.", FactIDs: []game.FactID{"f003"}},
			{Text: "I can still go north.", FactIDs: []game.FactID{"f005"}},
		},
		UsedFactIDs: []game.FactID{"f002", "f003", "f005"},
	}
	step := Step{Player: "look around then turn off the flashlight then look around", Before: before, After: after, Turn: session.ProcessedTurn{Result: result, Response: reply}}

	if violations := CheckResponse(step, state); hasViolation(violations, "response_darkness_leak") {
		t.Fatalf("known direction was treated as hidden after light-off: %#v", violations)
	}
}

func TestCheckResponseDoesNotExemptFallbackSentenceFromDarkness(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	result := turn.Result{Outcomes: []turn.ActionOutcome{roomAwarenessWithVisibleFact("none", "I cannot make out any distinct objects.")}}
	reply := response.Response{
		Text:         "I can see Storage Cabinet.",
		UsedFallback: true,
		Sentences: []response.ResponseSentence{{
			Text: "I can see Storage Cabinet.", FactIDs: []game.FactID{"f001"},
		}},
		UsedFactIDs: []game.FactID{"f001"},
	}

	assertResponseViolation(t, CheckResponse(responseStep(state, result, reply), state), "response_darkness_leak")
}

func TestCheckResponseRejectsDebugMarker(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	violations := CheckResponse(responseStep(state, normalResult(), response.Response{Text: "debug: I search the desk."}), state)
	assertResponseViolation(t, violations, "response_debug_marker")
}

func TestCheckResponseRejectsFallbackWithUngroundedFactID(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	violations := CheckResponse(responseStep(state, normalResult(), response.Response{
		Text:         "I search the desk.",
		UsedFallback: true,
		UsedFactIDs:  []game.FactID{"f999"},
	}), state)
	assertResponseViolation(t, violations, "response_fact_id_ungrounded")
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

func roomAwarenessWithVisibleFact(value, text string) turn.ActionOutcome {
	return turn.ActionOutcome{
		Intent: intent.Intent{Action: intent.ActionInspect},
		Result: game.ActionResult{
			Status:  game.ActionSucceeded,
			Outcome: "inspected_room",
			VisibleFacts: []game.Fact{{
				Kind: game.FactVisibleObjects, Value: value, Text: text, Required: true,
			}},
		},
	}
}

func roomAwarenessWithVisibleAndExitFacts(visibleObjects, knownExits string) turn.ActionOutcome {
	visibleText := "I can see: " + visibleObjects + "."
	if visibleObjects == "none" {
		visibleText = "I cannot make out any distinct objects."
	}
	return turn.ActionOutcome{
		Intent: intent.Intent{Action: intent.ActionInspect},
		Result: game.ActionResult{
			Status:  game.ActionSucceeded,
			Outcome: "inspected_room",
			VisibleFacts: []game.Fact{
				{Kind: game.FactVisibleObjects, Value: visibleObjects, Text: visibleText, Required: true},
				{Kind: game.FactKnownExits, Value: knownExits, Text: "I can go: " + knownExits + ".", Required: true},
			},
		},
	}
}
