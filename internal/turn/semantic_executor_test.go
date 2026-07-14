package turn

import (
	"testing"

	"kaya/internal/game"
	"kaya/internal/grounding"
	"kaya/internal/intent"
	"kaya/internal/scenario"
	"kaya/internal/world"
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

func TestExecuteSemanticPreservesResolvedUseRoleAcrossSecondClarification(t *testing.T) {
	state, keyID, doorID := ambiguousUseState(t)
	executor := NewExecutor(state)
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.UseAction{
			Item:     reference("key"),
			Target:   reference("door"),
			Evidence: "use the key on the door",
		},
	}}

	first := executor.ExecuteSemantic(plan, 0, nil)
	if first.Pending == nil || first.Pending.Role != grounding.RoleItem {
		t.Fatalf("first pending = %#v, want item clarification", first.Pending)
	}
	second := executor.ExecuteSemantic(first.Pending.RemainingPlan, 0, &grounding.Binding{
		Role: grounding.RoleItem, CandidateIDs: []string{string(keyID)},
	})
	if second.Pending == nil || second.Pending.Role != grounding.RoleDoor {
		t.Fatalf("second pending = %#v, want door clarification", second.Pending)
	}

	third := executor.ExecuteSemantic(second.Pending.RemainingPlan, 0, &grounding.Binding{
		Role: grounding.RoleDoor, CandidateIDs: []string{string(doorID)},
	})

	if third.Pending != nil || len(third.Result.Outcomes) != 1 {
		t.Fatalf("third execution = %#v, want one completed use", third)
	}
	if got := state.Doors[doorID].State; got != world.DoorClosed {
		t.Fatalf("selected door state = %q, want %q", got, world.DoorClosed)
	}
	if state.NowSeconds != 8 {
		t.Fatalf("elapsed = %d, want one unlock duration", state.NowSeconds)
	}
}

func TestExecuteSemanticPreservesPluralUseBindingAcrossDoorClarification(t *testing.T) {
	state, _, doorID := ambiguousUseState(t)
	resolver := &semanticRecordingResolver{state: state}
	executor := newExecutor(state, resolver)
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.UseAction{Item: reference("key"), Target: reference("door"), Evidence: "use both keys on the door"},
	}}
	first := executor.ExecuteSemantic(plan, 0, nil)
	itemIDs := pendingCandidateIDs(t, first.Pending, grounding.RoleItem)

	second := executor.ExecuteSemantic(first.Pending.RemainingPlan, 0, &grounding.Binding{
		Role: grounding.RoleItem, CandidateIDs: itemIDs,
	})
	if second.Pending == nil || second.Pending.Role != grounding.RoleDoor {
		t.Fatalf("second pending = %#v, want door clarification", second.Pending)
	}
	if len(resolver.intents) != 0 || state.NowSeconds != 0 {
		t.Fatalf("executed before both roles resolved: intents=%#v time=%d", resolver.intents, state.NowSeconds)
	}

	third := executor.ExecuteSemantic(second.Pending.RemainingPlan, 0, &grounding.Binding{
		Role: grounding.RoleDoor, CandidateIDs: []string{string(doorID)},
	})

	if third.Pending != nil || len(third.Result.Outcomes) != 2 || len(resolver.intents) != 2 {
		t.Fatalf("third execution = %#v intents=%#v, want both bound items", third, resolver.intents)
	}
	for index, in := range resolver.intents {
		if in.Item != itemIDs[index] || in.Target != string(doorID) {
			t.Fatalf("intent %d = %#v, want item %q on door %q", index, in, itemIDs[index], doorID)
		}
	}
	if state.NowSeconds != 8 {
		t.Fatalf("elapsed = %d, want exactly two resumed actions", state.NowSeconds)
	}
}

