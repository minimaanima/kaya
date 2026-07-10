package turn

import (
	"fmt"
	"strings"

	"kaya/internal/actions"
	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/kaya"
	"kaya/internal/world"
)

const maxPlanEntries = 4

type actionResolver interface {
	Resolve(intent.Intent) game.ActionResult
}

type Executor struct {
	state    *world.State
	resolver actionResolver
}

func NewExecutor(state *world.State) Executor {
	return newExecutor(state, actions.NewResolver(state))
}

func newExecutor(state *world.State, resolver actionResolver) Executor {
	return Executor{state: state, resolver: resolver}
}

func (e Executor) Execute(plan intent.TurnPlan) Result {
	result := Result{Outcomes: []ActionOutcome{}, QuestionFacts: []game.Fact{}}
	if err := validatePlan(plan); err != nil {
		result.StopReason = "invalid_plan"
		result.ClarificationQuestion = err.Error()
		result.Emotion = e.emotion()
		return result
	}
	if plan.NeedsClarification {
		result.StopReason = "clarification"
		result.ClarificationQuestion = strings.TrimSpace(plan.ClarificationQuestion)
		if result.ClarificationQuestion == "" {
			result.ClarificationQuestion = "What do you want Kaya to do?"
		}
		result.Emotion = e.emotion()
		return result
	}
	if e.state == nil || e.resolver == nil {
		result.StopReason = "missing_world"
		result.ClarificationQuestion = "I cannot read the room state right now."
		result.Emotion = e.emotion()
		return result
	}

	stopped := false
	for _, planned := range plan.Actions {
		if stopped {
			break
		}
		if planned.TargetMode == intent.TargetAll && !isObjectAction(planned.Intent.Action) {
			outcome := ActionOutcome{Intent: planned.Intent, Result: clarificationResult("Which single target should I use?")}
			result.Outcomes = append(result.Outcomes, outcome)
			result.StopReason = "clarification"
			result.ClarificationQuestion = outcome.Result.ClarificationQuestion
			stopped = true
			continue
		}

		if isObjectAction(planned.Intent.Action) {
			resolution, err := e.state.ResolveObjectGroup(planned.Intent.Target, planned.TargetMode == intent.TargetAll)
			if err != nil {
				outcome := failedTargetOutcome(planned.Intent, "target_resolution_failed", err.Error())
				result.Outcomes = append(result.Outcomes, outcome)
				result.StopReason = outcome.Result.Outcome
				stopped = true
				continue
			}
			if resolution.Missing() {
				outcome := failedTargetOutcome(planned.Intent, "target_missing", "I cannot see that here.")
				result.Outcomes = append(result.Outcomes, outcome)
				result.StopReason = outcome.Result.Outcome
				stopped = true
				continue
			}
			if resolution.Ambiguous() {
				outcome := failedTargetOutcome(planned.Intent, "target_ambiguous", ambiguityText(resolution))
				result.Outcomes = append(result.Outcomes, outcome)
				result.StopReason = outcome.Result.Outcome
				stopped = true
				continue
			}

			ids := make([]game.ObjectID, 0, len(resolution.Matches))
			for _, object := range resolution.Matches {
				ids = append(ids, object.ID)
			}
			for _, object := range resolution.Matches {
				if stopped {
					break
				}
				expanded := planned.Intent
				expanded.Target = object.Name
				actionResult := e.resolver.Resolve(expanded)
				outcome := ActionOutcome{Intent: expanded, TargetObjectID: object.ID, Result: actionResult}
				result.Outcomes = append(result.Outcomes, outcome)
				if actionResult.Status != game.ActionSucceeded {
					result.StopReason = string(actionResult.Status)
					result.ClarificationQuestion = actionResult.ClarificationQuestion
					stopped = true
				}
			}
			if planned.TargetMode == intent.TargetAll {
				e.state.RememberObjects(ids)
			}
			continue
		}

		actionResult := e.resolver.Resolve(planned.Intent)
		result.Outcomes = append(result.Outcomes, ActionOutcome{Intent: planned.Intent, Result: actionResult})
		if actionResult.Status != game.ActionSucceeded {
			result.StopReason = string(actionResult.Status)
			result.ClarificationQuestion = actionResult.ClarificationQuestion
			stopped = true
		}
	}

	result.QuestionFacts = e.answerQuestions(plan.Questions)
	result.Emotion = e.emotion()
	return result
}

func (e Executor) answerQuestions(questions []intent.FactQuestion) []game.Fact {
	facts := make([]game.Fact, 0)
	if e.state == nil {
		return facts
	}
	for _, question := range questions {
		resolution, err := e.state.ResolveObjectGroup(question.Target, question.TargetMode == intent.TargetAll)
		if err != nil || resolution.Missing() || resolution.Ambiguous() {
			continue
		}
		for _, object := range resolution.Matches {
			fact, ok := e.state.ObservedFact(object.ID, question.Kind)
			if !ok {
				facts = append(facts, game.Fact{Kind: game.FactFailure, Subject: object.Name, Value: "unknown", Text: "I cannot tell whether " + object.Name + " is dead yet.", Required: true})
				continue
			}
			fact.Subject = object.Name
			facts = append(facts, fact)
		}
	}
	return facts
}

func (e Executor) emotion() kaya.Emotion {
	if e.state == nil {
		return ""
	}
	return e.state.Kaya.DominantEmotion()
}

func validatePlan(plan intent.TurnPlan) error {
	if len(plan.Actions) > maxPlanEntries || len(plan.Questions) > maxPlanEntries {
		return fmt.Errorf("turn plan exceeds %d entries", maxPlanEntries)
	}
	for i, planned := range plan.Actions {
		if !planned.Intent.Action.Valid() {
			return fmt.Errorf("invalid action %q", planned.Intent.Action)
		}
		if planned.TargetMode != intent.TargetSingle && planned.TargetMode != intent.TargetAll {
			return fmt.Errorf("invalid target mode %q for action %d", planned.TargetMode, i)
		}
	}
	for i, question := range plan.Questions {
		if question.Kind != game.FactLifeStatus {
			return fmt.Errorf("invalid question kind %q", question.Kind)
		}
		if question.TargetMode != intent.TargetSingle && question.TargetMode != intent.TargetAll {
			return fmt.Errorf("invalid target mode %q for question %d", question.TargetMode, i)
		}
	}
	return nil
}

func isObjectAction(action intent.Action) bool {
	return action == intent.ActionInspect || action == intent.ActionSearch
}

func failedTargetOutcome(in intent.Intent, outcome, text string) ActionOutcome {
	return ActionOutcome{Intent: in, Result: game.ActionResult{Status: game.ActionFailed, Outcome: outcome, VisibleFacts: []game.Fact{{Kind: game.FactFailure, Subject: "action", Value: outcome, Text: text, Required: true}}}}
}

func clarificationResult(question string) game.ActionResult {
	return game.ActionResult{Status: game.ActionClarification, Outcome: "needs_clarification", NeedsClarification: true, ClarificationQuestion: question, VisibleFacts: []game.Fact{{Kind: game.FactClarification, Subject: "action", Value: "needs_clarification", Text: question, Required: true}}}
}

func ambiguityText(resolution world.ObjectResolution) string {
	names := make([]string, 0, len(resolution.Matches))
	for _, object := range resolution.Matches {
		names = append(names, object.Name)
	}
	return "Which one do you mean: " + strings.Join(names, ", ") + "?"
}
