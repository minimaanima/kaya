package playtest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"kaya/internal/intent"
	"kaya/internal/llm"
	"kaya/internal/response"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/session"
)

func TestLiveSliceEnabledRecognizesTruthyValues(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("KAYA_LIVE_SLICE_TESTS", value)
			if !liveSliceEnabled() {
				t.Fatalf("KAYA_LIVE_SLICE_TESTS=%q did not enable live certification", value)
			}
		})
	}

	for _, value := range []string{"", "0", "false", "no", "off", "unexpected"} {
		t.Run("disabled_"+value, func(t *testing.T) {
			t.Setenv("KAYA_LIVE_SLICE_TESTS", value)
			if liveSliceEnabled() {
				t.Fatalf("KAYA_LIVE_SLICE_TESTS=%q unexpectedly enabled live certification", value)
			}
		})
	}
}

func TestLiveProvenanceSummaryCountsResponseRepairs(t *testing.T) {
	summary := liveProvenanceSummary{}
	summary.record(Step{Processed: true, Turn: session.ProcessedTurn{
		SemanticProvenance: intent.SemanticProvenance{Source: intent.ParseSourceModel, HasRawPlan: true},
		Response: response.Response{
			RepairAttempted: true,
			RepairSucceeded: true,
		},
	}}, "repaired response")

	if summary.ResponseRepairAttempts != 1 || summary.ResponseRepairSuccesses != 1 || summary.ResponseFallbacks != 0 || summary.ResponseGenerated != 1 {
		t.Fatalf("live provenance = %#v", summary)
	}
}

func TestLiveProvenanceSummarySkipsUnprocessedTurnCounters(t *testing.T) {
	summary := liveProvenanceSummary{}
	summary.record(Step{
		Error: "parser unavailable",
		Turn: session.ProcessedTurn{
			SemanticProvenance: intent.SemanticProvenance{Source: intent.ParseSourceRepair, HasRawPlan: true},
			Response:           response.Response{UsedFallback: true, RepairAttempted: true, RepairSucceeded: true},
		},
	}, "unprocessed response")

	if summary.Turns != 1 || summary.ProcessedTurns != 0 || summary.GeneratorUsed != 0 || summary.Repaired != 0 || summary.RawDTOs != 0 || summary.SemanticPlans != 0 || summary.ValidationErrors != 0 || summary.PendingClarifications != 0 || summary.ParseFallbacks != 0 || summary.FallbackErrors != 0 || summary.ResponseGenerated != 0 || summary.ResponseFallbacks != 0 || summary.ResponseRepairAttempts != 0 || summary.ResponseRepairSuccesses != 0 || summary.LastResponseRaw != "" {
		t.Fatalf("live provenance = %#v", summary)
	}
}

func TestLiveProvenanceSummaryCountsProcessedInvariantStep(t *testing.T) {
	summary := liveProvenanceSummary{}
	summary.record(Step{
		Processed:  true,
		Violations: []Violation{{Code: "response_debug_marker", Detail: "debug response"}},
		Turn: session.ProcessedTurn{
			SemanticProvenance: intent.SemanticProvenance{Source: intent.ParseSourceModel, HasRawPlan: true},
			Response:           response.Response{RepairAttempted: true, RepairSucceeded: true},
		},
	}, "processed response")

	if summary.Turns != 1 || summary.ProcessedTurns != 1 || summary.GeneratorUsed != 1 || summary.RawDTOs != 1 || summary.SemanticPlans != 1 || summary.ResponseGenerated != 1 || summary.ResponseRepairAttempts != 1 || summary.ResponseRepairSuccesses != 1 {
		t.Fatalf("live provenance = %#v", summary)
	}
}

func TestLiveProvenanceSummaryRejectsUnknownSourceAndMissingRawPlan(t *testing.T) {
	for _, test := range []struct {
		name       string
		provenance intent.SemanticProvenance
	}{
		{name: "empty source", provenance: intent.SemanticProvenance{HasRawPlan: true}},
		{name: "unknown source", provenance: intent.SemanticProvenance{Source: intent.ParseSource("unknown"), HasRawPlan: true}},
		{name: "missing raw plan", provenance: intent.SemanticProvenance{Source: intent.ParseSourceModel}},
	} {
		t.Run(test.name, func(t *testing.T) {
			step := Step{Processed: true, Turn: session.ProcessedTurn{SemanticProvenance: test.provenance}}
			summary := liveProvenanceSummary{}
			summary.record(step, "")
			if reason := summary.lastTurnFailure(step); reason == "" {
				t.Fatal("lastTurnFailure accepted incomplete parser provenance")
			}
			if reason := summary.acceptanceFailure(1); reason == "" {
				t.Fatalf("acceptanceFailure accepted summary %#v", summary)
			}
		})
	}
}

