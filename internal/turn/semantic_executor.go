package turn

import (
	"fmt"
	"reflect"
	"strings"

	"kaya/internal/game"
	"kaya/internal/grounding"
	"kaya/internal/intent"
)

func (e Executor) ExecuteSemantic(plan intent.SemanticPlan, start int, binding *grounding.Binding) SemanticExecution {
	execution := SemanticExecution{Result: Result{Outcomes: []ActionOutcome{}, QuestionFacts: []game.Fact{}}}
	if err := validateSemanticPlan(plan, start); err != nil {
		execution.Result.StopReason = "invalid_plan"
		execution.Result.ClarificationQuestion = err.Error()
		execution.Result.Emotion = e.emotion()
		return execution
	}
	if plan.NeedsClarification {
		execution.Result.StopReason = "clarification"
		execution.Result.ClarificationQuestion = strings.TrimSpace(plan.ClarificationQuestion)
		if execution.Result.ClarificationQuestion == "" {
			execution.Result.ClarificationQuestion = "What do you want Kaya to do?"
		}
		execution.Result.Emotion = e.emotion()
		return execution
	}
	if len(plan.Actions) == 0 && len(plan.Questions) == 0 {
		execution.Result.StopReason = "clarification"
		execution.Result.ClarificationQuestion = "What do you want Kaya to do?"
		execution.Result.Emotion = e.emotion()
		return execution
	}
	if e.state == nil || e.resolver == nil {
		execution.Result.StopReason = "missing_world"
		execution.Result.ClarificationQuestion = "I cannot read the room state right now."
		execution.Result.Emotion = e.emotion()
		return execution
	}

	for index := start; index < len(plan.Actions); index++ {
		action := plan.Actions[index]
		currentBinding := binding
		binding = nil
		grounded := grounding.New(e.state).Ground(action, currentBinding)
		if grounded.Clarification != nil {
			execution.Pending = pendingSemanticAction(plan, index, grounded.Clarification)
			execution.Result.StopReason = "clarification"
			execution.Result.ClarificationQuestion = semanticClarificationQuestion(grounded.Clarification)
			execution.Result.Emotion = e.emotion()
			return execution
		}
		if grounded.Err != nil || grounded.Missing != nil {
			outcome := semanticGroundingFailure(action, plan.RawText, grounded)
			execution.Result.Outcomes = append(execution.Result.Outcomes, outcome)
			execution.Result.StopReason = outcome.Result.Outcome
			execution.Result.Emotion = e.emotion()
			return execution
		}

		selections := groundedSelections(grounded.References)
		for _, selection := range selections {
			in := semanticResolverIntent(action, plan.RawText, selection)
			resolved := e.resolver.Resolve(in)
			outcome := ActionOutcome{
				Intent:         in,
				TargetObjectID: selectedObjectID(selection),
				Result:         resolved,
			}
			execution.Result.Outcomes = append(execution.Result.Outcomes, outcome)
			if resolved.Status != game.ActionSucceeded {
				execution.Result.StopReason = string(resolved.Status)
				execution.Result.ClarificationQuestion = resolved.ClarificationQuestion
				execution.Result.Emotion = e.emotion()
				return execution
			}
		}
		if objectIDs := selectedObjectIDs(selections); len(objectIDs) > 1 {
			e.state.RememberObjects(objectIDs)
		}
	}

	var questionClarification string
	execution.Result.QuestionFacts, questionClarification = e.answerQuestions(plan.Questions)
	if questionClarification != "" {
		execution.Result.StopReason = "clarification"
		execution.Result.ClarificationQuestion = questionClarification
	}
	execution.Result.Emotion = e.emotion()
	return execution
}

func validateSemanticPlan(plan intent.SemanticPlan, start int) error {
	if len(plan.Actions) > maxPlanEntries || len(plan.Questions) > maxPlanEntries {
		return fmt.Errorf("semantic plan exceeds %d entries", maxPlanEntries)
	}
	if start < 0 || start > len(plan.Actions) {
		return fmt.Errorf("semantic action start %d is out of range", start)
	}
	for index, action := range plan.Actions {
		value := reflect.ValueOf(action)
		if !value.IsValid() || value.Kind() == reflect.Pointer && value.IsNil() {
			return fmt.Errorf("semantic action %d is nil", index)
		}
		if !action.ActionKind().Valid() || action.ActionKind() == intent.ActionUnknown {
			return fmt.Errorf("semantic action %d has invalid kind %q", index, action.ActionKind())
		}
	}
	return nil
}

func pendingSemanticAction(plan intent.SemanticPlan, index int, clarification *grounding.Clarification) *PendingSemanticAction {
	remaining := plan
	remaining.Actions = append([]intent.SemanticAction(nil), plan.Actions[index:]...)
	remaining.Questions = append([]intent.FactQuestion(nil), plan.Questions...)
	return &PendingSemanticAction{
		ActionIndex:   index,
		Role:          clarification.Role,
		Candidates:    cloneGroundingCandidates(clarification.Candidates),
		RemainingPlan: remaining,
	}
}

