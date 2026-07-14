package intent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"kaya/internal/game"
)

const TargetOne TargetMode = "one"

type ModelAction struct {
	Kind          string     `json:"kind"`
	TargetMention string     `json:"targetMention"`
	ItemMention   string     `json:"itemMention"`
	Direction     string     `json:"direction"`
	State         string     `json:"state"`
	Evidence      string     `json:"evidence"`
	Quantity      TargetMode `json:"quantity"`
}

type ModelFactQuestion struct {
	Kind          game.FactKind `json:"kind"`
	TargetMention string        `json:"targetMention"`
	Quantity      TargetMode    `json:"quantity"`
}

type ModelTurnPlan struct {
	Actions               []ModelAction       `json:"actions"`
	Questions             []ModelFactQuestion `json:"questions"`
	RawText               string              `json:"rawText"`
	NeedsClarification    bool                `json:"needsClarification"`
	ClarificationQuestion string              `json:"clarificationQuestion"`
}

type modelTurnPlanJSON struct {
	Actions               *[]*modelActionJSON       `json:"actions"`
	Questions             *[]*modelFactQuestionJSON `json:"questions"`
	RawText               *string                   `json:"rawText"`
	NeedsClarification    *bool                     `json:"needsClarification"`
	ClarificationQuestion *string                   `json:"clarificationQuestion"`
}

type modelActionJSON struct {
	Kind          *string     `json:"kind"`
	TargetMention *string     `json:"targetMention"`
	ItemMention   *string     `json:"itemMention"`
	Direction     *string     `json:"direction"`
	State         *string     `json:"state"`
	Evidence      *string     `json:"evidence"`
	Quantity      *TargetMode `json:"quantity"`
}

type modelFactQuestionJSON struct {
	Kind          *game.FactKind `json:"kind"`
	TargetMention *string        `json:"targetMention"`
	Quantity      *TargetMode    `json:"quantity"`
}

func ParseModelPlanJSON(raw string) (ModelTurnPlan, error) {
	var wire modelTurnPlanJSON
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return ModelTurnPlan{}, fmt.Errorf("decode model turn plan: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return ModelTurnPlan{}, fmt.Errorf("decode model turn plan: trailing JSON value")
		}
		return ModelTurnPlan{}, fmt.Errorf("decode model turn plan trailing data: %w", err)
	}
	plan, err := wire.modelTurnPlan()
	if err != nil {
		return ModelTurnPlan{}, fmt.Errorf("validate model turn plan: %w", err)
	}
	return plan, nil
}

func (wire modelTurnPlanJSON) modelTurnPlan() (ModelTurnPlan, error) {
	if wire.Actions == nil {
		return ModelTurnPlan{}, fmt.Errorf("actions is required and must not be null")
	}
	if wire.Questions == nil {
		return ModelTurnPlan{}, fmt.Errorf("questions is required and must not be null")
	}
	if wire.RawText == nil {
		return ModelTurnPlan{}, fmt.Errorf("rawText is required and must not be null")
	}
	if wire.NeedsClarification == nil {
		return ModelTurnPlan{}, fmt.Errorf("needsClarification is required and must not be null")
	}
	if wire.ClarificationQuestion == nil {
		return ModelTurnPlan{}, fmt.Errorf("clarificationQuestion is required and must not be null")
	}
	if len(*wire.Actions) > 4 {
		return ModelTurnPlan{}, fmt.Errorf("actions has %d items; maximum is 4", len(*wire.Actions))
	}
	if len(*wire.Questions) > 4 {
		return ModelTurnPlan{}, fmt.Errorf("questions has %d items; maximum is 4", len(*wire.Questions))
	}

	plan := ModelTurnPlan{
		Actions:               make([]ModelAction, 0, len(*wire.Actions)),
		Questions:             make([]ModelFactQuestion, 0, len(*wire.Questions)),
		RawText:               *wire.RawText,
		NeedsClarification:    *wire.NeedsClarification,
		ClarificationQuestion: *wire.ClarificationQuestion,
	}
	for index, action := range *wire.Actions {
		if action == nil {
			return ModelTurnPlan{}, fmt.Errorf("actions[%d] must not be null", index)
		}
		compiled, err := action.modelAction()
		if err != nil {
			return ModelTurnPlan{}, fmt.Errorf("actions[%d]: %w", index, err)
		}
		plan.Actions = append(plan.Actions, compiled)
	}
	for index, question := range *wire.Questions {
		if question == nil {
			return ModelTurnPlan{}, fmt.Errorf("questions[%d] must not be null", index)
		}
		compiled, err := question.modelFactQuestion()
		if err != nil {
			return ModelTurnPlan{}, fmt.Errorf("questions[%d]: %w", index, err)
		}
		plan.Questions = append(plan.Questions, compiled)
	}
	return plan, nil
}