func TestLiveProvenanceSummaryFinalAcceptanceRequiresExactCounts(t *testing.T) {
	validStep := Step{Processed: true, Turn: session.ProcessedTurn{
		SemanticProvenance: intent.SemanticProvenance{Source: intent.ParseSourceModel, HasRawPlan: true},
	}}
	valid := liveProvenanceSummary{}
	valid.record(validStep, "")
	if reason := valid.acceptanceFailure(1); reason != "" {
		t.Fatalf("complete summary rejected: %s; summary=%#v", reason, valid)
	}

	tests := []struct {
		name   string
		mutate func(*liveProvenanceSummary)
	}{
		{name: "expected processed turns", mutate: func(s *liveProvenanceSummary) { s.ProcessedTurns = 0 }},
		{name: "model or repair source", mutate: func(s *liveProvenanceSummary) { s.GeneratorUsed = 0 }},
		{name: "raw DTOs", mutate: func(s *liveProvenanceSummary) { s.RawDTOs = 0 }},
		{name: "semantic plans", mutate: func(s *liveProvenanceSummary) { s.SemanticPlans = 0 }},
		{name: "generated responses", mutate: func(s *liveProvenanceSummary) { s.ResponseGenerated = 0 }},
		{name: "parse fallback", mutate: func(s *liveProvenanceSummary) { s.ParseFallbacks = 1 }},
		{name: "fallback error", mutate: func(s *liveProvenanceSummary) { s.FallbackErrors = 1 }},
		{name: "response fallback", mutate: func(s *liveProvenanceSummary) { s.ResponseFallbacks = 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary := valid
			test.mutate(&summary)
			if reason := summary.acceptanceFailure(1); reason == "" {
				t.Fatalf("acceptanceFailure accepted mismatched summary %#v", summary)
			}
		})
	}
	if reason := valid.acceptanceFailure(2); reason == "" {
		t.Fatalf("acceptanceFailure accepted one processed turn when two were required: %#v", valid)
	}
}

func TestOllamaPrototypeCompletePlaythroughs(t *testing.T) {
	if !liveSliceEnabled() {
		t.Skip("set KAYA_LIVE_SLICE_TESTS=1 to run live Ollama prototype certification")
	}

	model := liveEnvOrDefault("KAYA_OLLAMA_MODEL", "qwen3.5:4b")
	baseURL := liveEnvOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)
	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		t.Fatalf("create Ollama client: %v", err)
	}
	t.Logf("live Ollama configuration: model=%q base_url=%q", model, baseURL)

	for _, seed := range []int64{10, 11, 12} {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			run, err := rungen.Generate(
				rungen.RunConfig{Seed: seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
				runscenario.PrototypeDefinition(),
			)
			if err != nil {
				t.Fatalf("seed %d: generate run: %v", seed, err)
			}

			parser := intent.NewParser(client)
			responseRecorder := &liveResponseRecorder{generator: client}
			composer := response.NewComposer(responseRecorder)
			runner := NewRunner(runscenario.PrototypeDefinition(), run, parser, composer)
			messages, err := PrototypeWinningMessages(run, seed)
			if err != nil {
				failLiveSession(t, runner, liveProvenanceSummary{}, "seed %d: build winning messages: %v", seed, err)
			}

			summary := liveProvenanceSummary{}
			for _, message := range messages {
				step, stepErr := runner.Step(context.Background(), message)
				summary.record(step, responseRecorder.raw)
				if stepErr != nil {
					failLiveSession(t, runner, summary, "seed %d message %q: %v", seed, message, stepErr)
				}
				if reason := summary.lastTurnFailure(step); reason != "" {
					failLiveSession(t, runner, summary, "seed %d message %q: %s", seed, message, reason)
				}
			}

			session := runner.Session()
			if runner.State().CurrentRoomID != scenario.RoomStairwell {
				failLiveSession(t, runner, summary, "seed %d ended in room %q, want %q", seed, runner.State().CurrentRoomID, scenario.RoomStairwell)
			}
			if session.ObjectiveEmissions != 1 {
				failLiveSession(t, runner, summary, "seed %d objective emissions = %d, want 1", seed, session.ObjectiveEmissions)
			}
			if reason := summary.acceptanceFailure(len(messages)); reason != "" {
				failLiveSession(t, runner, summary, "seed %d incomplete live provenance: %s", seed, reason)
			}
			t.Logf("seed %d live provenance: %s", seed, summary)
		})
	}
}

func liveSliceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KAYA_LIVE_SLICE_TESTS"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func liveEnvOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

type liveProvenanceSummary struct {
	Turns                   int
	ProcessedTurns          int
	GeneratorUsed           int
	Repaired                int
	RawDTOs                 int
	SemanticPlans           int
	ValidationErrors        int
	PendingClarifications   int
	ParseFallbacks          int
	FallbackErrors          int
	ResponseGenerated       int
	ResponseFallbacks       int
	ResponseRepairAttempts  int
	ResponseRepairSuccesses int
	LastResponseRaw         string
}

