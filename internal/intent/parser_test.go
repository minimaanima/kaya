package intent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"kaya/internal/game"
)

type fakeGenerator struct {
	responses []string
	err       error
	calls     int
}

func (f *fakeGenerator) Generate(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	if len(f.responses) == 0 {
		return "", errors.New("missing fake response")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func (f *fakeGenerator) GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, schema any) (string, error) {
	return f.Generate(ctx, systemPrompt, userPrompt)
}

func TestParserParsesPluralCompoundTurn(t *testing.T) {
	generator := &fakeGenerator{responses: []string{`{
		"actions":[{"intent":{"action":"search","target":"doctors","item":"","direction":"","modifiers":[],"confidence":0.96,"rawText":"search the doctors","needsClarification":false,"clarificationQuestion":""},"targetMode":"all"}],
		"questions":[{"kind":"life_status","target":"they","targetMode":"all"}],
		"confidence":0.96,"needsClarification":false,"clarificationQuestion":"","rawText":"search the doctors are they dead"
	}`}}
	parser := NewParser(generator)
	plan, err := parser.Parse(context.Background(), "search the doctors are they dead", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].TargetMode != TargetAll || len(plan.Questions) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestParserUsesDeterministicPlanForDirectCommands(t *testing.T) {
	badModelPlan := `{"actions":[{"intent":{"action":"explore","target":"desk","item":"","direction":"","modifiers":[],"confidence":0.95,"rawText":"wrong","needsClarification":false,"clarificationQuestion":""},"targetMode":"all"}],"questions":[],"confidence":0.95,"needsClarification":false,"clarificationQuestion":"","rawText":"wrong"}`
	tests := []struct {
		message string
		action  Action
		target  string
	}{
		{message: "search the desk", action: ActionSearch, target: "desk"},
		{message: "whats on the reception floor", action: ActionInspect, target: "reception floor"},
		{message: "search the Reception Desk", action: ActionSearch, target: "reception desk"},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			generator := &fakeGenerator{responses: []string{badModelPlan}}
			plan, err := NewParser(generator).Parse(context.Background(), tt.message, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if generator.calls != 0 {
				t.Fatalf("model calls = %d, want direct command bypass", generator.calls)
			}
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != tt.action {
				t.Fatalf("plan = %#v", plan)
			}
			if got := plan.Actions[0].Intent.Target; got != tt.target {
				t.Fatalf("target = %q, want %q", got, tt.target)
			}
		})
	}
}

func TestFallbackPlanExploresWalls(t *testing.T) {
	plan := FallbackPlan("feel along the walls for another exit")
	if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != ActionExplore {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestFallbackPlanExtractsObjectTargets(t *testing.T) {
	tests := []struct {
		message string
		action  Action
		target  string
	}{
		{message: "search the desk", action: ActionSearch, target: "desk"},
		{message: "look through the drawers", action: ActionSearch, target: "drawers"},
		{message: "look inside the drawers", action: ActionSearch, target: "drawers"},
		{message: "search for the desk", action: ActionSearch, target: "desk"},
		{message: "grab the flashlight", action: ActionTakeItem, target: "flashlight"},
		{message: "take the flashlight", action: ActionTakeItem, target: "flashlight"},
		{message: "what is on the desk", action: ActionInspect, target: "desk"},
		{message: "inspect the cabinet", action: ActionInspect, target: "cabinet"},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			plan := FallbackPlan(tt.message)
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != tt.action {
				t.Fatalf("plan = %#v", plan)
			}
			if got := plan.Actions[0].Intent.Target; got != tt.target {
				t.Fatalf("Target = %q, want %q", got, tt.target)
			}
		})
	}
}

func TestFallbackPlanRoutesInventoryQuestionsToTalk(t *testing.T) {
	for _, message := range []string{"what is in your bag", "what's in your inventory", "do you have anything"} {
		t.Run(message, func(t *testing.T) {
			plan := FallbackPlan(message)
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != ActionTalk {
				t.Fatalf("plan = %#v, want talk", plan)
			}
			if plan.Actions[0].Intent.Target != "inventory" {
				t.Fatalf("Target = %q, want inventory", plan.Actions[0].Intent.Target)
			}
		})
	}
}

func TestParserNormalizesApprovedContextualPhrases(t *testing.T) {
	valid := func(raw string) string {
		return `{"actions":[{"intent":{"action":"explore","target":"","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"` + raw + `","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"` + raw + `"}`
	}
	tests := []struct {
		name string
		msg  string
		want Action
	}{
		{name: "singular doctor", msg: "inspect the doctor", want: ActionInspect},
		{name: "both doctors", msg: "inspect both doctors", want: ActionInspect},
		{name: "them", msg: "search them", want: ActionSearch},
		{name: "compound", msg: "search the doctors are they dead", want: ActionSearch},
		{name: "walls", msg: "feel along the walls for another exit", want: ActionExplore},
		{name: "cabinet typo", msg: "what is isnide the storage cabiner", want: ActionInspect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := NewParser(&fakeGenerator{responses: []string{valid(tt.msg)}}).Parse(context.Background(), tt.msg, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != tt.want {
				t.Fatalf("plan = %#v", plan)
			}
		})
	}
}

func TestParserNormalizesPluralDoctorTargetAsAll(t *testing.T) {
	raw := `{"actions":[{"intent":{"action":"inspect","target":"doctors","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"inspect the doctors","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"inspect the doctors"}`
	plan, err := NewParser(&fakeGenerator{responses: []string{raw}}).Parse(context.Background(), "inspect the doctors", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != ActionInspect || plan.Actions[0].TargetMode != TargetAll {
		t.Fatalf("plan = %#v, want plural inspect with targetMode all", plan)
	}
}

func TestParserPreservesRepeatedGenericActions(t *testing.T) {
	raw := `{"actions":[{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"wait twice","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"wait twice","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"wait twice"}`
	plan, err := NewParser(&fakeGenerator{responses: []string{raw}}).Parse(context.Background(), "wait twice", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 || plan.Actions[0].Intent.Action != ActionWait || plan.Actions[1].Intent.Action != ActionWait {
		t.Fatalf("plan = %#v, want two ordered wait actions", plan)
	}
}

func TestParserNormalizesUnsupportedQuestionToClarification(t *testing.T) {
	raw := `{"actions":[{"intent":{"action":"search","target":"room","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"do they have anything","needsClarification":false,"clarificationQuestion":""},"targetMode":"all"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"do they have anything"}`
	plan, err := NewParser(&fakeGenerator{responses: []string{raw}}).Parse(context.Background(), "do they have anything", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 || !plan.NeedsClarification || plan.ClarificationQuestion == "" {
		t.Fatalf("plan = %#v, want clarification without action", plan)
	}
}

func TestParseTurnPlanRejectsMoreThanFourActions(t *testing.T) {
	_, err := ParseTurnPlanJSON(fiveActionPlanJSON)
	if !errors.Is(err, ErrPlanTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestParseTurnPlanRejectsNullSchemaFields(t *testing.T) {
	base := `{"actions":[],"questions":[],"confidence":1,"needsClarification":false,"clarificationQuestion":"","rawText":"wait"}`
	for _, field := range []string{"actions", "questions", "confidence", "needsClarification", "clarificationQuestion", "rawText"} {
		t.Run(field, func(t *testing.T) {
			raw := strings.Replace(base, `"`+field+`":`+fieldValue(base, field), `"`+field+`":null`, 1)
			if _, err := ParseTurnPlanJSON(raw); err == nil {
				t.Fatalf("expected null %s to fail", field)
			}
		})
	}
}

func TestParseTurnPlanRejectsNullEmbeddedModifiers(t *testing.T) {
	raw := `{"actions":[{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":null,"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":1,"needsClarification":false,"clarificationQuestion":"","rawText":"wait"}`
	if _, err := ParseTurnPlanJSON(raw); err == nil {
		t.Fatal("expected null modifiers to fail")
	}
}

func fieldValue(raw, field string) string {
	marker := `"` + field + `":`
	start := strings.Index(raw, marker) + len(marker)
	rest := raw[start:]
	if strings.HasPrefix(rest, `"`) {
		end := strings.Index(rest[1:], `"`) + 2
		return rest[:end]
	}
	for i, r := range rest {
		if r == ',' || r == '}' {
			return rest[:i]
		}
	}
	return rest
}

const fiveActionPlanJSON = `{
	"actions":[
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}
	],"questions":[],"confidence":1,"needsClarification":false,"clarificationQuestion":"","rawText":"wait five times"
}`

func TestParserParseValidIntent(t *testing.T) {
	generator := &fakeGenerator{responses: []string{`{"actions":[{"intent":{"action":"search","target":"dead doctor coat pockets","item":"flashlight","direction":"","modifiers":["carefully","keep_light_low"],"confidence":0.93,"rawText":"check the pockets","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.93,"needsClarification":false,"clarificationQuestion":"","rawText":"check the pockets"}`}}

	parser := NewParser(generator)
	got, err := parser.Parse(context.Background(), "check the pockets", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if got.Actions[0].Intent.Action != ActionSearch {
		t.Fatalf("Action = %q, want %q", got.Actions[0].Intent.Action, ActionSearch)
	}
	if got.Actions[0].Intent.Target != "dead doctor coat pockets" {
		t.Fatalf("Target = %q, want dead doctor coat pockets", got.Actions[0].Intent.Target)
	}
	if generator.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", generator.calls)
	}
}

func TestParserRepairsInvalidJSON(t *testing.T) {
	generator := &fakeGenerator{responses: []string{
		`not json`,
		`{"actions":[{"intent":{"action":"unknown","target":"","item":"","direction":"","modifiers":[],"confidence":0.18,"rawText":"Do it.","needsClarification":true,"clarificationQuestion":"What do you want Kaya to do?"},"targetMode":"single"}],"questions":[],"confidence":0.18,"needsClarification":true,"clarificationQuestion":"What do you want Kaya to do?","rawText":"Do it."}`,
	}}

	parser := NewParser(generator)
	got, err := parser.Parse(context.Background(), "Do it.", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if len(got.Actions) != 0 {
		t.Fatalf("Actions = %#v, want no executable actions", got.Actions)
	}
	if !got.NeedsClarification {
		t.Fatal("NeedsClarification = false, want true")
	}
	if generator.calls != 2 {
		t.Fatalf("generator calls = %d, want 2", generator.calls)
	}
}

func TestParseJSONRejectsInvalidAction(t *testing.T) {
	_, err := ParseJSON(`{
		"action": "teleport",
		"target": "door",
		"item": "",
		"direction": "",
		"modifiers": [],
		"confidence": 0.8,
		"rawText": "teleport",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err == nil {
		t.Fatal("ParseJSON returned nil error for invalid action")
	}
}

func TestParseJSONNormalizesMoveDirectionFromTarget(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "move",
		"target": "left",
		"item": "",
		"direction": "",
		"modifiers": ["quietly"],
		"confidence": 0.95,
		"rawText": "Maybe go left, but quietly.",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Direction != "left" {
		t.Fatalf("Direction = %q, want left", got.Direction)
	}
	if got.Target != "" {
		t.Fatalf("Target = %q, want empty", got.Target)
	}
}

func TestParseJSONNormalizesGeneralRoomAwareness(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "search",
		"target": "room",
		"item": "",
		"direction": "",
		"modifiers": [],
		"confidence": 0.95,
		"rawText": "What's in the room?",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionInspect {
		t.Fatalf("Action = %q, want %q", got.Action, ActionInspect)
	}
	if got.Target != "" {
		t.Fatalf("Target = %q, want empty", got.Target)
	}
}

func TestParseJSONNormalizesAroundYouAwareness(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "inspect",
		"target": "around you",
		"item": "",
		"direction": "",
		"modifiers": [],
		"confidence": 0.95,
		"rawText": "is there anything around you",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionInspect {
		t.Fatalf("Action = %q, want %q", got.Action, ActionInspect)
	}
	if got.Target != "" {
		t.Fatalf("Target = %q, want empty", got.Target)
	}
}

func TestParseJSONNormalizesVagueFollowUp(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "move",
		"target": "empty string",
		"item": "empty string",
		"direction": "empty string",
		"modifiers": [],
		"confidence": 1,
		"rawText": "Do it.",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionUnknown {
		t.Fatalf("Action = %q, want %q", got.Action, ActionUnknown)
	}
	if !got.NeedsClarification {
		t.Fatal("NeedsClarification = false, want true")
	}
	if got.Confidence > 0.25 {
		t.Fatalf("Confidence = %.2f, want <= 0.25", got.Confidence)
	}
}

func TestParseJSONNormalizesInventoryQuestion(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "unknown",
		"target": "",
		"item": "flashlight",
		"direction": "",
		"modifiers": [],
		"confidence": 0.8,
		"rawText": "do ypou have flashlight",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionTalk {
		t.Fatalf("Action = %q, want %q", got.Action, ActionTalk)
	}
	if got.Item != "flashlight" {
		t.Fatalf("Item = %q, want flashlight", got.Item)
	}
}

func TestParseJSONNormalizesKeyUse(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "force_open",
		"target": "emergency stairwell door",
		"item": "key",
		"direction": "",
		"modifiers": [],
		"confidence": 1,
		"rawText": "Try the key on the emergency stairwell door.",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionUseItem {
		t.Fatalf("Action = %q, want %q", got.Action, ActionUseItem)
	}
}

func TestParseJSONRestoresExplicitFlashlightItem(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "search",
		"target": "dead doctor's coat pockets",
		"item": "",
		"direction": "",
		"modifiers": ["keep_light_low"],
		"confidence": 0.95,
		"rawText": "Can you check the dead doctor's coat pockets but keep the flashlight low?",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Item != "flashlight" {
		t.Fatalf("Item = %q, want flashlight", got.Item)
	}
}

func TestParseJSONMergesNonMovementDirectionIntoSearchTarget(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "search",
		"target": "the doctor",
		"item": "",
		"direction": "near cabinet",
		"modifiers": [],
		"confidence": 0.95,
		"rawText": "search the doctor near cabinet",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Target != "the doctor near cabinet" {
		t.Fatalf("Target = %q, want the doctor near cabinet", got.Target)
	}
	if got.Direction != "" {
		t.Fatalf("Direction = %q, want empty", got.Direction)
	}
}