func TestExecuteSemanticRejectsStaleMemberOfPersistedPluralUseBinding(t *testing.T) {
	state, _, doorID := ambiguousUseState(t)
	resolver := &semanticRecordingResolver{state: state}
	executor := newExecutor(state, resolver)
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.UseAction{Item: reference("key"), Target: reference("door"), Evidence: "use both keys on the door"},
	}}
	first := executor.ExecuteSemantic(plan, 0, nil)
	itemIDs := pendingCandidateIDs(t, first.Pending, grounding.RoleItem)
	second := executor.ExecuteSemantic(first.Pending.RemainingPlan, 0, &grounding.Binding{
		Role: grounding.RoleItem, CandidateIDs: itemIDs,
	})
	if second.Pending == nil || second.Pending.Role != grounding.RoleDoor {
		t.Fatalf("second pending = %#v, want door clarification", second.Pending)
	}
	state.RemoveInventory(game.ItemID(itemIDs[1]))

	got := executor.ExecuteSemantic(second.Pending.RemainingPlan, 0, &grounding.Binding{
		Role: grounding.RoleDoor, CandidateIDs: []string{string(doorID)},
	})

	if got.Pending != nil || len(got.Result.Outcomes) != 1 || got.Result.Outcomes[0].Result.Outcome != string(grounding.MissingReasonStaleBinding) {
		t.Fatalf("execution = %#v, want stale persisted binding failure", got)
	}
	if len(resolver.intents) != 0 || state.NowSeconds != 0 {
		t.Fatalf("stale binding executed: intents=%#v time=%d", resolver.intents, state.NowSeconds)
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

func TestExecuteSemanticPreservesSameNameObjectIdentityAtResolverBoundary(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	for _, id := range []game.ObjectID{scenario.ObjectReceptionDesk, scenario.ObjectCollapsedChair} {
		object := state.Objects[id]
		object.Name = "Console"
		object.Aliases = []string{"console"}
		state.Objects[id] = object
	}
	executor := NewExecutor(state)
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.SearchAction{Target: reference("console"), Evidence: "search the console"},
	}}
	first := executor.ExecuteSemantic(plan, 0, nil)
	if first.Pending == nil || len(first.Pending.Candidates) != 2 {
		t.Fatalf("first execution = %#v, want same-name clarification", first)
	}

	got := executor.ExecuteSemantic(plan, first.Pending.ActionIndex, &grounding.Binding{
		Role: grounding.RoleObject, CandidateIDs: []string{string(scenario.ObjectCollapsedChair)},
	})

	if got.Pending != nil || len(got.Result.Outcomes) != 1 {
		t.Fatalf("execution = %#v, want one exact search", got)
	}
	outcome := got.Result.Outcomes[0]
	if outcome.Result.Status != game.ActionSucceeded || len(outcome.Result.TargetObjectIDs) != 1 || outcome.Result.TargetObjectIDs[0] != scenario.ObjectCollapsedChair {
		t.Fatalf("outcome = %#v, want selected same-name object", outcome)
	}
}

func TestExecuteSemanticPreservesSameNameItemIdentityAtResolverBoundary(t *testing.T) {
	const (
		itemA game.ItemID = "token_a"
		itemB game.ItemID = "token_b"
	)
	state := scenario.NewPrototypeWorld()
	state.Items[itemA] = world.Item{ID: itemA, Name: "Token", Aliases: []string{"token"}, Portable: true}
	state.Items[itemB] = world.Item{ID: itemB, Name: "Token", Aliases: []string{"token"}, Portable: true}
	desk := state.Objects[scenario.ObjectReceptionDesk]
	desk.ContainedItems = []game.ItemID{itemA}
	state.Objects[desk.ID] = desk
	chair := state.Objects[scenario.ObjectCollapsedChair]
	chair.ContainedItems = []game.ItemID{itemB}
	state.Objects[chair.ID] = chair
	state.DiscoverItems([]game.ItemID{itemA, itemB})
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.TakeAction{Target: reference("token"), Evidence: "take the token"},
	}}
	executor := NewExecutor(state)
	first := executor.ExecuteSemantic(plan, 0, nil)
	if first.Pending == nil || len(first.Pending.Candidates) != 2 {
		t.Fatalf("first execution = %#v, want same-name clarification", first)
	}

	got := executor.ExecuteSemantic(plan, first.Pending.ActionIndex, &grounding.Binding{
		Role: grounding.RoleItem, CandidateIDs: []string{string(itemB)},
	})

	if got.Pending != nil || len(got.Result.Outcomes) != 1 || got.Result.Outcomes[0].Result.Status != game.ActionSucceeded {
		t.Fatalf("execution = %#v, want one exact take", got)
	}
	if !state.HasItem(itemB) || state.HasItem(itemA) {
		t.Fatalf("inventory = %#v, want only selected item %q", state.Inventory, itemB)
	}
}

