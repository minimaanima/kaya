package intent_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/llm"
	"kaya/internal/scenario"
	"kaya/internal/turn"
)

func TestOllamaNaturalLanguageIntents(t *testing.T) {
	if os.Getenv("KAYA_RUN_OLLAMA_TESTS") != "1" {
		t.Skip("set KAYA_RUN_OLLAMA_TESTS=1 to run Ollama integration tests")
	}

	model := envOrDefault("KAYA_OLLAMA_MODEL", "qwen3.5:4b")
	baseURL := envOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)

	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		t.Fatalf("NewOllamaClient returned error: %v", err)
	}

	parser := intent.NewParser(client)
	tests := []struct {
		name      string
		message   string
		action    intent.Action
		direction string
		item      string
		modifier  string
	}{
		{
			name:    "look around",
			message: "Look around.",
			action:  intent.ActionInspect,
		},
		{
			name:    "whats in the room",
			message: "What's in the room?",
			action:  intent.ActionInspect,
		},
		{
			name:    "can you see anything",
			message: "Can you see anything useful here?",
			action:  intent.ActionInspect,
		},
		{
			name:    "anything around you",
			message: "Is there anything around you?",
			action:  intent.ActionInspect,
		},
		{
			name:    "search coat",
			message: "Can you check the dead doctor's coat pockets?",
			action:  intent.ActionSearch,
		},
		{
			name:     "search coat with flashlight",
			message:  "Can you check the dead doctor's coat pockets but keep the flashlight low?",
			action:   intent.ActionSearch,
			item:     "flashlight",
			modifier: "keep_light_low",
		},
		{
			name:      "move left quietly",
			message:   "Maybe go left, but quietly.",
			action:    intent.ActionMove,
			direction: "left",
			modifier:  "quietly",
		},
		{
			name:    "wait",
			message: "Stay still for a second.",
			action:  intent.ActionWait,
		},
		{
			name:    "listen",
			message: "Can you listen at the door before opening it?",
			action:  intent.ActionListen,
		},
		{
			name:    "hide",
			message: "Get behind the cabinet and hide.",
			action:  intent.ActionHide,
		},
		{
			name:    "use key",
			message: "Try the key on the emergency stairwell door.",
			action:  intent.ActionUseItem,
			item:    "key",
		},
		{
			name:    "throw brick",
			message: "Throw the brick down the hallway to distract it.",
			action:  intent.ActionThrow,
			item:    "brick",
		},
		{
			name:    "ambiguous follow up",
			message: "Do it.",
			action:  intent.ActionUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			plan, err := parser.Parse(ctx, tt.message, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.message, err)
			}

			if len(plan.Actions) == 0 || plan.Actions[0].Intent.Action != tt.action {
				t.Fatalf("Action = %q, want %q; full plan: %+v", plan.Actions[0].Intent.Action, tt.action, plan)
			}
			got := plan.Actions[0].Intent
			if tt.direction != "" && !strings.Contains(strings.ToLower(got.Direction), tt.direction) {
				t.Fatalf("Direction = %q, want to contain %q; full intent: %+v", got.Direction, tt.direction, got)
			}
			if tt.item != "" && !strings.Contains(strings.ToLower(got.Item), tt.item) {
				t.Fatalf("Item = %q, want to contain %q; full intent: %+v", got.Item, tt.item, got)
			}
			if tt.modifier != "" && !containsString(got.Modifiers, tt.modifier) {
				t.Fatalf("Modifiers = %v, want %q; full intent: %+v", got.Modifiers, tt.modifier, got)
			}
		})
	}
}

