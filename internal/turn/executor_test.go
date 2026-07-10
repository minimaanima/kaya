package turn

import (
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

func TestExecutorSearchesBothDoctorsInOrder(t *testing.T) {
	state := newLitStorageState(t)
	executor := NewExecutor(state)
	start := state.NowSeconds
	result := executor.Execute(intent.TurnPlan{
		Actions:   []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionSearch, Target: "doctors"}, TargetMode: intent.TargetAll}},
		Questions: []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "they", TargetMode: intent.TargetAll}},
	})
	if len(result.Outcomes) != 2 {
		t.Fatalf("outcomes = %#v", result.Outcomes)
	}
	if result.Outcomes[0].TargetObjectID != scenario.ObjectBodyCabinet || result.Outcomes[1].TargetObjectID != scenario.ObjectBodyDoor {
		t.Fatalf("order = %#v", result.Outcomes)
	}
	if state.NowSeconds-start != 65 {
		t.Fatalf("elapsed = %d, want 65", state.NowSeconds-start)
	}
	if len(result.QuestionFacts) != 2 || result.QuestionFacts[0].Value != "dead" || result.QuestionFacts[1].Value != "dead" {
		t.Fatalf("facts = %#v", result.QuestionFacts)
	}
}

func TestExecutorPreservesFirstTargetWhenSecondRefuses(t *testing.T) {
	state := newLitStorageState(t)
	resolver := &sequenceResolver{state: state, results: []game.ActionResult{
		{Status: game.ActionSucceeded, Outcome: "searched_empty", DurationSeconds: 30},
		{Status: game.ActionRefused, Outcome: "kaya_refused"},
	}}
	executor := newExecutor(state, resolver)
	result := executor.Execute(doctorSearchPlan())
	if len(result.Outcomes) != 2 || result.Outcomes[0].Result.Status != game.ActionSucceeded || result.Outcomes[1].Result.Status != game.ActionRefused {
		t.Fatalf("outcomes = %#v", result.Outcomes)
	}
	if state.NowSeconds != 30 {
		t.Fatalf("time = %d, want committed first action time", state.NowSeconds)
	}
}

func TestExecutorClarifiesEmptyPlan(t *testing.T) {
	state := newLitStorageState(t)
	result := NewExecutor(state).Execute(intent.TurnPlan{})
	if len(result.Outcomes) != 0 {
		t.Fatalf("outcomes = %#v, want none", result.Outcomes)
	}
	if result.StopReason != "clarification" || result.ClarificationQuestion == "" {
		t.Fatalf("result = %#v, want clarification", result)
	}
}

func TestExecutorInspectsCurrentRoomWithoutTarget(t *testing.T) {
	state := newLitStorageState(t)
	result := NewExecutor(state).Execute(intent.TurnPlan{
		Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionInspect}, TargetMode: intent.TargetSingle}},
	})
	if len(result.Outcomes) != 1 || result.Outcomes[0].Result.Status != game.ActionSucceeded {
		t.Fatalf("outcomes = %#v, want successful room inspection", result.Outcomes)
	}
}

func TestExecutorExecutesFallbackObjectTargets(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	executor := NewExecutor(state)
	for _, message := range []string{"search the desk", "take the flashlight"} {
		result := executor.Execute(intent.FallbackPlan(message))
		if len(result.Outcomes) != 1 {
			t.Fatalf("%q outcomes = %#v", message, result.Outcomes)
		}
	}
	if !state.HasItem(scenario.ItemFlashlight) {
		t.Fatal("fallback take plan did not add flashlight")
	}
}

func TestExecutorClarifiesUnresolvedFactQuestionAfterAction(t *testing.T) {
	state := newLitStorageState(t)
	result := NewExecutor(state).Execute(intent.TurnPlan{
		Actions:   []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionSearch, Target: "doctor near door"}, TargetMode: intent.TargetSingle}},
		Questions: []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "they", TargetMode: intent.TargetAll}},
	})
	if len(result.Outcomes) != 1 || result.Outcomes[0].Result.Status != game.ActionSucceeded {
		t.Fatalf("outcomes = %#v, want completed action", result.Outcomes)
	}
	if result.StopReason != "clarification" || result.ClarificationQuestion == "" {
		t.Fatalf("result = %#v, want question clarification", result)
	}
}

func TestExecutorClarifiesAmbiguousFactQuestion(t *testing.T) {
	state := newLitStorageState(t)
	result := NewExecutor(state).Execute(intent.TurnPlan{
		Questions: []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "doctor", TargetMode: intent.TargetSingle}},
	})
	if result.StopReason != "clarification" || result.ClarificationQuestion == "" {
		t.Fatalf("result = %#v, want question clarification", result)
	}
}

func TestFactBundlePreservesOptionalVisibleFact(t *testing.T) {
	result := Result{Outcomes: []ActionOutcome{{Result: game.ActionResult{
		Status:       game.ActionSucceeded,
		VisibleFacts: []game.Fact{{Kind: game.FactAction, Text: "optional", Required: false}},
	}}}}
	bundle := result.FactBundle("look")
	if len(bundle.Facts) != 1 || bundle.Facts[0].Required {
		t.Fatalf("facts = %#v, want optional fact preserved", bundle.Facts)
	}
}

func TestFactBundleIncludesPlanClarification(t *testing.T) {
	bundle := (Result{ClarificationQuestion: "What should I inspect?"}).FactBundle("look")
	if len(bundle.Facts) != 1 || bundle.Facts[0].Kind != game.FactClarification || !bundle.Facts[0].Required {
		t.Fatalf("facts = %#v, want required clarification", bundle.Facts)
	}
}

func newLitStorageState(t *testing.T) *world.State {
	t.Helper()
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil {
		t.Fatal(err)
	}
	return state
}

func doctorSearchPlan() intent.TurnPlan {
	return intent.TurnPlan{Actions: []intent.PlannedAction{{
		Intent:     intent.Intent{Action: intent.ActionSearch, Target: "doctors"},
		TargetMode: intent.TargetAll,
	}}}
}

type sequenceResolver struct {
	state   *world.State
	results []game.ActionResult
}

func (r *sequenceResolver) Resolve(intent.Intent) game.ActionResult {
	result := r.results[0]
	r.results = r.results[1:]
	if result.DurationSeconds > 0 {
		r.state.Advance(result.DurationSeconds)
	}
	return result
}
