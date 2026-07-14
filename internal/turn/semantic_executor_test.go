package turn

import (
	"testing"

	"kaya/internal/game"
	"kaya/internal/grounding"
	"kaya/internal/intent"
	"kaya/internal/scenario"
)

func TestExecuteSemanticGroundsEachActionAfterPreviousMutation(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	start := state.NowSeconds
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.SearchAction{Target: reference("desk"), Evidence: "search the desk"},
		intent.TakeAction{Target: reference("flashlight"), Evidence: "take the flashlight"},
	}}

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if got.Pending != nil || len(got.Result.Outcomes) != 2 {
		t.Fatalf("execution = %#v, want two completed actions", got)
	}
	if !state.HasItem(scenario.ItemFlashlight) {
		t.Fatal("flashlight was not grounded after search and taken")
	}
	if elapsed := state.NowSeconds - start; elapsed != 40 {
		t.Fatalf("elapsed = %d, want 40", elapsed)
	}
	if got.Result.Outcomes[0].Result.DurationSeconds != 35 || got.Result.Outcomes[1].Result.DurationSeconds != 5 {
		t.Fatalf("durations = %d, %d", got.Result.Outcomes[0].Result.DurationSeconds, got.Result.Outcomes[1].Result.DurationSeconds)
	}
	if len(got.Result.Outcomes[0].Result.VisibleFacts) == 0 || len(got.Result.Outcomes[1].Result.VisibleFacts) == 0 {
		t.Fatalf("visible facts were not preserved: %#v", got.Result.Outcomes)
	}
}

func TestExecuteSemanticGroundsDestinationActionAfterMove(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.ActiveLight = true
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.MoveAction{Direction: "east", Evidence: "go east"},
		intent.InspectAction{Target: reference("storage cabinet"), Evidence: "inspect the storage cabinet"},
	}}

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if got.Pending != nil || len(got.Result.Outcomes) != 2 {
		t.Fatalf("execution = %#v, want move and inspect", got)
	}
	if state.CurrentRoomID != scenario.RoomStorage {
		t.Fatalf("room = %q, want %q", state.CurrentRoomID, scenario.RoomStorage)
	}
	if got.Result.Outcomes[1].TargetObjectID != scenario.ObjectStorageCabinet || got.Result.Outcomes[1].Result.Outcome != "inspected_object" {
		t.Fatalf("inspect outcome = %#v", got.Result.Outcomes[1])
	}
}

func TestExecuteSemanticStopsAtAmbiguousActionAfterCompletedAction(t *testing.T) {
	state := newLitStorageState(t)
	start := state.NowSeconds
	plan := waitThenSearchDoctorPlan()

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if len(got.Result.Outcomes) != 1 || got.Result.Outcomes[0].Result.Outcome != "waited" {
		t.Fatalf("completed outcomes = %#v, want only wait", got.Result.Outcomes)
	}
	if state.NowSeconds-start != 10 {
		t.Fatalf("elapsed = %d, want completed wait only", state.NowSeconds-start)
	}
	if got.Pending == nil {
		t.Fatal("missing pending clarification")
	}
	if got.Pending.ActionIndex != 1 || got.Pending.Role != grounding.RoleObject || len(got.Pending.Candidates) != 2 {
		t.Fatalf("pending = %#v", got.Pending)
	}
	if len(got.Pending.RemainingPlan.Actions) != 1 || got.Pending.RemainingPlan.Actions[0].ActionKind() != intent.ActionSearch {
		t.Fatalf("remaining plan = %#v", got.Pending.RemainingPlan)
	}
}

func TestExecuteSemanticResumesExactPendingActionWithoutReplay(t *testing.T) {
	state := newLitStorageState(t)
	executor := NewExecutor(state)
	plan := waitThenSearchDoctorPlan()
	first := executor.ExecuteSemantic(plan, 0, nil)
	if first.Pending == nil {
		t.Fatal("missing pending clarification")
	}
	afterFirst := state.NowSeconds
	binding := &grounding.Binding{Role: first.Pending.Role, CandidateIDs: []string{string(scenario.ObjectBodyDoor)}}

	second := executor.ExecuteSemantic(plan, first.Pending.ActionIndex, binding)

	if second.Pending != nil || len(second.Result.Outcomes) != 1 {
		t.Fatalf("resumed execution = %#v, want one completed search", second)
	}
	if second.Result.Outcomes[0].TargetObjectID != scenario.ObjectBodyDoor {
		t.Fatalf("target = %q, want %q", second.Result.Outcomes[0].TargetObjectID, scenario.ObjectBodyDoor)
	}
	if elapsed := state.NowSeconds - afterFirst; elapsed != 30 {
		t.Fatalf("resume elapsed = %d, want 30 without replaying wait", elapsed)
	}
}

