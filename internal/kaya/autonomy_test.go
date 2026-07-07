package kaya

import (
	"testing"

	"kaya/internal/game"
)

func TestDefaultStateAllowsModerateRisk(t *testing.T) {
	state := DefaultState()

	got := state.CanAttempt(game.DangerModerate)

	if !got.Allowed {
		t.Fatalf("Allowed = false, want true: %+v", got)
	}
	if got.NeedsConfirmation {
		t.Fatalf("NeedsConfirmation = true, want false: %+v", got)
	}
}

func TestHighFearLowTrustRefusesHighRisk(t *testing.T) {
	state := State{
		Stress: 85,
		Trust:  5,
		Fear:   80,
	}

	got := state.CanAttempt(game.DangerHigh)

	if got.Allowed {
		t.Fatalf("Allowed = true, want false: %+v", got)
	}
	if got.NeedsConfirmation {
		t.Fatalf("NeedsConfirmation = true, want false on refusal: %+v", got)
	}
	if got.Reason == "" {
		t.Fatal("Reason is empty")
	}
}

func TestHighTrustCanRequireConfirmationForHighRisk(t *testing.T) {
	state := State{
		Stress: 55,
		Trust:  90,
		Fear:   55,
	}

	got := state.CanAttempt(game.DangerHigh)

	if !got.Allowed {
		t.Fatalf("Allowed = false, want true: %+v", got)
	}
	if !got.NeedsConfirmation {
		t.Fatalf("NeedsConfirmation = false, want true: %+v", got)
	}
}

func TestApplyActionResultUpdatesEmotionalState(t *testing.T) {
	state := DefaultState()
	result := game.ActionResult{
		StressDelta: 5,
		TrustDelta:  3,
		Events: []game.WorldEvent{{
			Danger: game.DangerModerate,
		}},
	}

	got := state.Apply(result)

	if got.Stress <= state.Stress {
		t.Fatalf("Stress = %d, want greater than %d", got.Stress, state.Stress)
	}
	if got.Fear <= state.Fear {
		t.Fatalf("Fear = %d, want greater than %d", got.Fear, state.Fear)
	}
	if got.Trust != state.Trust+3 {
		t.Fatalf("Trust = %d, want %d", got.Trust, state.Trust+3)
	}
}

func TestApplyClampsEmotionalState(t *testing.T) {
	state := State{
		Stress:     99,
		Trust:      1,
		Fear:       99,
		Pain:       99,
		Exhaustion: 99,
	}

	got := state.Apply(game.ActionResult{
		StressDelta:     50,
		TrustDelta:      -50,
		FearDelta:       50,
		PainDelta:       50,
		ExhaustionDelta: 50,
	})

	if got.Stress != 100 || got.Fear != 100 || got.Pain != 100 || got.Exhaustion != 100 {
		t.Fatalf("state not clamped high: %+v", got)
	}
	if got.Trust != 0 {
		t.Fatalf("Trust = %d, want 0", got.Trust)
	}
}
