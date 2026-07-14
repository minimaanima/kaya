package session

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"kaya/internal/game"
	"kaya/internal/grounding"
	"kaya/internal/intent"
	"kaya/internal/response"
	"kaya/internal/scenario"
	"kaya/internal/turn"
	"kaya/internal/world"
)

type clarificationGenerator struct {
	response string
	prompt   string
	payload  string
	schema   any
}

func (g *clarificationGenerator) GenerateJSON(_ context.Context, prompt, payload string, schema any) (string, error) {
	g.prompt = prompt
	g.payload = payload
	g.schema = schema
	return g.response, nil
}

func TestParseClarificationExposesOnlyCandidateViews(t *testing.T) {
	generator := &clarificationGenerator{response: `{"decision":"select","mention":"Doctor Near Door","ordinal":0}`}
	parser := intent.NewParser(generator)
	candidates := []intent.CandidateView{
		{Ordinal: 1, Name: "Doctor Near Cabinet", Aliases: []string{"doctor", "coat pockets"}},
		{Ordinal: 2, Name: "Doctor Near Door", Aliases: []string{"doctor", "body"}},
	}

	got, err := parser.ParseClarification(context.Background(), "the one near the door", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != intent.ClarificationSelect || got.Mention != "Doctor Near Door" || got.Ordinal != 0 {
		t.Fatalf("decision = %#v", got)
	}
	if generator.prompt != intent.ClarificationPrompt {
		t.Fatalf("prompt = %q", generator.prompt)
	}
	if !reflect.DeepEqual(generator.schema, intent.ClarificationDecisionSchema) {
		t.Fatalf("schema = %#v", generator.schema)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(generator.payload), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["player"] != "the one near the door" {
		t.Fatalf("player = %#v", payload["player"])
	}
	encoded, err := json.Marshal(payload["candidates"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "object_body") || strings.Contains(string(encoded), `"id"`) {
		t.Fatalf("candidate payload leaked an ID: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"ordinal":1`) || !strings.Contains(string(encoded), `"aliases"`) {
		t.Fatalf("candidate payload = %s", encoded)
	}
}

func TestParseClarificationReturnsTypedDecisions(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     intent.ClarificationDecision
	}{
		{name: "select by mention", response: `{"decision":"select","mention":"coat pockets","ordinal":0}`, want: intent.ClarificationDecision{Kind: intent.ClarificationSelect, Mention: "coat pockets"}},
		{name: "select by ordinal", response: `{"decision":"select","mention":"","ordinal":2}`, want: intent.ClarificationDecision{Kind: intent.ClarificationSelect, Ordinal: 2}},
		{name: "all", response: `{"decision":"all","mention":"","ordinal":0}`, want: intent.ClarificationDecision{Kind: intent.ClarificationAll}},
		{name: "confirm", response: `{"decision":"confirm","mention":"Doctor Near Door","ordinal":0}`, want: intent.ClarificationDecision{Kind: intent.ClarificationConfirm, Mention: "Doctor Near Door"}},
		{name: "bare confirm", response: `{"decision":"confirm","mention":"","ordinal":0}`, want: intent.ClarificationDecision{Kind: intent.ClarificationConfirm}},
		{name: "cancel", response: `{"decision":"cancel","mention":"","ordinal":0}`, want: intent.ClarificationDecision{Kind: intent.ClarificationCancel}},
		{name: "new command", response: `{"decision":"new_command","mention":"","ordinal":0}`, want: intent.ClarificationDecision{Kind: intent.ClarificationNewCommand}},
	}
	candidates := []intent.CandidateView{{Ordinal: 1, Name: "Doctor Near Cabinet"}, {Ordinal: 2, Name: "Doctor Near Door"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := intent.NewParser(&clarificationGenerator{response: tt.response})
			got, err := parser.ParseClarification(context.Background(), "answer", candidates)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decision = %#v, want %#v", got, tt.want)
			}
		})
	}
}

type scriptedSessionParser struct {
	plans                   map[string]intent.SemanticPlan
	decisions               map[string]intent.ClarificationDecision
	clarificationErrors     map[string]error
	semanticMessages        []string
	clarificationMessages   []string
	clarificationCandidates [][]intent.CandidateView
}

func (p *scriptedSessionParser) ParseSemanticWithProvenance(
	_ context.Context,
	message string,
	_ game.PerceptionSnapshot,
) (intent.SemanticPlan, intent.SemanticProvenance, error) {
	p.semanticMessages = append(p.semanticMessages, message)
	plan, ok := p.plans[message]
	if !ok {
		return intent.SemanticPlan{}, intent.SemanticProvenance{}, errors.New("unexpected semantic parse")
	}
	return plan, intent.SemanticProvenance{Source: intent.ParseSourceModel}, nil
}

func (p *scriptedSessionParser) ParseClarification(
	_ context.Context,
	message string,
	candidates []intent.CandidateView,
) (intent.ClarificationDecision, error) {
	p.clarificationMessages = append(p.clarificationMessages, message)
	p.clarificationCandidates = append(p.clarificationCandidates, append([]intent.CandidateView(nil), candidates...))
	if err := p.clarificationErrors[message]; err != nil {
		return intent.ClarificationDecision{}, err
	}
	decision, ok := p.decisions[message]
	if !ok {
		return intent.ClarificationDecision{}, errors.New("unexpected clarification parse")
	}
	return decision, nil
}

type semanticSessionComposer struct{}

func (semanticSessionComposer) Compose(ctx context.Context, bundle turn.FactBundle) response.Response {
	return response.NewComposer(nil).Compose(ctx, bundle)
}

func TestSessionResumesCandidateBoundSelections(t *testing.T) {
	tests := []struct {
		name     string
		answer   string
		decision intent.ClarificationDecision
		wantIDs  []game.ObjectID
	}{
		{
			name:     "exact name",
			answer:   "Doctor Near Door",
			decision: intent.ClarificationDecision{Kind: intent.ClarificationSelect, Mention: "Doctor Near Door"},
			wantIDs:  []game.ObjectID{scenario.ObjectBodyDoor},
		},
		{
			name:     "exact alias",
			answer:   "coat pockets",
			decision: intent.ClarificationDecision{Kind: intent.ClarificationSelect, Mention: "coat pockets"},
			wantIDs:  []game.ObjectID{scenario.ObjectBodyCabinet},
		},
		{
			name:     "ordinal",
			answer:   "the second one",
			decision: intent.ClarificationDecision{Kind: intent.ClarificationSelect, Ordinal: 2},
			wantIDs:  []game.ObjectID{scenario.ObjectBodyDoor},
		},
		{
			name:     "all",
			answer:   "both",
			decision: intent.ClarificationDecision{Kind: intent.ClarificationAll},
			wantIDs:  []game.ObjectID{scenario.ObjectBodyCabinet, scenario.ObjectBodyDoor},
		},
		{
			name:     "confirmation with candidate",
			answer:   "yes, the one near the door",
			decision: intent.ClarificationDecision{Kind: intent.ClarificationConfirm, Mention: "Doctor Near Door"},
			wantIDs:  []game.ObjectID{scenario.ObjectBodyDoor},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := litStorageWorld(t)
			parser := &scriptedSessionParser{
				plans:     map[string]intent.SemanticPlan{"search the doctor": searchDoctorPlan()},
				decisions: map[string]intent.ClarificationDecision{tt.answer: tt.decision},
			}
			s := New(state, parser, semanticSessionComposer{})

			first := mustSessionTurn(t, s, "search the doctor")
			if first.Pending == nil || first.Pending.ActionIndex != 0 || first.Pending.Role != grounding.RoleObject {
				t.Fatalf("first pending = %#v", first.Pending)
			}
			second := mustSessionTurn(t, s, tt.answer)

			if second.Pending != nil {
				t.Fatalf("second pending = %#v", second.Pending)
			}
			if len(second.Result.Outcomes) != len(tt.wantIDs) {
				t.Fatalf("outcomes = %#v, want %d", second.Result.Outcomes, len(tt.wantIDs))
			}
			for index, wantID := range tt.wantIDs {
				if gotID := second.Result.Outcomes[index].TargetObjectID; gotID != wantID {
					t.Fatalf("outcome %d target = %q, want %q", index, gotID, wantID)
				}
			}
			if len(parser.semanticMessages) != 1 || !reflect.DeepEqual(parser.clarificationMessages, []string{tt.answer}) {
				t.Fatalf("semantic=%#v clarification=%#v", parser.semanticMessages, parser.clarificationMessages)
			}
			views := parser.clarificationCandidates[0]
			if len(views) != 2 || views[0].Ordinal != 1 || views[1].Ordinal != 2 {
				t.Fatalf("candidate views = %#v", views)
			}
		})
	}
}

