package response

import (
	"kaya/internal/game"
	"kaya/internal/kaya"
	"kaya/internal/turn"
)

const SystemPrompt = `Write concise first-person dialogue as Dr. Kaya. Preserve action order. Every sentence must cite its supporting fact IDs. Include every required fact. Use only named entities present in the supplied facts. Add no room, exit, item, creature, injury, event, or outcome.`

type ResponseDraft struct {
	Sentences []DraftSentence `json:"sentences"`
}

type DraftSentence struct {
	FactIDs []game.FactID `json:"factIds"`
	Text    string        `json:"text"`
}

var ResponseSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"sentences"},
	"properties": map[string]any{
		"sentences": map[string]any{
			"type": "array", "minItems": 1, "maxItems": 6,
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"factIds", "text"},
				"properties": map[string]any{
					"factIds": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}},
					"text":    map[string]any{"type": "string", "minLength": 1, "maxLength": 300},
				},
			},
		},
	},
}

func responseInput(bundle turn.FactBundle) any {
	return struct {
		PlayerMessage string       `json:"playerMessage"`
		Facts         []game.Fact  `json:"facts"`
		Emotion       kaya.Emotion `json:"emotion"`
	}{bundle.PlayerMessage, bundle.Facts, bundle.Emotion}
}