func (s *liveProvenanceSummary) record(step Step, responseRaw string) {
	s.Turns++
	if !step.Processed {
		return
	}
	s.ProcessedTurns++
	provenance := step.Turn.SemanticProvenance
	if provenance.Source == intent.ParseSourceModel || provenance.Source == intent.ParseSourceRepair {
		s.GeneratorUsed++
	}
	if provenance.Source == intent.ParseSourceRepair {
		s.Repaired++
	}
	if provenance.HasRawPlan {
		s.RawDTOs++
	}
	s.SemanticPlans++
	s.ValidationErrors += len(provenance.InitialValidationErrors) + len(provenance.ValidationErrors)
	if step.Turn.Pending != nil {
		s.PendingClarifications++
	}
	if provenance.Source == intent.ParseSourceFallback {
		s.ParseFallbacks++
	}
	if provenance.FallbackError != nil {
		s.FallbackErrors++
	}
	if step.Turn.Response.RepairAttempted {
		s.ResponseRepairAttempts++
	}
	if step.Turn.Response.RepairSucceeded {
		s.ResponseRepairSuccesses++
	}
	if step.Turn.Response.UsedFallback {
		s.ResponseFallbacks++
		s.LastResponseRaw = responseRaw
		return
	}
	s.ResponseGenerated++
}

func (s liveProvenanceSummary) lastTurnFailure(step Step) string {
	provenance := step.Turn.SemanticProvenance
	if provenance.Source != intent.ParseSourceModel && provenance.Source != intent.ParseSourceRepair {
		return fmt.Sprintf("intent parser source is not model or repair: %q", provenance.Source)
	}
	if !provenance.HasRawPlan {
		return "intent parser did not capture a raw plan"
	}
	if provenance.FallbackError != nil {
		return fmt.Sprintf("intent parser fallback/provenance error: %v", provenance.FallbackError)
	}
	if step.Turn.Response.UsedFallback {
		return fmt.Sprintf("response generation used fallback: %s", step.Turn.Response.FallbackReason)
	}
	return ""
}

func (s liveProvenanceSummary) acceptanceFailure(expectedProcessed int) string {
	if s.Turns != expectedProcessed {
		return fmt.Sprintf("attempted turns=%d, want %d", s.Turns, expectedProcessed)
	}
	if s.ProcessedTurns != expectedProcessed {
		return fmt.Sprintf("processed turns=%d, want %d", s.ProcessedTurns, expectedProcessed)
	}
	if s.GeneratorUsed != s.ProcessedTurns {
		return fmt.Sprintf("model/repair sources=%d, want processed turns %d", s.GeneratorUsed, s.ProcessedTurns)
	}
	if s.RawDTOs != s.ProcessedTurns {
		return fmt.Sprintf("raw DTOs=%d, want processed turns %d", s.RawDTOs, s.ProcessedTurns)
	}
	if s.SemanticPlans != s.ProcessedTurns {
		return fmt.Sprintf("semantic plans=%d, want processed turns %d", s.SemanticPlans, s.ProcessedTurns)
	}
	if s.ParseFallbacks != 0 || s.FallbackErrors != 0 {
		return fmt.Sprintf("parse fallbacks/errors=%d/%d, want 0/0", s.ParseFallbacks, s.FallbackErrors)
	}
	if s.ResponseGenerated != s.ProcessedTurns {
		return fmt.Sprintf("generated responses=%d, want processed turns %d", s.ResponseGenerated, s.ProcessedTurns)
	}
	if s.ResponseFallbacks != 0 {
		return fmt.Sprintf("response fallbacks=%d, want 0", s.ResponseFallbacks)
	}
	if s.ResponseRepairAttempts != s.ResponseRepairSuccesses {
		return fmt.Sprintf("response repair attempts/successes=%d/%d, want equality", s.ResponseRepairAttempts, s.ResponseRepairSuccesses)
	}
	if s.LastResponseRaw != "" {
		return "last fallback response raw output is non-empty"
	}
	return ""
}

func (s liveProvenanceSummary) String() string {
	return fmt.Sprintf(
		"turns=%d processed_turns=%d generator_used=%d repaired=%d raw_dtos=%d semantic_plans=%d validation_errors=%d pending_clarifications=%d parse_fallbacks=%d fallback_errors=%d response_generated=%d response_fallbacks=%d response_repair_attempts=%d response_repair_successes=%d last_response_raw=%q",
		s.Turns,
		s.ProcessedTurns,
		s.GeneratorUsed,
		s.Repaired,
		s.RawDTOs,
		s.SemanticPlans,
		s.ValidationErrors,
		s.PendingClarifications,
		s.ParseFallbacks,
		s.FallbackErrors,
		s.ResponseGenerated,
		s.ResponseFallbacks,
		s.ResponseRepairAttempts,
		s.ResponseRepairSuccesses,
		s.LastResponseRaw,
	)
}

type liveResponseRecorder struct {
	generator response.StructuredGenerator
	raw       string
}

func (g *liveResponseRecorder) GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, schema any) (string, error) {
	raw, err := g.generator.GenerateJSON(ctx, systemPrompt, userPrompt, schema)
	g.raw = raw
	return raw, err
}

func failLiveSession(t *testing.T, runner *Runner, summary liveProvenanceSummary, format string, args ...any) {
	t.Helper()
	t.Fatalf("%s\nlive provenance: %s\n\n%s", fmt.Sprintf(format, args...), summary, RenderMarkdown(runner.Session()))
}