func TestSessionCancellationClearsPendingWithoutExecution(t *testing.T) {
	state := litStorageWorld(t)
	parser := &scriptedSessionParser{
		plans: map[string]intent.SemanticPlan{"search the doctor": searchDoctorPlan()},
		decisions: map[string]intent.ClarificationDecision{
			"no": {Kind: intent.ClarificationCancel},
		},
	}
	s := New(state, parser, semanticSessionComposer{})
	mustSessionTurn(t, s, "search the doctor")
	before := state.NowSeconds

	got := mustSessionTurn(t, s, "no")

	if got.Pending != nil || s.pending != nil {
		t.Fatalf("pending was not cleared: turn=%#v session=%#v", got.Pending, s.pending)
	}
	if len(got.Result.Outcomes) != 0 || state.NowSeconds != before {
		t.Fatalf("cancel executed work: result=%#v time=%d", got.Result, state.NowSeconds)
	}
	if !reflect.DeepEqual(parser.clarificationMessages, []string{"no"}) || len(parser.semanticMessages) != 1 {
		t.Fatalf("semantic=%#v clarification=%#v", parser.semanticMessages, parser.clarificationMessages)
	}
}

func TestSessionNewCommandCancelsPendingAndParsesNormally(t *testing.T) {
	state := litStorageWorld(t)
	parser := &scriptedSessionParser{
		plans: map[string]intent.SemanticPlan{
			"search the doctor": searchDoctorPlan(),
			"wait": {
				Actions: []intent.SemanticAction{intent.WaitAction{Evidence: "wait"}},
				RawText: "wait",
			},
		},
		decisions: map[string]intent.ClarificationDecision{
			"wait": {Kind: intent.ClarificationNewCommand},
		},
	}
	s := New(state, parser, semanticSessionComposer{})
	mustSessionTurn(t, s, "search the doctor")
	before := state.NowSeconds

	got := mustSessionTurn(t, s, "wait")

	if got.Pending != nil || s.pending != nil {
		t.Fatalf("pending was not cleared: turn=%#v session=%#v", got.Pending, s.pending)
	}
	if len(got.Result.Outcomes) != 1 || got.Result.Outcomes[0].Result.Outcome != "waited" {
		t.Fatalf("result = %#v", got.Result)
	}
	if state.NowSeconds-before != 10 {
		t.Fatalf("elapsed = %d, want new wait only", state.NowSeconds-before)
	}
	if !reflect.DeepEqual(parser.semanticMessages, []string{"search the doctor", "wait"}) || !reflect.DeepEqual(parser.clarificationMessages, []string{"wait"}) {
		t.Fatalf("semantic=%#v clarification=%#v", parser.semanticMessages, parser.clarificationMessages)
	}
}

