package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/response"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/turn"
)

type fakePlaytestGenerator struct {
	responses []string
}

func TestProcessPlayerTurnUsesPlanExecutorAndComposer(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	parser := fakeTurnParser{plan: intent.TurnPlan{Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionInspect}, TargetMode: intent.TargetSingle}}}}
	composer := fakeComposer{text: "The reception is damaged. I can go east."}
	got, err := processPlayerTurn(context.Background(), "what is around you", state, parser, turn.NewExecutor(state), composer)
	if err != nil {
		t.Fatal(err)
	}
	if got.Response.Text != composer.text || len(got.Result.Outcomes) != 1 {
		t.Fatalf("turn = %#v", got)
	}
}

type fakeTurnParser struct {
	plan intent.TurnPlan
	err  error
}

func (f fakeTurnParser) Parse(context.Context, string, game.PerceptionSnapshot) (intent.TurnPlan, error) {
	return f.plan, f.err
}

type fakeComposer struct{ text string }

func (f fakeComposer) Compose(context.Context, turn.FactBundle) response.Response {
	return response.Response{Text: f.text}
}

var _ turnParser = fakeTurnParser{}
var _ responseComposer = fakeComposer{}

func TestParsePlayOptionsUsesExplicitSeed(t *testing.T) {
	options, err := parsePlayOptions([]string{"--seed", "-42"}, func() (int64, error) { return 99, nil })

	if err != nil {
		t.Fatal(err)
	}
	if options.Seed != -42 {
		t.Fatalf("seed = %d, want -42", options.Seed)
	}
}

func TestParsePlayOptionsGeneratesMissingSeed(t *testing.T) {
	options, err := parsePlayOptions(nil, func() (int64, error) { return 99, nil })

	if err != nil {
		t.Fatal(err)
	}
	if options.Seed != 99 {
		t.Fatalf("seed = %d, want 99", options.Seed)
	}
}

func TestParsePlayOptionsRejectsPositionals(t *testing.T) {
	_, err := parsePlayOptions([]string{"extra"}, func() (int64, error) { return 1, nil })

	if err == nil {
		t.Fatal("expected positional argument error")
	}
}

func TestReadRunSeedRetriesZeroAndClearsSignBit(t *testing.T) {
	var data [16]byte
	binary.LittleEndian.PutUint64(data[8:], uint64(1)<<63|42)

	seed, err := readRunSeed(bytes.NewReader(data[:]))

	if err != nil {
		t.Fatal(err)
	}
	if seed != 42 {
		t.Fatalf("seed = %d, want 42", seed)
	}
}

func TestPrintRunDebugIncludesReproductionData(t *testing.T) {
	run := mustGenerateTestRun(t, 12345)
	var output strings.Builder

	printRunDebug(&output, run)

	for _, expected := range []string{
		"Run seed: 12345",
		"Generator: 1",
		"Flashlight:",
		"Brass Key:",
		"Validation: playable",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output %q missing %q", output.String(), expected)
		}
	}
}