func TestExecuteSemanticPreservesSameNameUseIdentityAtResolverBoundary(t *testing.T) {
	state, keyID, doorID := ambiguousUseState(t)
	for id, item := range state.Items {
		item.Name = "Brass Key"
		state.Items[id] = item
	}
	for id, door := range state.Doors {
		door.Name = "Door"
		state.Doors[id] = door
	}
	executor := NewExecutor(state)
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.UseAction{Item: reference("key"), Target: reference("door"), Evidence: "use the key on the door"},
	}}
	first := executor.ExecuteSemantic(plan, 0, nil)
	second := executor.ExecuteSemantic(first.Pending.RemainingPlan, 0, &grounding.Binding{
		Role: grounding.RoleItem, CandidateIDs: []string{string(keyID)},
	})

	got := executor.ExecuteSemantic(second.Pending.RemainingPlan, 0, &grounding.Binding{
		Role: grounding.RoleDoor, CandidateIDs: []string{string(doorID)},
	})

	if got.Pending != nil || len(got.Result.Outcomes) != 1 || got.Result.Outcomes[0].Result.Status != game.ActionSucceeded {
		t.Fatalf("execution = %#v, want one exact use", got)
	}
	if state.Doors[doorID].State != world.DoorClosed {
		t.Fatalf("selected door state = %q, want %q", state.Doors[doorID].State, world.DoorClosed)
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

func TestExecuteSemanticPreservesPluralItemReferentGroup(t *testing.T) {
	const (
		itemA     game.ItemID = "token_a"
		itemB     game.ItemID = "token_b"
		unrelated game.ItemID = "badge"
	)
	state := scenario.NewPrototypeWorld()
	state.Items[itemA] = world.Item{ID: itemA, Name: "Red Token", Aliases: []string{"token"}, Portable: true}
	state.Items[itemB] = world.Item{ID: itemB, Name: "Blue Token", Aliases: []string{"token"}, Portable: true}
	state.Items[unrelated] = world.Item{ID: unrelated, Name: "Badge", Portable: true}
	desk := state.Objects[scenario.ObjectReceptionDesk]
	desk.ContainedItems = []game.ItemID{itemA, itemB, unrelated}
	state.Objects[desk.ID] = desk
	state.DiscoverItems([]game.ItemID{itemA, itemB, unrelated})
	plan := intent.SemanticPlan{Actions: []intent.SemanticAction{
		intent.TakeAction{
			Target:   intent.Reference{Mention: "token", Quantity: intent.TargetAll},
			Evidence: "take the tokens",
		},
	}}

	got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

	if got.Pending != nil || len(got.Result.Outcomes) != 2 {
		t.Fatalf("execution = %#v, want two completed takes", got)
	}
	if len(state.RecentReferents) == 0 {
		t.Fatal("missing recent referent group")
	}
	group := state.RecentReferents[len(state.RecentReferents)-1].ItemIDs
	if len(group) != 2 || group[0] != itemA || group[1] != itemB {
		t.Fatalf("recent item group = %#v, want [%q %q] without %q", group, itemA, itemB, unrelated)
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

func TestExecuteSemanticEvaluatesFactQuestionsAfterResolverNonSuccess(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*world.State)
		action     intent.SemanticAction
		wantStatus game.ActionStatus
		wantTime   int
	}{
		{
			name: "failure",
			setup: func(state *world.State) {
				chair := state.Objects[scenario.ObjectCollapsedChair]
				chair.Searchable = false
				state.Objects[chair.ID] = chair
			},
			action:     intent.SearchAction{Target: reference("chair"), Evidence: "search the chair"},
			wantStatus: game.ActionFailed,
			wantTime:   2,
		},
		{
			name: "refusal",
			setup: func(state *world.State) {
				state.Kaya.Trust = 0
				state.Kaya.Stress = 60
			},
			action:     intent.MoveAction{Direction: "east", Evidence: "go east"},
			wantStatus: game.ActionRefused,
		},
		{
			name: "autonomy clarification",
			setup: func(state *world.State) {
				state.Kaya.Trust = 0
			},
			action:     intent.MoveAction{Direction: "east", Evidence: "go east"},
			wantStatus: game.ActionClarification,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := scenario.NewPrototypeWorld()
			tt.setup(state)
			plan := intent.SemanticPlan{
				Actions:   []intent.SemanticAction{tt.action, intent.WaitAction{Evidence: "wait"}},
				Questions: []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "desk", TargetMode: intent.TargetOne}},
			}

			got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)

			if len(got.Result.Outcomes) != 1 || got.Result.Outcomes[0].Result.Status != tt.wantStatus {
				t.Fatalf("outcomes = %#v, want one %q outcome", got.Result.Outcomes, tt.wantStatus)
			}
			if len(got.Result.QuestionFacts) != 1 || got.Result.QuestionFacts[0].Kind != game.FactFailure {
				t.Fatalf("question facts = %#v, want represented unknown life status", got.Result.QuestionFacts)
			}
			if state.NowSeconds != tt.wantTime {
				t.Fatalf("elapsed = %d, want %d with no later wait", state.NowSeconds, tt.wantTime)
			}
		})
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