func TestSessionRoutesYesNoAndBothThroughPendingClarification(t *testing.T) {
	tests := []struct {
		message  string
		decision intent.ClarificationDecision
	}{
		{message: "yes", decision: intent.ClarificationDecision{Kind: intent.ClarificationConfirm, Ordinal: 2}},
		{message: "no", decision: intent.ClarificationDecision{Kind: intent.ClarificationCancel}},
		{message: "both", decision: intent.ClarificationDecision{Kind: intent.ClarificationAll}},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			state := litStorageWorld(t)
			parser := &scriptedSessionParser{
				plans:     map[string]intent.SemanticPlan{"search the doctor": searchDoctorPlan()},
				decisions: map[string]intent.ClarificationDecision{tt.message: tt.decision},
			}
			s := New(state, parser, semanticSessionComposer{})
			mustSessionTurn(t, s, "search the doctor")

			mustSessionTurn(t, s, tt.message)

			if !reflect.DeepEqual(parser.clarificationMessages, []string{tt.message}) {
				t.Fatalf("clarification messages = %#v", parser.clarificationMessages)
			}
			if !reflect.DeepEqual(parser.semanticMessages, []string{"search the doctor"}) {
				t.Fatalf("semantic messages = %#v", parser.semanticMessages)
			}
		})
	}
}