func semanticClarificationQuestion(clarification *grounding.Clarification) string {
	names := make([]string, 0, len(clarification.Candidates))
	for _, candidate := range clarification.Candidates {
		names = append(names, candidate.Name)
	}
	if len(names) == 0 {
		return "Which one do you mean?"
	}
	return "Which one do you mean: " + strings.Join(names, ", ") + "?"
}

func cloneGroundingCandidates(candidates []grounding.Candidate) []grounding.Candidate {
	cloned := make([]grounding.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Aliases = append([]string(nil), candidate.Aliases...)
		cloned = append(cloned, candidate)
	}
	return cloned
}

func groundedSelections(references []grounding.GroundedReference) []map[grounding.Role]grounding.Candidate {
	selections := []map[grounding.Role]grounding.Candidate{{}}
	for _, reference := range references {
		if len(reference.Candidates) == 0 {
			continue
		}
		next := make([]map[grounding.Role]grounding.Candidate, 0, len(selections)*len(reference.Candidates))
		for _, selection := range selections {
			for _, candidate := range reference.Candidates {
				expanded := make(map[grounding.Role]grounding.Candidate, len(selection)+1)
				for role, selected := range selection {
					expanded[role] = selected
				}
				expanded[reference.Role] = candidate
				next = append(next, expanded)
			}
		}
		selections = next
	}
	return selections
}

func semanticResolverIntent(action intent.SemanticAction, rawText string, selection map[grounding.Role]grounding.Candidate) intent.Intent {
	in := intent.Intent{Action: action.ActionKind(), RawText: rawText}
	switch typed := action.(type) {
	case intent.MoveAction:
		in.Direction = selectedID(selection, grounding.RoleExit, typed.Direction)
	case *intent.MoveAction:
		in.Direction = selectedID(selection, grounding.RoleExit, typed.Direction)
	case intent.InspectAction:
		in.Target = selectedID(selection, grounding.RoleObject, typed.Target.Mention)
	case *intent.InspectAction:
		in.Target = selectedID(selection, grounding.RoleObject, typed.Target.Mention)
	case intent.SearchAction:
		in.Target = selectedID(selection, grounding.RoleObject, typed.Target.Mention)
	case *intent.SearchAction:
		in.Target = selectedID(selection, grounding.RoleObject, typed.Target.Mention)
	case intent.TakeAction:
		in.Target = selectedID(selection, grounding.RoleItem, typed.Target.Mention)
	case *intent.TakeAction:
		in.Target = selectedID(selection, grounding.RoleItem, typed.Target.Mention)
	case intent.UseAction:
		in.Item = selectedID(selection, grounding.RoleItem, typed.Item.Mention)
		in.Target = selectedID(selection, grounding.RoleDoor, typed.Target.Mention)
	case *intent.UseAction:
		in.Item = selectedID(selection, grounding.RoleItem, typed.Item.Mention)
		in.Target = selectedID(selection, grounding.RoleDoor, typed.Target.Mention)
	case intent.ToggleAction:
		in.Item = selectedID(selection, grounding.RoleItem, typed.Item.Mention)
	case *intent.ToggleAction:
		in.Item = selectedID(selection, grounding.RoleItem, typed.Item.Mention)
	case intent.TalkAction:
		in.Target = selectedID(selection, grounding.RoleObject, typed.Target.Mention)
	case *intent.TalkAction:
		in.Target = selectedID(selection, grounding.RoleObject, typed.Target.Mention)
	case intent.ListenAction:
		in.Target = selectedID(selection, grounding.RoleDoor, typed.Target.Mention)
	case *intent.ListenAction:
		in.Target = selectedID(selection, grounding.RoleDoor, typed.Target.Mention)
	}
	return in
}

func selectedID(selection map[grounding.Role]grounding.Candidate, role grounding.Role, fallback string) string {
	if candidate, ok := selection[role]; ok {
		return candidate.ID
	}
	return fallback
}

func selectedObjectID(selection map[grounding.Role]grounding.Candidate) game.ObjectID {
	return game.ObjectID(selectedID(selection, grounding.RoleObject, ""))
}

func selectedObjectIDs(selections []map[grounding.Role]grounding.Candidate) []game.ObjectID {
	seen := make(map[game.ObjectID]bool, len(selections))
	ids := make([]game.ObjectID, 0, len(selections))
	for _, selection := range selections {
		id := selectedObjectID(selection)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func semanticGroundingFailure(action intent.SemanticAction, rawText string, grounded grounding.Result) ActionOutcome {
	in := semanticResolverIntent(action, rawText, nil)
	outcome := "grounding_failed"
	text := "I cannot resolve that reference here."
	if grounded.Missing != nil {
		outcome = string(grounded.Missing.Reason)
	}
	if grounded.Err != nil {
		text = grounded.Err.Error()
	}
	return ActionOutcome{Intent: in, Result: game.ActionResult{
		Status:  game.ActionFailed,
		Outcome: outcome,
		VisibleFacts: []game.Fact{{
			ID:       game.FactID(outcome),
			Kind:     game.FactFailure,
			Subject:  "action",
			Value:    outcome,
			Text:     text,
			Required: true,
		}},
	}}
}
