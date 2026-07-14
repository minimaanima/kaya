package turn

import (
	"fmt"
	"reflect"
	"strings"

	"kaya/internal/actions"
	"kaya/internal/game"
	"kaya/internal/grounding"
	"kaya/internal/intent"
	"kaya/internal/world"
)

const persistedGroundingPrefix = "\x00grounded:"

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

actionsLoop:
	for index := start; index < len(plan.Actions); index++ {
		action := plan.Actions[index]
		currentBinding := binding
		binding = nil
		grounded := groundSemanticAction(e.state, action, currentBinding)
		if grounded.Clarification != nil {
			execution.Pending = pendingSemanticAction(plan, index, grounded.Clarification, grounded.References)
			execution.Result.StopReason = "clarification"
			execution.Result.ClarificationQuestion = semanticClarificationQuestion(grounded.Clarification)
			execution.Result.Emotion = e.emotion()
			return execution
		}
		if grounded.Err != nil || grounded.Missing != nil {
			outcome := semanticGroundingFailure(action, plan.RawText, grounded)
			execution.Result.Outcomes = append(execution.Result.Outcomes, outcome)
			execution.Result.StopReason = outcome.Result.Outcome
			break actionsLoop
		}

		selections := groundedSelections(grounded.References)
		for _, selection := range selections {
			in := semanticResolverIntent(action, plan.RawText, selection)
			resolved := resolveGroundedAction(e.resolver, in, selection)
			outcome := ActionOutcome{
				Intent:         in,
				TargetObjectID: selectedObjectID(selection),
				Result:         resolved,
			}
			execution.Result.Outcomes = append(execution.Result.Outcomes, outcome)
			if resolved.Status != game.ActionSucceeded {
				execution.Result.StopReason = string(resolved.Status)
				execution.Result.ClarificationQuestion = resolved.ClarificationQuestion
				break actionsLoop
			}
		}
		if objectIDs := selectedObjectIDs(selections); len(objectIDs) > 1 {
			e.state.RememberObjects(objectIDs)
		}
		if itemIDs := selectedItemIDs(selections); len(itemIDs) > 1 {
			e.state.RememberItems(itemIDs)
		}
	}

	var questionClarification string
	execution.Result.QuestionFacts, questionClarification = e.answerQuestions(plan.Questions)
	if questionClarification != "" {
		execution.Result.ClarificationQuestion = questionClarification
		if execution.Result.StopReason == "" {
			execution.Result.StopReason = "clarification"
		}
	}
	execution.Result.Emotion = e.emotion()
	return execution
}

type groundedActionResolver interface {
	ResolveGrounded(intent.Intent, actions.GroundedSelection) game.ActionResult
}

func resolveGroundedAction(resolver actionResolver, in intent.Intent, selection map[grounding.Role]grounding.Candidate) game.ActionResult {
	groundedResolver, ok := resolver.(groundedActionResolver)
	if !ok {
		return resolver.Resolve(in)
	}
	return groundedResolver.ResolveGrounded(in, actions.GroundedSelection{
		ObjectID: game.ObjectID(selectedID(selection, grounding.RoleObject, "")),
		ItemID:   game.ItemID(selectedID(selection, grounding.RoleItem, "")),
		DoorID:   game.DoorID(selectedID(selection, grounding.RoleDoor, "")),
	})
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

func pendingSemanticAction(plan intent.SemanticPlan, index int, clarification *grounding.Clarification, references []grounding.GroundedReference) *PendingSemanticAction {
	remaining := plan
	remaining.Actions = append([]intent.SemanticAction(nil), plan.Actions[index:]...)
	remaining.Actions[0] = persistGroundedReferences(remaining.Actions[0], references)
	remaining.Questions = append([]intent.FactQuestion(nil), plan.Questions...)
	return &PendingSemanticAction{
		ActionIndex:   index,
		Role:          clarification.Role,
		Candidates:    cloneGroundingCandidates(clarification.Candidates),
		RemainingPlan: remaining,
	}
}

func groundSemanticAction(state *world.State, action intent.SemanticAction, binding *grounding.Binding) grounding.Result {
	grounder := grounding.New(state)
	switch typed := action.(type) {
	case intent.UseAction:
		return groundUseAction(grounder, action, typed.Item, typed.Target, binding)
	case *intent.UseAction:
		return groundUseAction(grounder, action, typed.Item, typed.Target, binding)
	default:
		return grounder.Ground(action, binding)
	}
}

func groundUseAction(grounder grounding.Grounder, action intent.SemanticAction, item, target intent.Reference, binding *grounding.Binding) grounding.Result {
	if binding != nil && binding.Role != grounding.RoleItem && binding.Role != grounding.RoleDoor {
		return grounder.Ground(action, binding)
	}
	result := grounding.Result{Action: action}
	itemResult := grounder.Ground(intent.ToggleAction{Item: item, State: "on"}, referenceBinding(item, grounding.RoleItem, binding))
	result.References = append(result.References, itemResult.References...)
	if copyGroundingStop(&result, itemResult) {
		return result
	}
	doorResult := grounder.Ground(intent.ListenAction{Target: target}, referenceBinding(target, grounding.RoleDoor, binding))
	result.References = append(result.References, doorResult.References...)
	copyGroundingStop(&result, doorResult)
	return result
}

func referenceBinding(reference intent.Reference, role grounding.Role, supplied *grounding.Binding) *grounding.Binding {
	if supplied != nil && supplied.Role == role {
		return supplied
	}
	prefix := persistedGroundingPrefix + string(role) + ":"
	if !strings.HasPrefix(reference.Mention, prefix) {
		return nil
	}
	return &grounding.Binding{Role: role, CandidateIDs: []string{strings.TrimPrefix(reference.Mention, prefix)}}
}

func copyGroundingStop(target *grounding.Result, source grounding.Result) bool {
	target.Clarification = source.Clarification
	target.Missing = source.Missing
	target.Err = source.Err
	return !source.Ready()
}

func persistGroundedReferences(action intent.SemanticAction, references []grounding.GroundedReference) intent.SemanticAction {
	setReference := func(reference intent.Reference, role grounding.Role) intent.Reference {
		for _, grounded := range references {
			if grounded.Role == role && len(grounded.Candidates) == 1 {
				reference.Mention = persistedGroundingPrefix + string(role) + ":" + grounded.Candidates[0].ID
				reference.Quantity = intent.TargetOne
			}
		}
		return reference
	}
	switch typed := action.(type) {
	case intent.UseAction:
		typed.Item = setReference(typed.Item, grounding.RoleItem)
		typed.Target = setReference(typed.Target, grounding.RoleDoor)
		return typed
	case *intent.UseAction:
		copy := *typed
		copy.Item = setReference(copy.Item, grounding.RoleItem)
		copy.Target = setReference(copy.Target, grounding.RoleDoor)
		return &copy
	default:
		return action
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

func selectedItemIDs(selections []map[grounding.Role]grounding.Candidate) []game.ItemID {
	seen := make(map[game.ItemID]bool, len(selections))
	ids := make([]game.ItemID, 0, len(selections))
	for _, selection := range selections {
		id := game.ItemID(selectedID(selection, grounding.RoleItem, ""))
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