func TestSessionResumesExactActionWithoutReplayingTimeOrEvents(t *testing.T) {
	state := litStorageWorld(t)
	state.ScheduledEvents = nil
	state.ScheduleEvent(5, game.WorldEvent{Type: game.EventSound, Description: "A timer chimes."})
	plan := intent.SemanticPlan{
		Actions: []intent.SemanticAction{
			intent.WaitAction{Evidence: "wait"},
			intent.SearchAction{Target: intent.Reference{Mention: "doctor", Quantity: intent.TargetOne}, Evidence: "search the doctor"},
		},
		RawText: "wait, then search the doctor",
	}
	parser := &scriptedSessionParser{
		plans: map[string]intent.SemanticPlan{"wait, then search the doctor": plan},
		decisions: map[string]intent.ClarificationDecision{
			"the second one": {Kind: intent.ClarificationSelect, Ordinal: 2},
		},
	}
	s := New(state, parser, semanticSessionComposer{})

	first := mustSessionTurn(t, s, "wait, then search the doctor")
	if first.Pending == nil || first.Pending.ActionIndex != 1 {
		t.Fatalf("first pending = %#v", first.Pending)
	}
	if first.DurationSeconds != 10 || len(first.Result.Outcomes) != 1 || len(first.Result.Outcomes[0].Result.Events) != 1 {
		t.Fatalf("first turn = %#v", first)
	}
	if s.pending == nil || s.pending.actionIndex != 1 || s.pending.role != grounding.RoleObject {
		t.Fatalf("session pending = %#v", s.pending)
	}
	if len(s.pending.plan.Actions) != 2 || !reflect.DeepEqual(s.pending.candidateIDs, []string{string(scenario.ObjectBodyCabinet), string(scenario.ObjectBodyDoor)}) {
		t.Fatalf("stored pending = %#v", s.pending)
	}

	second := mustSessionTurn(t, s, "the second one")

	if second.DurationSeconds != 30 || len(second.Result.Outcomes) != 1 {
		t.Fatalf("second turn = %#v", second)
	}
	if second.Result.Outcomes[0].TargetObjectID != scenario.ObjectBodyDoor {
		t.Fatalf("target = %q", second.Result.Outcomes[0].TargetObjectID)
	}
	if len(second.Result.Outcomes[0].Result.Events) != 0 {
		t.Fatalf("event replayed: %#v", second.Result.Outcomes[0].Result.Events)
	}
	if state.NowSeconds != 40 || len(state.ScheduledEvents) != 0 {
		t.Fatalf("time=%d scheduled=%#v", state.NowSeconds, state.ScheduledEvents)
	}
}

