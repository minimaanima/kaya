package playtest

import (
	"context"
	"strings"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/response"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/turn"
)

type fallbackParser struct{}

func (fallbackParser) ParseSemanticWithProvenance(
	_ context.Context,
	message string,
	_ game.PerceptionSnapshot,
) (intent.SemanticPlan, intent.SemanticProvenance, error) {
	return semanticFallbackPlan(message), intent.SemanticProvenance{
		Source: intent.ParseSourceFallback,
	}, nil
}

func (fallbackParser) ParseClarification(_ context.Context, message string, candidates []intent.CandidateView) (intent.ClarificationDecision, error) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "both" || normalized == "all" || normalized == "both of them" || normalized == "all of them" {
		return intent.ClarificationDecision{Kind: intent.ClarificationAll}, nil
	}
	for _, candidate := range candidates {
		if normalized == strings.ToLower(candidate.Name) {
			return intent.ClarificationDecision{Kind: intent.ClarificationSelect, Ordinal: candidate.Ordinal}, nil
		}
		for _, alias := range candidate.Aliases {
			if normalized == strings.ToLower(alias) {
				return intent.ClarificationDecision{Kind: intent.ClarificationSelect, Ordinal: candidate.Ordinal}, nil
			}
		}
	}
	return intent.ClarificationDecision{Kind: intent.ClarificationNewCommand}, nil
}

type errorParser struct {
	err error
}

func (p errorParser) ParseSemanticWithProvenance(
	_ context.Context,
	_ string,
	_ game.PerceptionSnapshot,
) (intent.SemanticPlan, intent.SemanticProvenance, error) {
	return intent.SemanticPlan{}, intent.SemanticProvenance{}, p.err
}

func (p errorParser) ParseClarification(_ context.Context, _ string, _ []intent.CandidateView) (intent.ClarificationDecision, error) {
	return intent.ClarificationDecision{}, p.err
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

func semanticFallbackPlan(message string) intent.SemanticPlan {
	legacy := intent.FallbackPlan(message)
	plan := intent.SemanticPlan{
		Actions:               make([]intent.SemanticAction, 0, len(legacy.Actions)),
		Questions:             append([]intent.FactQuestion(nil), legacy.Questions...),
		RawText:               legacy.RawText,
		NeedsClarification:    legacy.NeedsClarification,
		ClarificationQuestion: legacy.ClarificationQuestion,
	}
	for _, planned := range legacy.Actions {
		if action := semanticFallbackAction(planned, message); action != nil {
			plan.Actions = append(plan.Actions, action)
		}
	}
	return plan
}

func semanticFallbackAction(planned intent.PlannedAction, message string) intent.SemanticAction {
	in := planned.Intent
	evidence := in.RawText
	if evidence == "" {
		evidence = message
	}
	quantity := planned.TargetMode
	if quantity == "" || quantity == intent.TargetSingle {
		quantity = intent.TargetOne
	}
	target := intent.Reference{Mention: in.Target, Quantity: quantity}
	item := intent.Reference{Mention: in.Item, Quantity: quantity}
	switch in.Action {
	case intent.ActionMove:
		return intent.MoveAction{Direction: in.Direction, Evidence: evidence}
	case intent.ActionInspect:
		return intent.InspectAction{Target: target, Evidence: evidence}
	case intent.ActionSearch:
		return intent.SearchAction{Target: target, Evidence: evidence}
	case intent.ActionTakeItem:
		return intent.TakeAction{Target: target, Evidence: evidence}
	case intent.ActionUseItem:
		return intent.UseAction{Item: item, Target: target, Evidence: evidence}
	case intent.ActionTurnOn:
		return intent.ToggleAction{Item: item, State: "on", Evidence: evidence}
	case intent.ActionTurnOff:
		return intent.ToggleAction{Item: item, State: "off", Evidence: evidence}
	case intent.ActionWait:
		return intent.WaitAction{Evidence: evidence}
	case intent.ActionTalk:
		if target.Mention == "" {
			target.Mention = in.Item
		}
		return intent.TalkAction{Target: target, Evidence: evidence}
	case intent.ActionListen:
		return intent.ListenAction{Target: target, Evidence: evidence}
	case intent.ActionExplore:
		return intent.ExploreAction{Target: target, Evidence: evidence}
	default:
		return nil
	}
}
