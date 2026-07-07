package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"kaya/internal/intent"
)

type fakePlaytestGenerator struct {
	responses []string
}

func (f *fakePlaytestGenerator) Generate(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	if len(f.responses) == 0 {
		return "", errors.New("missing fake response")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func TestRunPlaytestScriptMarksExpectedActionMismatchSuspicious(t *testing.T) {
	parser := intent.NewParser(&fakePlaytestGenerator{responses: []string{`{
		"action": "inspect",
		"target": "",
		"item": "",
		"direction": "",
		"modifiers": [],
		"confidence": 0.9,
		"rawText": "what's in your bag",
		"needsClarification": false,
		"clarificationQuestion": ""
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

	if len(got.Suspicious) != 1 {
		t.Fatalf("Suspicious len = %d, want 1: %+v", len(got.Suspicious), got.Suspicious)
	}
	if !strings.Contains(got.Suspicious[0], "expected action") {
		t.Fatalf("Suspicious = %q, want expected action note", got.Suspicious[0])
	}
	if got.Steps[0].Suspicion != "expected action talk, got inspect" {
		t.Fatalf("Suspicion = %q", got.Steps[0].Suspicion)
	}
}
