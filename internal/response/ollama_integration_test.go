package response_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"kaya/internal/game"
	"kaya/internal/kaya"
	"kaya/internal/llm"
	"kaya/internal/response"
	"kaya/internal/turn"
)

func TestOllamaFactLockedResponseDraft(t *testing.T) {
	if os.Getenv("KAYA_RUN_OLLAMA_TESTS") != "1" {
		t.Skip("set KAYA_RUN_OLLAMA_TESTS=1 to run Ollama integration tests")
	}

	model := responseEnvOrDefault("KAYA_OLLAMA_MODEL", "qwen3.5:4b")
	baseURL := responseEnvOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)
	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		t.Fatalf("NewOllamaClient returned error: %v", err)
	}

	bundle := turn.FactBundle{
		PlayerMessage: "search the doctors are they dead",
		Emotion:       kaya.EmotionUneasy,
		Facts: []game.Fact{
			{ID: "f001", Kind: game.FactAction, Subject: "Doctor Near Cabinet", Value: "searched", Text: "I searched Doctor Near Cabinet.", Required: true},
			{ID: "f002", Kind: game.FactAction, Subject: "Doctor Near Door", Value: "searched", Text: "I searched Doctor Near Door.", Required: true},
			{ID: "f003", Kind: game.FactLifeStatus, Subject: "Doctor Near Cabinet", Value: "dead", Text: "Doctor Near Cabinet is dead.", Required: true},
			{ID: "f004", Kind: game.FactLifeStatus, Subject: "Doctor Near Door", Value: "dead", Text: "Doctor Near Door is dead.", Required: true},
			{ID: "f005", Kind: game.FactElapsedTime, Subject: "time", Value: "65", Text: "65 seconds pass.", Required: true},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	got := response.NewComposer(client).Compose(ctx, bundle)
	if got.UsedFallback {
		t.Fatalf("response used fallback: %s (%s)", got.Text, got.FallbackReason)
	}
	if strings.TrimSpace(got.Text) == "" {
		t.Fatal("response text is empty")
	}
	approved := make(map[game.FactID]bool, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		approved[fact.ID] = true
	}
	for _, id := range got.UsedFactIDs {
		if !approved[id] {
			t.Fatalf("response used unapproved fact ID %q", id)
		}
	}
	if strings.Contains(strings.ToLower(got.Text), "basement door") || strings.Contains(strings.ToLower(got.Text), "monster") {
		t.Fatalf("response contains unknown named entity: %q", got.Text)
	}
}

func responseEnvOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
