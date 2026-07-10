package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"kaya/internal/intent"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
)

type fakePlaytestGenerator struct {
	responses []string
}

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
