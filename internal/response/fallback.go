package response

import (
	"strings"

	"kaya/internal/game"
	"kaya/internal/turn"
)

// Fallback renders required facts in bundle order without inventing content.
type Fallback struct{}

func (Fallback) Render(bundle turn.FactBundle) string {
	seen := make(map[game.FactID]bool)
	parts := make([]string, 0, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		text := strings.TrimSpace(fact.Text)
		if !fact.Required || text == "" || seen[fact.ID] {
			continue
		}
		seen[fact.ID] = true
		parts = append(parts, ensureSentence(text))
	}
	if len(parts) == 0 {
		return "What do you want me to do?"
	}
	return strings.Join(parts, " ")
}

func ensureSentence(text string) string {
	if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "!") || strings.HasSuffix(text, "?") {
		return text
	}
	return text + "."
}
