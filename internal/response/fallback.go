package response

import (
	"strings"

	"kaya/internal/game"
	"kaya/internal/turn"
)

// Fallback renders required facts in bundle order without inventing content.
type Fallback struct{}

func (Fallback) Render(bundle turn.FactBundle) string {
	return renderSentences((Fallback{}).Sentences(bundle))
}

// Sentences returns deterministic one-fact evidence in bundle order.
func (Fallback) Sentences(bundle turn.FactBundle) []ResponseSentence {
	seen := make(map[game.FactID]bool)
	sentences := make([]ResponseSentence, 0, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		text := strings.TrimSpace(fact.Text)
		if !fact.Required || text == "" || seen[fact.ID] {
			continue
		}
		seen[fact.ID] = true
		sentences = append(sentences, ResponseSentence{Text: ensureSentence(text), FactIDs: []game.FactID{fact.ID}})
	}
	if len(sentences) == 0 {
		return []ResponseSentence{{Text: "What do you want me to do?"}}
	}
	return sentences
}

func renderSentences(sentences []ResponseSentence) string {
	parts := make([]string, len(sentences))
	for index, sentence := range sentences {
		parts[index] = strings.TrimSpace(sentence.Text)
	}
	return strings.Join(parts, " ")
}

func ensureSentence(text string) string {
	if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "!") || strings.HasSuffix(text, "?") {
		return text
	}
	return text + "."
}
