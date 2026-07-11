package response

import (
	"testing"

	"kaya/internal/game"
	"kaya/internal/turn"
)

func TestFallbackIncludesRequiredFactsOnceInOrder(t *testing.T) {
	bundle := turn.FactBundle{Facts: []game.Fact{
		{ID: "f001", Text: "I search Doctor Near Cabinet.", Required: true},
		{ID: "f002", Text: "Doctor Near Cabinet is dead.", Required: true},
		{ID: "f003", Text: "I search Doctor Near Door.", Required: true},
		{ID: "f004", Text: "Doctor Near Door is dead.", Required: true},
	}}
	got := (Fallback{}).Render(bundle)
	want := "I search Doctor Near Cabinet. Doctor Near Cabinet is dead. I search Doctor Near Door. Doctor Near Door is dead."
	if got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}

func TestFallbackUsesClarificationWhenNoFacts(t *testing.T) {
	got := (Fallback{}).Render(turn.FactBundle{})
	if got != "What do you want me to do?" {
		t.Fatalf("Render = %q", got)
	}
}

func TestFallbackSkipsOptionalEmptyAndDuplicateFacts(t *testing.T) {
	bundle := turn.FactBundle{Facts: []game.Fact{
		{ID: "optional", Text: "Optional context", Required: false},
		{ID: "empty", Text: "  ", Required: true},
		{ID: "f001", Text: "First", Required: true},
		{ID: "f001", Text: "Duplicate", Required: true},
	}}
	if got, want := (Fallback{}).Render(bundle), "First."; got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}

func TestFallbackPreservesSentenceEndings(t *testing.T) {
	bundle := turn.FactBundle{Facts: []game.Fact{
		{ID: "f001", Text: "Really!", Required: true},
		{ID: "f002", Text: "Really?", Required: true},
		{ID: "f003", Text: "Really.", Required: true},
	}}
	if got, want := (Fallback{}).Render(bundle), "Really! Really? Really."; got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}