func TestOllamaContextualSemanticTurnPlans(t *testing.T) {
	if os.Getenv("KAYA_RUN_OLLAMA_TESTS") != "1" {
		t.Skip("set KAYA_RUN_OLLAMA_TESTS=1 to run Ollama integration tests")
	}

	model := envOrDefault("KAYA_OLLAMA_MODEL", "qwen3.5:4b")
	baseURL := envOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)
	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		t.Fatalf("NewOllamaClient returned error: %v", err)
	}

	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil {
		t.Fatalf("ObserveRoom returned error: %v", err)
	}
	snapshot, err := state.PerceptionSnapshot()
	if err != nil {
		t.Fatalf("PerceptionSnapshot returned error: %v", err)
	}
	parser := intent.NewParser(client)

	tests := []struct {
		name  string
		text  string
		check func(*testing.T, intent.TurnPlan)
	}{
		{name: "singular doctor is engine ambiguous", text: "inspect the doctor", check: func(t *testing.T, plan intent.TurnPlan) {
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != intent.ActionInspect || plan.Actions[0].TargetMode != intent.TargetSingle {
				t.Fatalf("plan = %#v, want singular inspect", plan)
			}
			result := turn.NewExecutor(state).Execute(plan)
			if result.StopReason != "target_ambiguous" {
				t.Fatalf("stop reason = %q, want target_ambiguous; result = %#v", result.StopReason, result)
			}
		}},
		{name: "explicit both uses all", text: "inspect both doctors", check: requireAllObjectAction(intent.ActionInspect)},
		{name: "remembered them uses all", text: "search them", check: requireAllObjectAction(intent.ActionSearch)},
		{name: "unsupported inventory question clarifies", text: "do they have anything", check: func(t *testing.T, plan intent.TurnPlan) {
			if len(plan.Actions) != 0 || !plan.NeedsClarification || strings.TrimSpace(plan.ClarificationQuestion) == "" {
				t.Fatalf("plan = %#v, want clarification without actions", plan)
			}
		}},
		{name: "compound search and life question", text: "search the doctors are they dead", check: func(t *testing.T, plan intent.TurnPlan) {
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != intent.ActionSearch || plan.Actions[0].TargetMode != intent.TargetAll {
				t.Fatalf("actions = %#v, want one all-target search", plan.Actions)
			}
			if len(plan.Questions) != 1 || plan.Questions[0].Kind != game.FactLifeStatus || plan.Questions[0].TargetMode != intent.TargetAll {
				t.Fatalf("questions = %#v, want one all-target life-status question", plan.Questions)
			}
		}},
		{name: "wall wording explores", text: "feel along the walls for another exit", check: func(t *testing.T, plan intent.TurnPlan) {
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != intent.ActionExplore {
				t.Fatalf("plan = %#v, want explore action", plan)
			}
		}},
		{name: "cabinet typo resolves", text: "what is isnide the storage cabiner", check: func(t *testing.T, plan intent.TurnPlan) {
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != intent.ActionInspect {
				t.Fatalf("plan = %#v, want inspect action", plan)
			}
			result := turn.NewExecutor(state).Execute(plan)
			if len(result.Outcomes) != 1 || result.Outcomes[0].TargetObjectID != scenario.ObjectStorageCabinet {
				t.Fatalf("result = %#v, want Storage Cabinet resolution", result)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			plan, err := parser.Parse(ctx, tt.text, snapshot)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.text, err)
			}
			tt.check(t, plan)
		})
	}
}

func TestOllamaIntentCorpus(t *testing.T) {
	if !truthyEnv("KAYA_OLLAMA_EVAL") {
		t.Skip("set KAYA_OLLAMA_EVAL=1 to run the Ollama intent corpus")
	}
	model := envOrDefault("KAYA_OLLAMA_MODEL", "qwen3.5:4b")
	baseURL := envOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)
	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		t.Fatal(err)
	}
	parser := intent.NewParser(client)
	eval := corpusEvaluation{Total: len(intentCorpus)}

	for _, tc := range intentCorpus {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		plan, provenance, parseErr := parser.ParseWithProvenance(ctx, tc.Message, game.PerceptionSnapshot{})
		cancel()
		if parseErr != nil {
			eval.Errors = append(eval.Errors, fmt.Sprintf("%s (%q): parser error: %v", tc.Name, tc.Message, parseErr))
			continue
		}
		eval.Record(tc, plan, provenance)
	}

	for _, mismatch := range eval.RawMismatches {
		t.Logf("RAW MISMATCH: %s", mismatch)
	}
	for _, mismatch := range eval.ResolvedMismatches {
		t.Logf("RESOLVED MISMATCH: %s", mismatch)
	}
	for _, parseError := range eval.Errors {
		t.Logf("ERROR: %s", parseError)
	}
	t.Logf("raw model: %d/%d exact matches, %d mismatches, %.1f%% accuracy",
		eval.RawMatches, eval.Total, len(eval.RawMismatches), eval.RawAccuracy())
	t.Logf("resolved parser: %d/%d exact matches, %d mismatches, %.1f%% accuracy (threshold 90.0%%)",
		eval.ResolvedMatches, eval.Total, len(eval.ResolvedMismatches), eval.ResolvedAccuracy())
	t.Logf("resolution diagnostics: %d canonicalized/fallback-assisted, %d repaired, %d generator/decoding fallback errors, %d total errors",
		eval.FallbackAssisted, eval.Repairs, eval.FallbackErrorCount, len(eval.Errors))
	if eval.Fails(90) {
		t.Fatalf("Ollama intent corpus failed: resolved accuracy %.1f%%, threshold 90.0%%, generator/decoding fallback errors %d, total errors %d",
			eval.ResolvedAccuracy(), eval.FallbackErrorCount, len(eval.Errors))
	}
}

func requireAllObjectAction(action intent.Action) func(*testing.T, intent.TurnPlan) {
	return func(t *testing.T, plan intent.TurnPlan) {
		if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != action || plan.Actions[0].TargetMode != intent.TargetAll {
			t.Fatalf("plan = %#v, want one %q action with targetMode all", plan, action)
		}
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(value, expected) {
			return true
		}
	}
	return false
}

func truthyEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
