package response

import (
	"kaya/internal/game"
	"kaya/internal/kaya"
	"kaya/internal/turn"
)

const SystemPrompt = `Write concise first-person dialogue as Dr. Kaya. Preserve action order. Every sentence must cite its supporting fact IDs. Include every required fact. Use only named entities present in the supplied facts. Add no room, exit, item, creature, injury, event, or outcome.`

const RepairSystemPrompt = `Repair one response draft into JSON matching the supplied schema. Output exactly one sentence per requiredFacts entry, in requiredFacts order. Each sentence must cite only that entry's ID and copy its supplied Text exactly. Add, remove, or rephrase no words. Optional facts may be omitted. Return JSON only.`

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

type repairFact struct {
	ID   game.FactID `json:"id"`
	Text string      `json:"text"`
}

func responseRepairInput(bundle turn.FactBundle, originalDraft, validationReason string) any {
	required := make([]repairFact, 0, len(bundle.Facts))
	optional := make([]game.Fact, 0, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		if fact.Required {
			required = append(required, repairFact{ID: fact.ID, Text: fact.Text})
			continue
		}
		optional = append(optional, fact)
	}
	return struct {
		RequiredFacts    []repairFact `json:"requiredFacts"`
		OptionalFacts    []game.Fact  `json:"optionalFacts"`
		OriginalDraft    string       `json:"originalDraft"`
		ValidationReason string       `json:"validationReason"`
	}{required, optional, originalDraft, validationReason}
}
