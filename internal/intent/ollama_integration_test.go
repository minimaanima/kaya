package intent_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"kaya/internal/intent"
	"kaya/internal/llm"
)

func TestOllamaNaturalLanguageIntents(t *testing.T) {
	if os.Getenv("KAYA_RUN_OLLAMA_TESTS") != "1" {
		t.Skip("set KAYA_RUN_OLLAMA_TESTS=1 to run Ollama integration tests")
	}

	model := envOrDefault("KAYA_OLLAMA_MODEL", "mistral:latest")
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

			got, err := parser.Parse(ctx, tt.message)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.message, err)
			}

			if got.Action != tt.action {
				t.Fatalf("Action = %q, want %q; full intent: %+v", got.Action, tt.action, got)
			}
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

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(value, expected) {
			return true
		}
	}
	return false
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