func ambiguousUseState(t *testing.T) (*world.State, game.ItemID, game.DoorID) {
	t.Helper()
	const (
		roomID game.RoomID = "junction"
		keyA   game.ItemID = "key_a"
		keyB   game.ItemID = "key_b"
		doorA  game.DoorID = "door_a"
		doorB  game.DoorID = "door_b"
	)
	state := world.NewState(roomID)
	state.Rooms[roomID] = world.Room{
		ID: roomID, Name: "Junction", Visibility: world.VisibilityLit,
		Exits: []world.Exit{
			{Direction: "north", To: "north_room", Door: doorA},
			{Direction: "south", To: "south_room", Door: doorB},
		},
	}
	state.Rooms["north_room"] = world.Room{ID: "north_room", Name: "North Room", Visibility: world.VisibilityLit}
	state.Rooms["south_room"] = world.Room{ID: "south_room", Name: "South Room", Visibility: world.VisibilityLit}
	state.Items[keyA] = world.Item{ID: keyA, Name: "Brass Key", Aliases: []string{"key"}, Portable: true}
	state.Items[keyB] = world.Item{ID: keyB, Name: "Small Key", Aliases: []string{"key"}, Portable: true}
	state.AddInventory(keyA)
	state.AddInventory(keyB)
	state.Doors[doorA] = world.Door{ID: doorA, Name: "North Door", Aliases: []string{"door"}, From: roomID, To: "north_room", State: world.DoorLocked, RequiredKey: keyB}
	state.Doors[doorB] = world.Door{ID: doorB, Name: "South Door", Aliases: []string{"door"}, From: roomID, To: "south_room", State: world.DoorLocked, RequiredKey: keyA}
	if err := state.ObserveRoom(roomID, ""); err != nil {
		t.Fatal(err)
	}
	return state, keyA, doorB
}

func pendingCandidateIDs(t *testing.T, pending *PendingSemanticAction, role grounding.Role) []string {
	t.Helper()
	if pending == nil || pending.Role != role {
		t.Fatalf("pending = %#v, want role %q", pending, role)
	}
	ids := make([]string, 0, len(pending.Candidates))
	for _, candidate := range pending.Candidates {
		ids = append(ids, candidate.ID)
	}
	return ids
}

type semanticRecordingResolver struct {
	state   *world.State
	intents []intent.Intent
}

func (r *semanticRecordingResolver) Resolve(in intent.Intent) game.ActionResult {
	r.intents = append(r.intents, in)
	result := game.ActionResult{
		Status:           game.ActionSucceeded,
		StartedAtSeconds: r.state.NowSeconds,
		DurationSeconds:  4,
		Outcome:          "recorded_use",
	}
	r.state.Advance(result.DurationSeconds)
	return result
}
