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
