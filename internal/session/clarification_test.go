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
