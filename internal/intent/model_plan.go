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

func ParseModelPlanJSON(raw string) (ModelTurnPlan, error) {
	var plan ModelTurnPlan
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return ModelTurnPlan{}, fmt.Errorf("decode model turn plan: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return ModelTurnPlan{}, fmt.Errorf("decode model turn plan: trailing JSON value")
		}
		return ModelTurnPlan{}, fmt.Errorf("decode model turn plan trailing data: %w", err)
	}
	return plan, nil
}
