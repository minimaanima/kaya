package intent

import "kaya/internal/game"

type TargetMode string

const (
	TargetSingle TargetMode = "single"
	TargetAll    TargetMode = "all"
)

type PlannedAction struct {
	Intent     Intent     `json:"intent"`
	TargetMode TargetMode `json:"targetMode"`
}

type FactQuestion struct {
	Kind       game.FactKind `json:"kind"`
	Target     string        `json:"target"`
	TargetMode TargetMode    `json:"targetMode"`
}

type TurnPlan struct {
	Actions               []PlannedAction `json:"actions"`
	Questions             []FactQuestion  `json:"questions"`
	Confidence            float64         `json:"confidence"`
	NeedsClarification    bool            `json:"needsClarification"`
	ClarificationQuestion string          `json:"clarificationQuestion"`
	RawText               string          `json:"rawText"`
}