func TestCloneSemanticPlanDeepClonesEveryConcreteAction(t *testing.T) {
	actions := []struct {
		name   string
		action intent.SemanticAction
	}{
		{name: "move", action: &intent.MoveAction{Direction: "north", Evidence: "go north"}},
		{name: "inspect", action: &intent.InspectAction{Target: intent.Reference{Mention: "desk", Quantity: intent.TargetOne}, Evidence: "inspect desk"}},
		{name: "search", action: &intent.SearchAction{Target: intent.Reference{Mention: "desk", Quantity: intent.TargetOne}, Evidence: "search desk"}},
		{name: "take", action: &intent.TakeAction{Target: intent.Reference{Mention: "key", Quantity: intent.TargetOne}, Evidence: "take key"}},
		{name: "use", action: &intent.UseAction{Item: intent.Reference{Mention: "key", Quantity: intent.TargetOne}, Target: intent.Reference{Mention: "door", Quantity: intent.TargetOne}, Evidence: "use key"}},
		{name: "toggle", action: &intent.ToggleAction{Item: intent.Reference{Mention: "light", Quantity: intent.TargetOne}, State: "on", Evidence: "turn on light"}},
		{name: "wait", action: &intent.WaitAction{Evidence: "wait"}},
		{name: "talk", action: &intent.TalkAction{Target: intent.Reference{Mention: "doctor", Quantity: intent.TargetOne}, Evidence: "talk"}},
		{name: "listen", action: &intent.ListenAction{Target: intent.Reference{Mention: "door", Quantity: intent.TargetOne}, Evidence: "listen"}},
		{name: "explore", action: &intent.ExploreAction{Target: intent.Reference{Mention: "room", Quantity: intent.TargetOne}, Evidence: "explore"}},
	}
	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			plan := intent.SemanticPlan{
				Actions:   []intent.SemanticAction{tt.action},
				Questions: []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "doctor", TargetMode: intent.TargetOne}},
			}
			originalBefore := semanticActionValue(tt.action)
			cloned := cloneSemanticPlan(plan)
			if reflect.ValueOf(cloned.Actions[0]).Pointer() == reflect.ValueOf(tt.action).Pointer() {
				t.Fatal("cloned action retained the original pointer")
			}

			mutateSemanticAction(t, cloned.Actions[0], "clone")
			if !reflect.DeepEqual(semanticActionValue(tt.action), originalBefore) {
				t.Fatalf("mutating clone changed original: got %#v want %#v", semanticActionValue(tt.action), originalBefore)
			}

			secondClone := cloneSemanticPlan(plan)
			secondCloneBefore := semanticActionValue(secondClone.Actions[0])
			mutateSemanticAction(t, tt.action, "original")
			if !reflect.DeepEqual(semanticActionValue(secondClone.Actions[0]), secondCloneBefore) {
				t.Fatalf("mutating original changed clone: got %#v want %#v", semanticActionValue(secondClone.Actions[0]), secondCloneBefore)
			}

			cloned.Questions[0].Target = "changed"
			if plan.Questions[0].Target != "doctor" {
				t.Fatalf("mutating cloned questions changed original: %#v", plan.Questions)
			}
		})
	}
}