func mustGenerateTestRun(t *testing.T, seed int64) rungen.GeneratedRun {
	t.Helper()
	run, err := rungen.Generate(
		rungen.RunConfig{Seed: seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
		runscenario.PrototypeDefinition(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (f *fakePlaytestGenerator) GenerateJSON(ctx context.Context, systemPrompt string, userPrompt string, schema any) (string, error) {
	if len(f.responses) == 0 {
		return "", errors.New("missing fake response")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func TestRunPlaytestScriptMarksExpectedActionMismatchSuspicious(t *testing.T) {
	parser := intent.NewParser(&fakePlaytestGenerator{responses: []string{`{
		"actions": [{"intent": {"action": "inspect", "target": "", "item": "", "direction": "", "modifiers": [], "confidence": 0.9, "rawText": "what's in your bag", "needsClarification": false, "clarificationQuestion": ""}, "targetMode": "single"}],
		"questions": [], "confidence": 0.9, "needsClarification": false, "clarificationQuestion": "", "rawText": "what's in your bag"
	}`}})
	script := playtestScript{
		Name: "intent collision",
		Steps: []playtestMessage{
			{
				Player: "what's in your bag",
				Expect: playtestExpectation{
					Action: intent.ActionTalk,
				},
			},
		},
	}

	got, err := runPlaytestScript(parser, script)
	if err != nil {
		t.Fatalf("runPlaytestScript returned error: %v", err)
	}

	if len(got.Suspicious) != 2 {
		t.Fatalf("Suspicious len = %d, want 2: %+v", len(got.Suspicious), got.Suspicious)
	}
	foundActionMismatch := false
	for _, suspicion := range got.Suspicious {
		if strings.Contains(suspicion, "expected action talk, got inspect") {
			foundActionMismatch = true
			break
		}
	}
	if !foundActionMismatch {
		t.Fatalf("Suspicion = %#v", got.Suspicious)
	}
}

func TestRunPlaytestScriptObservesInitialRoomAfterSetup(t *testing.T) {
	parser := scriptedParser{plans: map[string]intent.TurnPlan{
		"go north": {Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionMove, Direction: "north"}, TargetMode: intent.TargetSingle}}},
	}}
	script := playtestScript{Name: "storage start", InitialRoom: scenario.RoomStorage, InitialLight: true, Steps: []playtestMessage{{Player: "go north"}}}
	got, err := runPlaytestScript(parser, script)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 1 || len(got.Steps[0].Result.Outcomes) != 1 || got.Steps[0].Result.Outcomes[0].Result.Outcome != "door_blocked" {
		t.Fatalf("step = %#v, want observed north exit and blocked door", got.Steps)
	}
}

func TestUserRegressionPlaytestTracksDarknessAndPluralFacts(t *testing.T) {
	plans := map[string]intent.TurnPlan{
		"what is around you":      roomInspectPlan(),
		"what is on the desk":     objectInspectPlan("desk"),
		"look inside the drawers": searchPlan("drawers"),
		"take the flashlight":     itemPlan(intent.ActionTakeItem, "flashlight"),
		"go east":                 intent.TurnPlan{Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionMove, Direction: "east"}, TargetMode: intent.TargetSingle}}},
		"whats around you":        roomInspectPlan(),
		"turn on the flashlight":  itemPlan(intent.ActionTurnOn, "flashlight"),
		"look around":             roomInspectPlan(),
		"search the doctors are they dead": {
			Actions:   []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionSearch, Target: "doctors"}, TargetMode: intent.TargetAll}},
			Questions: []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "they", TargetMode: intent.TargetAll}},
		},
	}
	parser := scriptedParser{plans: plans}
	got, err := runPlaytestScript(parser, playtestScript{Name: "regression", Steps: defaultPlaytestScripts()[0].Steps}, fakeComposer{text: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 9 || len(got.Suspicious) != 0 {
		t.Fatalf("steps=%d suspicious=%v", len(got.Steps), got.Suspicious)
	}
	darkFacts := got.Steps[5].Result.FactBundle(got.Steps[5].Player).Facts
	if !containsFactText(darkFacts, "west") || containsFactText(darkFacts, "north") {
		t.Fatalf("dark facts = %#v", darkFacts)
	}
	litFacts := got.Steps[7].Result.FactBundle(got.Steps[7].Player).Facts
	if !containsFactText(litFacts, "north") {
		t.Fatalf("lit facts = %#v", litFacts)
	}
	final := got.Steps[8].Result
	if len(final.Outcomes) != 2 || len(final.QuestionFacts) != 2 {
		t.Fatalf("final result = %#v", final)
	}
}

type scriptedParser struct{ plans map[string]intent.TurnPlan }

func (p scriptedParser) Parse(_ context.Context, message string, _ game.PerceptionSnapshot) (intent.TurnPlan, error) {
	plan := p.plans[message]
	plan.Confidence = 1
	for i := range plan.Actions {
		plan.Actions[i].Intent.Confidence = 1
	}
	return plan, nil
}

func roomInspectPlan() intent.TurnPlan {
	return intent.TurnPlan{Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionInspect}, TargetMode: intent.TargetSingle}}}
}

func objectInspectPlan(target string) intent.TurnPlan {
	return intent.TurnPlan{Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionInspect, Target: target}, TargetMode: intent.TargetSingle}}}
}

func searchPlan(target string) intent.TurnPlan {
	return intent.TurnPlan{Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionSearch, Target: target}, TargetMode: intent.TargetSingle}}}
}

func itemPlan(action intent.Action, item string) intent.TurnPlan {
	return intent.TurnPlan{Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: action, Item: item}, TargetMode: intent.TargetSingle}}}
}

func containsFactText(facts []game.Fact, text string) bool {
	for _, fact := range facts {
		if strings.Contains(strings.ToLower(fact.Text), strings.ToLower(text)) {
			return true
		}
	}
	return false
}
