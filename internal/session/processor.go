package session

import (
	"context"
	"fmt"
	"time"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/response"
	"kaya/internal/turn"
	"kaya/internal/world"
)

type Parser interface {
	ParseWithProvenance(context.Context, string, game.PerceptionSnapshot) (intent.TurnPlan, intent.ParseProvenance, error)
}

type Composer interface {
	Compose(context.Context, turn.FactBundle) response.Response
}

type ProcessedTurn struct {
	Plan                  intent.TurnPlan
	Provenance            intent.ParseProvenance
	SemanticPlan          intent.SemanticPlan
	SemanticProvenance    intent.SemanticProvenance
	ClarificationDecision *intent.ClarificationDecision
	Pending               *turn.PendingSemanticAction
	Result                turn.Result
	Response              response.Response
	DurationSeconds       int
}

func ProcessTurn(ctx context.Context, message string, state *world.State, parser Parser, composer Composer) (ProcessedTurn, error) {
	if state == nil || parser == nil || composer == nil {
		return ProcessedTurn{}, fmt.Errorf("session dependencies must not be nil")
	}

	snapshot, err := state.PerceptionSnapshot()
	if err != nil {
		return ProcessedTurn{}, fmt.Errorf("snapshot world: %w", err)
	}

	plan, conversational := intent.PureConversationPlan(message)
	var provenance intent.ParseProvenance
	if conversational {
		provenance = intent.ParseProvenance{Source: intent.ParseSourceFallback}
	} else {
		parseCtx, cancelParse := context.WithTimeout(ctx, 60*time.Second)
		plan, provenance, err = parser.ParseWithProvenance(parseCtx, message, snapshot)
		cancelParse()
		if err != nil {
			return ProcessedTurn{}, err
		}
	}

	result := turn.NewExecutor(state).Execute(plan)
	responseCtx, cancelResponse := context.WithTimeout(ctx, 60*time.Second)
	composed := composer.Compose(responseCtx, result.FactBundle(message))
	cancelResponse()

	return ProcessedTurn{
		Plan:            plan,
		Provenance:      provenance,
		Result:          result,
		Response:        composed,
		DurationSeconds: ResultDuration(result),
	}, nil
}

func ResultDuration(result turn.Result) int {
	total := 0
	for _, outcome := range result.Outcomes {
		total += outcome.Result.DurationSeconds
	}
	return total
}
