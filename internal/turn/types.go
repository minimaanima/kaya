package turn

import (
	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/kaya"
)

type ActionOutcome struct {
	Intent         intent.Intent
	TargetObjectID game.ObjectID
	Result         game.ActionResult
}

type Result struct {
	Outcomes              []ActionOutcome
	QuestionFacts         []game.Fact
	StopReason            string
	ClarificationQuestion string
	Emotion               kaya.Emotion
}

type FactBundle struct {
	PlayerMessage string
	Outcomes      []ActionOutcome
	Facts         []game.Fact
	Emotion       kaya.Emotion
}
