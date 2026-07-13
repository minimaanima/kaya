package session

import (
	"context"
	"reflect"
	"testing"
	"time"

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

type recordingParser struct {
	ctx context.Context
}

func (p *recordingParser) ParseWithProvenance(ctx context.Context, message string, _ game.PerceptionSnapshot) (intent.TurnPlan, intent.ParseProvenance, error) {
	p.ctx = ctx
	plan := intent.FallbackPlan(message)
	return plan, intent.ParseProvenance{Source: intent.ParseSourceFallback}, nil
}

type recordingComposer struct {
	ctx    context.Context
	bundle turn.FactBundle
}

func (c *recordingComposer) Compose(ctx context.Context, bundle turn.FactBundle) response.Response {
	c.ctx = ctx
	c.bundle = bundle
	return response.NewComposer(nil).Compose(ctx, bundle)
}

func TestProcessTurnSetsAndCancelsStageDeadlinesWithExactFactBundle(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	parser := &recordingParser{}
	composer := &recordingComposer{}
	started := time.Now()

	got, err := ProcessTurn(context.Background(), "go east", state, parser, composer)
	if err != nil {
		t.Fatal(err)
	}
	for name, stageCtx := range map[string]context.Context{"parse": parser.ctx, "compose": composer.ctx} {
		deadline, ok := stageCtx.Deadline()
		if !ok {
			t.Fatalf("%s context has no deadline", name)
		}
		remaining := deadline.Sub(started)
		if remaining < 59*time.Second || remaining > 61*time.Second {
			t.Fatalf("%s deadline remaining = %s, want approximately 60s", name, remaining)
		}
		select {
		case <-stageCtx.Done():
			if stageCtx.Err() != context.Canceled {
				t.Fatalf("%s context error = %v, want canceled", name, stageCtx.Err())
			}
		default:
			t.Fatalf("%s context was not canceled after stage return", name)
		}
	}
	wantBundle := got.Result.FactBundle("go east")
	if !reflect.DeepEqual(composer.bundle, wantBundle) {
		t.Fatalf("composer bundle = %#v, want %#v", composer.bundle, wantBundle)
	}
}

func TestProcessTurnPropagatesImmediateParentCancellation(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	parser := &recordingParser{}
	composer := &recordingComposer{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := ProcessTurn(ctx, "go east", state, parser, composer); err != nil {
		t.Fatal(err)
	}
	if parser.ctx.Err() != context.Canceled || composer.ctx.Err() != context.Canceled {
		t.Fatalf("stage errors = parse:%v compose:%v, want canceled", parser.ctx.Err(), composer.ctx.Err())
	}
}