func (wire modelActionJSON) modelAction() (ModelAction, error) {
	if wire.Kind == nil {
		return ModelAction{}, fmt.Errorf("kind is required and must not be null")
	}
	if wire.TargetMention == nil {
		return ModelAction{}, fmt.Errorf("targetMention is required and must not be null")
	}
	if wire.ItemMention == nil {
		return ModelAction{}, fmt.Errorf("itemMention is required and must not be null")
	}
	if wire.Direction == nil {
		return ModelAction{}, fmt.Errorf("direction is required and must not be null")
	}
	if wire.State == nil {
		return ModelAction{}, fmt.Errorf("state is required and must not be null")
	}
	if wire.Evidence == nil {
		return ModelAction{}, fmt.Errorf("evidence is required and must not be null")
	}
	if wire.Quantity == nil {
		return ModelAction{}, fmt.Errorf("quantity is required and must not be null")
	}
	if !validModelActionKind(*wire.Kind) {
		return ModelAction{}, fmt.Errorf("kind %q is not supported", *wire.Kind)
	}
	if *wire.State != "" && *wire.State != "on" && *wire.State != "off" {
		return ModelAction{}, fmt.Errorf("state %q is not valid", *wire.State)
	}
	if !validModelQuantity(*wire.Quantity) {
		return ModelAction{}, fmt.Errorf("quantity %q is not valid", *wire.Quantity)
	}
	return ModelAction{
		Kind:          *wire.Kind,
		TargetMention: *wire.TargetMention,
		ItemMention:   *wire.ItemMention,
		Direction:     *wire.Direction,
		State:         *wire.State,
		Evidence:      *wire.Evidence,
		Quantity:      *wire.Quantity,
	}, nil
}

func (wire modelFactQuestionJSON) modelFactQuestion() (ModelFactQuestion, error) {
	if wire.Kind == nil {
		return ModelFactQuestion{}, fmt.Errorf("kind is required and must not be null")
	}
	if wire.TargetMention == nil {
		return ModelFactQuestion{}, fmt.Errorf("targetMention is required and must not be null")
	}
	if wire.Quantity == nil {
		return ModelFactQuestion{}, fmt.Errorf("quantity is required and must not be null")
	}
	if *wire.Kind != game.FactLifeStatus {
		return ModelFactQuestion{}, fmt.Errorf("kind %q is not supported", *wire.Kind)
	}
	if strings.TrimSpace(*wire.TargetMention) == "" {
		return ModelFactQuestion{}, fmt.Errorf("targetMention must not be empty")
	}
	if !validModelQuantity(*wire.Quantity) {
		return ModelFactQuestion{}, fmt.Errorf("quantity %q is not valid", *wire.Quantity)
	}
	return ModelFactQuestion{
		Kind:          *wire.Kind,
		TargetMention: *wire.TargetMention,
		Quantity:      *wire.Quantity,
	}, nil
}

func validModelActionKind(kind string) bool {
	switch kind {
	case "move", "inspect", "search", "take", "use", "toggle", "wait", "talk", "listen", "explore":
		return true
	default:
		return false
	}
}

func validModelQuantity(quantity TargetMode) bool {
	return quantity == TargetOne || quantity == TargetAll
}
