package session

import (
	"context"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/response"
	"kaya/internal/scenario"
	"kaya/internal/turn"
)

type fallbackParser struct{}

func (fallbackParser) ParseWithProvenance(
	_ context.Context,
	message string,
	_ game.PerceptionSnapshot,
) (intent.TurnPlan, intent.ParseProvenance, error) {
	plan := intent.FallbackPlan(message)
	return plan, intent.ParseProvenance{
		Source:     intent.ParseSourceFallback,
		RawPlan:    plan,
		HasRawPlan: true,
	}, nil
}

type fallbackComposer struct{}

func (fallbackComposer) Compose(_ context.Context, bundle turn.FactBundle) response.Response {
	return response.NewComposer(nil).Compose(context.Background(), bundle)
}

func TestProcessTurnUsesSharedStateAndCapturesProvenance(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	got, err := ProcessTurn(context.Background(), "go east", state, fallbackParser{}, fallbackComposer{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan.Actions[0].Intent.Action != intent.ActionMove {
		t.Fatalf("plan = %#v", got.Plan)
	}
	if got.Provenance.Source != intent.ParseSourceFallback {
		t.Fatalf("provenance = %#v", got.Provenance)
	}
	if state.CurrentRoomID != scenario.RoomStorage {
		t.Fatalf("room = %q", state.CurrentRoomID)
	}
	if got.DurationSeconds != 20 || state.NowSeconds != 20 {
		t.Fatalf("duration=%d time=%d", got.DurationSeconds, state.NowSeconds)
	}
}
