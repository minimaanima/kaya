package playtest

import (
	"context"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/response"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
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

type errorParser struct {
	err error
}

func (p errorParser) ParseWithProvenance(
	_ context.Context,
	_ string,
	_ game.PerceptionSnapshot,
) (intent.TurnPlan, intent.ParseProvenance, error) {
	return intent.TurnPlan{}, intent.ParseProvenance{}, p.err
}

type fallbackComposer struct{}

func (fallbackComposer) Compose(_ context.Context, bundle turn.FactBundle) response.Response {
	return response.NewComposer(nil).Compose(context.Background(), bundle)
}

func mustGeneratedRun(t *testing.T, seed int64) rungen.GeneratedRun {
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

func hasViolation(violations []Violation, code string) bool {
	for _, violation := range violations {
		if violation.Code == code {
			return true
		}
	}
	return false
}
