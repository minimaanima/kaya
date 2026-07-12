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
	summary.record(Step{Turn: session.ProcessedTurn{
		Provenance: intent.ParseProvenance{Source: intent.ParseSourceModel, HasRawPlan: true},
		Response: response.Response{
			RepairAttempted: true,
			RepairSucceeded: true,
		},
	}}, "repaired response")

	if summary.ResponseRepairAttempts != 1 || summary.ResponseRepairSuccesses != 1 || summary.ResponseFallbacks != 0 || summary.ResponseGenerated != 1 {
		t.Fatalf("live provenance = %#v", summary)
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
	GeneratorUsed           int
	Repaired                int
	Canonicalized           int
	RawPlans                int
	ResolvedPlans           int
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
	provenance := step.Turn.Provenance
	if provenance.Source == intent.ParseSourceModel || provenance.Source == intent.ParseSourceRepair {
		s.GeneratorUsed++
	}
	if provenance.Source == intent.ParseSourceRepair {
		s.Repaired++
	}
	if provenance.Canonicalized {
		s.Canonicalized++
	}
	if provenance.HasRawPlan {
		s.RawPlans++
	}
	s.ResolvedPlans++
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
	provenance := step.Turn.Provenance
	if provenance.Source == intent.ParseSourceFallback {
		return "intent parser used deterministic fallback"
	}
	if provenance.FallbackError != nil {
		return fmt.Sprintf("intent parser fallback/provenance error: %v", provenance.FallbackError)
	}
	if step.Turn.Response.UsedFallback {
		return fmt.Sprintf("response generation used fallback: %s", step.Turn.Response.FallbackReason)
	}
	return ""
}

func (s liveProvenanceSummary) String() string {
	return fmt.Sprintf(
		"turns=%d generator_used=%d repaired=%d canonicalized=%d raw_plans=%d resolved_plans=%d parse_fallbacks=%d fallback_errors=%d response_generated=%d response_fallbacks=%d response_repair_attempts=%d response_repair_successes=%d last_response_raw=%q",
		s.Turns,
		s.GeneratorUsed,
		s.Repaired,
		s.Canonicalized,
		s.RawPlans,
		s.ResolvedPlans,
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