func TestSessionPendingPlanViewsCannotMutateChainedResumeState(t *testing.T) {
	state, keyID, doorID := ambiguousSessionUseWorld(t)
	use := &intent.UseAction{
		Item:     intent.Reference{Mention: "key", Quantity: intent.TargetOne},
		Target:   intent.Reference{Mention: "door", Quantity: intent.TargetOne},
		Evidence: "use the key on the door",
	}
	plan := intent.SemanticPlan{
		Actions: []intent.SemanticAction{intent.WaitAction{Evidence: "wait"}, use},
		RawText: "wait, then use the key on the door",
	}
	parser := &scriptedSessionParser{
		plans: map[string]intent.SemanticPlan{plan.RawText: plan},
		decisions: map[string]intent.ClarificationDecision{
			"first key":   {Kind: intent.ClarificationSelect, Ordinal: 1},
			"second door": {Kind: intent.ClarificationSelect, Ordinal: 2},
		},
	}
	s := New(state, parser, semanticSessionComposer{})

	first := mustSessionTurn(t, s, plan.RawText)
	if first.Pending == nil || first.Pending.ActionIndex != 1 || first.Pending.Role != grounding.RoleItem || state.NowSeconds != 10 {
		t.Fatalf("first turn pending=%#v time=%d", first.Pending, state.NowSeconds)
	}
	use.Target.Mention = "source mutation"
	first.SemanticPlan.Actions[1].(*intent.UseAction).Target.Mention = "processed mutation"
	first.Pending.RemainingPlan.Actions[0].(*intent.UseAction).Target.Mention = "pending mutation"

	second := mustSessionTurn(t, s, "first key")
	if second.Pending == nil || second.Pending.ActionIndex != 1 || second.Pending.Role != grounding.RoleDoor {
		t.Fatalf("second pending = %#v, want chained door ambiguity", second.Pending)
	}
	if state.NowSeconds != 10 || len(second.Result.Outcomes) != 0 {
		t.Fatalf("second turn replayed work: time=%d outcomes=%#v", state.NowSeconds, second.Result.Outcomes)
	}

	third := mustSessionTurn(t, s, "second door")
	if third.Pending != nil || len(third.Result.Outcomes) != 1 || third.Result.Outcomes[0].Result.Status != game.ActionSucceeded {
		t.Fatalf("third turn = %#v", third)
	}
	if !state.HasItem(keyID) || state.Doors[doorID].State != world.DoorClosed {
		t.Fatalf("selected identities changed: inventory=%#v door=%#v", state.Inventory, state.Doors[doorID])
	}
	if state.NowSeconds != 10+third.DurationSeconds {
		t.Fatalf("time=%d duration=%d, want wait plus one resumed action", state.NowSeconds, third.DurationSeconds)
	}
}

func TestSessionRetainsPendingAfterClarificationErrorAndConflictingSelection(t *testing.T) {
	state := litStorageWorld(t)
	parseErr := errors.New("clarification unavailable")
	parser := &scriptedSessionParser{
		plans: map[string]intent.SemanticPlan{"search the doctor": searchDoctorPlan()},
		decisions: map[string]intent.ClarificationDecision{
			"conflict": {Kind: intent.ClarificationSelect, Mention: "Doctor Near Cabinet", Ordinal: 2},
			"first":    {Kind: intent.ClarificationSelect, Ordinal: 1},
		},
		clarificationErrors: map[string]error{"error": parseErr},
	}
	s := New(state, parser, semanticSessionComposer{})
	first := mustSessionTurn(t, s, "search the doctor")
	wantIDs := pendingIDs(first.Pending)

	if _, err := s.ProcessTurn(context.Background(), "error"); !errors.Is(err, parseErr) {
		t.Fatalf("parse error = %v, want %v", err, parseErr)
	}
	if s.pending == nil || !reflect.DeepEqual(s.pending.candidateIDs, wantIDs) || state.NowSeconds != 0 {
		t.Fatalf("pending changed after parse error: pending=%#v time=%d", s.pending, state.NowSeconds)
	}

	conflict := mustSessionTurn(t, s, "conflict")
	if conflict.Pending == nil || conflict.Result.StopReason != "clarification" || state.NowSeconds != 0 {
		t.Fatalf("conflict turn = %#v time=%d", conflict, state.NowSeconds)
	}
	if s.pending == nil || !reflect.DeepEqual(s.pending.candidateIDs, wantIDs) {
		t.Fatalf("pending changed after conflict: %#v", s.pending)
	}

	resolved := mustSessionTurn(t, s, "first")
	if resolved.Pending != nil || len(resolved.Result.Outcomes) != 1 || resolved.Result.Outcomes[0].TargetObjectID != scenario.ObjectBodyCabinet {
		t.Fatalf("resolved turn = %#v", resolved)
	}
}

