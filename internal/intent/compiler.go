package intent

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	Action  int    `json:"action"`
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type actionContract struct {
	required  []string
	forbidden []string
}

var executableActionContracts = map[string]actionContract{
	"move":    {required: []string{"direction"}, forbidden: []string{"targetMention", "itemMention", "state"}},
	"inspect": {forbidden: []string{"itemMention", "direction", "state"}},
	"search":  {required: []string{"targetMention"}, forbidden: []string{"itemMention", "direction", "state"}},
	"take":    {required: []string{"targetMention"}, forbidden: []string{"itemMention", "direction", "state"}},
	"use":     {required: []string{"itemMention", "targetMention"}, forbidden: []string{"direction", "state"}},
	"toggle":  {required: []string{"itemMention", "state"}, forbidden: []string{"targetMention", "direction"}},
	"wait":    {forbidden: []string{"targetMention", "itemMention", "direction", "state"}},
	"talk":    {required: []string{"targetMention"}, forbidden: []string{"itemMention", "direction", "state"}},
	"listen":  {forbidden: []string{"itemMention", "direction", "state"}},
	"explore": {forbidden: []string{"itemMention", "direction", "state"}},
}

func CompileModelPlan(message string, model ModelTurnPlan) (SemanticPlan, []ValidationError) {
	plan := SemanticPlan{
		Actions:               make([]SemanticAction, 0, len(model.Actions)),
		Questions:             compileQuestions(model.Questions),
		RawText:               model.RawText,
		NeedsClarification:    model.NeedsClarification,
		ClarificationQuestion: model.ClarificationQuestion,
	}
	problems := make([]ValidationError, 0)
	normalizedMessage := normalizeEvidence(message)
	claimedEvidence := make(map[string]int, len(model.Actions))

	for index, action := range model.Actions {
		problemStart := len(problems)
		contract, supported := executableActionContracts[action.Kind]
		if !supported {
			problems = append(problems, validationProblem(index, "kind", "unsupported_kind", fmt.Sprintf("action kind %q is not executable", action.Kind)))
		} else {
			problems = append(problems, validateSlots(index, action, contract)...)
			if action.Kind == "toggle" && action.State != "" && action.State != "on" && action.State != "off" {
				problems = append(problems, validationProblem(index, "state", "invalid_value", "toggle state must be on or off"))
			}
		}

		normalizedEvidence := normalizeEvidence(action.Evidence)
		if normalizedEvidence == "" {
			problems = append(problems, validationProblem(index, "evidence", "required_slot", "evidence is required"))
		} else {
			if !strings.Contains(normalizedMessage, normalizedEvidence) {
				problems = append(problems, validationProblem(index, "evidence", "evidence_not_found", "evidence does not occur in the original message"))
			}
			if first, duplicate := claimedEvidence[normalizedEvidence]; duplicate {
				problems = append(problems, validationProblem(index, "evidence", "duplicate_evidence", fmt.Sprintf("evidence duplicates action %d", first)))
			} else {
				claimedEvidence[normalizedEvidence] = index
			}
		}

		if supported && len(problems) == problemStart {
			plan.Actions = append(plan.Actions, compileAction(action))
		}
	}

	return plan, problems
}

func compileQuestions(questions []ModelFactQuestion) []FactQuestion {
	compiled := make([]FactQuestion, 0, len(questions))
	for _, question := range questions {
		compiled = append(compiled, FactQuestion{
			Kind:       question.Kind,
			Target:     question.TargetMention,
			TargetMode: semanticQuantity(question.Quantity),
		})
	}
	return compiled
}

func compileAction(action ModelAction) SemanticAction {
	quantity := semanticQuantity(action.Quantity)
	target := Reference{Mention: action.TargetMention, Quantity: quantity}
	item := Reference{Mention: action.ItemMention, Quantity: quantity}

	switch action.Kind {
	case "move":
		return MoveAction{Direction: action.Direction, Evidence: action.Evidence}
	case "inspect":
		return InspectAction{Target: target, Evidence: action.Evidence}
	case "search":
		return SearchAction{Target: target, Evidence: action.Evidence}
	case "take":
		return TakeAction{Target: target, Evidence: action.Evidence}
	case "use":
		return UseAction{Item: item, Target: target, Evidence: action.Evidence}
	case "toggle":
		return ToggleAction{Item: item, State: action.State, Evidence: action.Evidence}
	case "wait":
		return WaitAction{Evidence: action.Evidence}
	case "talk":
		return TalkAction{Target: target, Evidence: action.Evidence}
	case "listen":
		return ListenAction{Target: target, Evidence: action.Evidence}
	case "explore":
		return ExploreAction{Target: target, Evidence: action.Evidence}
	default:
		panic("compileAction called with unsupported kind")
	}
}

func validateSlots(index int, action ModelAction, contract actionContract) []ValidationError {
	var problems []ValidationError
	for _, field := range contract.required {
		if strings.TrimSpace(modelSlot(action, field)) == "" {
			problems = append(problems, validationProblem(index, field, "required_slot", field+" is required"))
		}
	}
	for _, field := range contract.forbidden {
		if strings.TrimSpace(modelSlot(action, field)) != "" {
			problems = append(problems, validationProblem(index, field, "forbidden_slot", field+" is forbidden"))
		}
	}
	return problems
}

func modelSlot(action ModelAction, field string) string {
	switch field {
	case "targetMention":
		return action.TargetMention
	case "itemMention":
		return action.ItemMention
	case "direction":
		return action.Direction
	case "state":
		return action.State
	default:
		return ""
	}
}

func semanticQuantity(quantity TargetMode) TargetMode {
	if quantity == "" || quantity == TargetSingle {
		return TargetOne
	}
	return quantity
}

func normalizeEvidence(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validationProblem(action int, field, code, message string) ValidationError {
	return ValidationError{Action: action, Field: field, Code: code, Message: message}
}