func TestExecuteSemanticPassesGroundedIDToCanonicalResolverBoundary(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	desk := state.Objects[scenario.ObjectReceptionDesk]
	desk.Name = "Archive Plinth"
	desk.Aliases = []string{"console"}
	state.Objects[desk.ID] = desk
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.SearchAction{Target: reference("console"), Evidence: "search the console"},
	}}

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if got.Pending != nil || len(got.Result.Outcomes) != 1 {
		t.Fatalf("execution = %#v", got)
	}
	if outcome := got.Result.Outcomes[0]; outcome.TargetObjectID != scenario.ObjectReceptionDesk || outcome.Result.Status != game.ActionSucceeded {
		t.Fatalf("outcome = %#v, want canonical desk selection", outcome)
	}
}

func TestExecuteSemanticPreservesEventsFromResolver(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.NowSeconds = 40
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.WaitAction{Evidence: "wait"},
	}}

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if len(got.Result.Outcomes) != 1 || len(got.Result.Outcomes[0].Result.Events) != 1 {
		t.Fatalf("outcomes = %#v, want scheduled event", got.Result.Outcomes)
	}
	if got.Result.Outcomes[0].Result.StartedAtSeconds != 40 {
		t.Fatalf("started at = %d, want 40", got.Result.Outcomes[0].Result.StartedAtSeconds)
	}
}

func TestExecuteSemanticPreservesPluralReferentForQuestions(t *testing.T) {
	state := newLitStorageState(t)
	plan := intent.SemanticPlan{
		Actions: []intent.SemanticAction{
			intent.SearchAction{
				Target:   intent.Reference{Mention: "doctors", Quantity: intent.TargetAll},
				Evidence: "search the doctors",
			},
		},
		Questions: []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "they", TargetMode: intent.TargetAll}},
	}

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if got.Pending != nil || got.Result.StopReason != "" {
		t.Fatalf("execution = %#v, want completed plural action and question", got)
	}
	if len(got.Result.Outcomes) != 2 || len(got.Result.QuestionFacts) != 2 {
		t.Fatalf("outcomes = %#v facts = %#v, want both doctors", got.Result.Outcomes, got.Result.QuestionFacts)
	}
}

func TestExecuteSemanticPreservesAutonomyClarification(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.Kaya.Trust = 0
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.MoveAction{Direction: "east", Evidence: "go east"},
	}}

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if got.Pending != nil || len(got.Result.Outcomes) != 1 {
		t.Fatalf("execution = %#v", got)
	}
	if outcome := got.Result.Outcomes[0].Result; outcome.Status != game.ActionClarification || !outcome.NeedsClarification {
		t.Fatalf("outcome = %#v, want autonomy clarification", outcome)
	}
	if state.CurrentRoomID != scenario.RoomReception || state.NowSeconds != 0 {
		t.Fatalf("world mutated on autonomy clarification: room=%q time=%d", state.CurrentRoomID, state.NowSeconds)
	}
}

func TestExecuteSemanticStopsAfterResolverFailure(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	chair := state.Objects[scenario.ObjectCollapsedChair]
	chair.Searchable = false
	state.Objects[chair.ID] = chair
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.SearchAction{Target: reference("chair"), Evidence: "search the chair"},
		intent.WaitAction{Evidence: "wait"},
	}}

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if len(got.Result.Outcomes) != 1 || got.Result.Outcomes[0].Result.Status != game.ActionFailed {
		t.Fatalf("outcomes = %#v, want one failed search", got.Result.Outcomes)
	}
	if state.NowSeconds != 2 {
		t.Fatalf("elapsed = %d, want failed action duration only", state.NowSeconds)
	}
}

func reference(mention string) intent.Reference {
	return intent.Reference{Mention: mention, Quantity: intent.TargetOne}
}

func waitThenSearchDoctorPlan() intent.SemanticPlan {
	return intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.WaitAction{Evidence: "wait"},
		intent.SearchAction{Target: reference("doctor"), Evidence: "search the doctor"},
	}}
}
