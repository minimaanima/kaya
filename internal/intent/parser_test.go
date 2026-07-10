package intent

import (
	"context"
	"errors"
	"testing"
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

func TestParserParseValidIntent(t *testing.T) {
	generator := &fakeGenerator{responses: []string{`{
		"action": "search",
		"target": "dead doctor coat pockets",
		"item": "flashlight",
		"direction": "",
		"modifiers": ["carefully", "keep_light_low"],
		"confidence": 0.93,
		"rawText": "check the pockets",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`}}

	parser := NewParser(generator)
	got, err := parser.Parse(context.Background(), "check the pockets")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if got.Action != ActionSearch {
		t.Fatalf("Action = %q, want %q", got.Action, ActionSearch)
	}
	if got.Target != "dead doctor coat pockets" {
		t.Fatalf("Target = %q, want dead doctor coat pockets", got.Target)
	}
	if generator.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", generator.calls)
	}
}

func TestParserRepairsInvalidJSON(t *testing.T) {
	generator := &fakeGenerator{responses: []string{
		`not json`,
		`{
			"action": "unknown",
			"target": "",
			"item": "",
			"direction": "",
			"modifiers": [],
			"confidence": 0.18,
			"rawText": "Do it.",
			"needsClarification": true,
			"clarificationQuestion": "What do you want Kaya to do?"
		}`,
	}}

	parser := NewParser(generator)
	got, err := parser.Parse(context.Background(), "Do it.")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if got.Action != ActionUnknown {
		t.Fatalf("Action = %q, want %q", got.Action, ActionUnknown)
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