func TestSessionRejectsStaleSingleAndAllBindingsWithoutFallback(t *testing.T) {
	tests := []struct {
		name     string
		decision intent.ClarificationDecision
	}{
		{name: "single", decision: intent.ClarificationDecision{Kind: intent.ClarificationSelect, Ordinal: 1}},
		{name: "all", decision: intent.ClarificationDecision{Kind: intent.ClarificationAll}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := litStorageWorld(t)
			parser := &scriptedSessionParser{
				plans:     map[string]intent.SemanticPlan{"search the doctor": searchDoctorPlan()},
				decisions: map[string]intent.ClarificationDecision{"choose": tt.decision},
			}
			s := New(state, parser, semanticSessionComposer{})
			first := mustSessionTurn(t, s, "search the doctor")
			staleID := game.ObjectID(first.Pending.Candidates[0].ID)
			removeRoomObject(state, staleID)

			got := mustSessionTurn(t, s, "choose")

			if got.Pending != nil || len(got.Result.Outcomes) != 1 {
				t.Fatalf("turn = %#v", got)
			}
			outcome := got.Result.Outcomes[0]
			if outcome.Result.Status != game.ActionFailed || outcome.Result.Outcome != string(grounding.MissingReasonStaleBinding) {
				t.Fatalf("outcome = %#v", outcome)
			}
			if outcome.TargetObjectID != "" || state.NowSeconds != 0 {
				t.Fatalf("stale binding fell back or executed: target=%q time=%d", outcome.TargetObjectID, state.NowSeconds)
			}
		})
	}
}

func mutateSemanticAction(t *testing.T, action intent.SemanticAction, value string) {
	t.Helper()
	reference := intent.Reference{Mention: value, Quantity: intent.TargetAll}
	switch typed := action.(type) {
	case *intent.MoveAction:
		typed.Direction, typed.Evidence = value, value
	case *intent.InspectAction:
		typed.Target, typed.Evidence = reference, value
	case *intent.SearchAction:
		typed.Target, typed.Evidence = reference, value
	case *intent.TakeAction:
		typed.Target, typed.Evidence = reference, value
	case *intent.UseAction:
		typed.Item, typed.Target, typed.Evidence = reference, reference, value
	case *intent.ToggleAction:
		typed.Item, typed.State, typed.Evidence = reference, "off", value
	case *intent.WaitAction:
		typed.Evidence = value
	case *intent.TalkAction:
		typed.Target, typed.Evidence = reference, value
	case *intent.ListenAction:
		typed.Target, typed.Evidence = reference, value
	case *intent.ExploreAction:
		typed.Target, typed.Evidence = reference, value
	default:
		t.Fatalf("unsupported action type %T", action)
	}
}

func semanticActionValue(action intent.SemanticAction) any {
	value := reflect.ValueOf(action)
	if value.Kind() == reflect.Pointer {
		return value.Elem().Interface()
	}
	return action
}

func pendingIDs(pending *turn.PendingSemanticAction) []string {
	if pending == nil {
		return nil
	}
	ids := make([]string, len(pending.Candidates))
	for index, candidate := range pending.Candidates {
		ids[index] = candidate.ID
	}
	return ids
}

func removeRoomObject(state *world.State, objectID game.ObjectID) {
	room := state.Rooms[state.CurrentRoomID]
	kept := room.Objects[:0]
	for _, current := range room.Objects {
		if current != objectID {
			kept = append(kept, current)
		}
	}
	room.Objects = kept
	state.Rooms[room.ID] = room
}

func ambiguousSessionUseWorld(t *testing.T) (*world.State, game.ItemID, game.DoorID) {
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

func mustSessionTurn(t *testing.T, session *Session, message string) ProcessedTurn {
	t.Helper()
	got, err := session.ProcessTurn(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func searchDoctorPlan() intent.SemanticPlan {
	return intent.SemanticPlan{
		Actions: []intent.SemanticAction{intent.SearchAction{
			Target:   intent.Reference{Mention: "doctor", Quantity: intent.TargetOne},
			Evidence: "search the doctor",
		}},
		RawText: "search the doctor",
	}
}

func litStorageWorld(t *testing.T) *world.State {
	t.Helper()
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, ""); err != nil {
		t.Fatal(err)
	}
	return state
}
