package response

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"kaya/internal/game"
	"kaya/internal/turn"
)

var entityRunPattern = regexp.MustCompile(`\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)+\b`)

func validateDraft(raw string, bundle turn.FactBundle) (ResponseDraft, string) {
	var draft ResponseDraft
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		return ResponseDraft{}, "invalid_draft"
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return ResponseDraft{}, "invalid_draft"
	} else if !errors.Is(err, io.EOF) {
		return ResponseDraft{}, "invalid_draft"
	}

	if len(draft.Sentences) == 0 || len(draft.Sentences) > 6 {
		return ResponseDraft{}, "invalid_draft"
	}
	total := 0
	known := make(map[game.FactID]bool, len(bundle.Facts))
	required := make(map[game.FactID]bool)
	for _, fact := range bundle.Facts {
		known[fact.ID] = true
		if fact.Required {
			required[fact.ID] = true
		}
	}
	covered := make(map[game.FactID]bool)
	for _, sentence := range draft.Sentences {
		text := strings.TrimSpace(sentence.Text)
		if len(sentence.FactIDs) == 0 || text == "" || utf8.RuneCountInString(text) > 300 {
			return ResponseDraft{}, "invalid_draft"
		}
		total += utf8.RuneCountInString(text)
		for _, id := range sentence.FactIDs {
			if !known[id] {
				return ResponseDraft{}, "unknown_fact_id"
			}
			covered[id] = true
		}
	}
	if total > 900 {
		return ResponseDraft{}, "invalid_draft"
	}
	for id := range required {
		if !covered[id] {
			return ResponseDraft{}, "missing_required_fact"
		}
	}
	if hasUnknownEntity(draft, bundle) {
		return ResponseDraft{}, "unknown_entity"
	}
	return draft, ""
}

func hasUnknownEntity(draft ResponseDraft, bundle turn.FactBundle) bool {
	approved := make([]string, 0, len(bundle.Facts)*3)
	for _, fact := range bundle.Facts {
		approved = append(approved, fact.Subject, fact.Value, fact.Text)
	}
	for _, sentence := range draft.Sentences {
		for _, candidate := range entityRunPattern.FindAllString(sentence.Text, -1) {
			if !approvedEntity(candidate, approved) {
				return true
			}
		}
	}
	return false
}

func approvedEntity(candidate string, approved []string) bool {
	candidate = strings.ToLower(strings.Join(strings.Fields(candidate), " "))
	for _, field := range approved {
		field = strings.ToLower(strings.Join(strings.Fields(field), " "))
		if strings.Contains(" "+field+" ", " "+candidate+" ") {
			return true
		}
		for _, article := range []string{"the ", "a ", "an "} {
			if strings.HasPrefix(candidate, article) && strings.Contains(" "+field+" ", " "+strings.TrimPrefix(candidate, article)+" ") {
				return true
			}
		}
	}
	return false
}
