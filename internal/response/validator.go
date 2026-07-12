package response

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"kaya/internal/game"
	"kaya/internal/turn"
)

var entityRunPattern = regexp.MustCompile(`\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)+\b`)
var claimTokenPattern = regexp.MustCompile(`[\p{L}\p{N}]+(?:['’][\p{L}]+)?`)

// safeVoiceLexicon contains grammar and ordinary Kaya phrasing that is not a
// world fact by itself. Every other content token must come from an approved
// fact field, keeping prose claims conservative and deterministic.
var safeVoiceLexicon = func() map[string]bool {
	words := strings.Fields(`a an the i me my we our you your it its this that these those they them their
	here there’s there's now i'm i've they're they're
is are was were be been being am can cannot could should have
	no not yes and or but if then as so very still only just all both one first second
	in on at by near beside next to from into of for with without around through inside outside
	to feel along seconds second minute minutes view views include includes exit access take stand where
	passed since arrival observation began observing space`)
	lexicon := make(map[string]bool, len(words))
	for _, word := range words {
		lexicon[word] = true
	}
	return lexicon
}()

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
	if hasUnsupportedClaim(draft, bundle) {
		return ResponseDraft{}, "unsupported_claim"
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

func hasUnsupportedClaim(draft ResponseDraft, bundle turn.FactBundle) bool {
	approved := make(map[string]bool, len(bundle.Facts)*8)
	approvedNumbers := approvedNumberValues(bundle)
	for _, fact := range bundle.Facts {
		for _, field := range []string{fact.Subject, fact.Value, fact.Text} {
			for _, token := range claimTokenPattern.FindAllString(strings.ToLower(field), -1) {
				approved[token] = true
			}
		}
	}
	for _, sentence := range draft.Sentences {
		tokens := claimTokenPattern.FindAllString(strings.ToLower(sentence.Text), -1)
		for index := 0; index < len(tokens); {
			if number, width, ok := parseNumberWords(tokens[index:]); ok && approvedNumbers[number] {
				index += width
				continue
			}
			token := tokens[index]
			if !approved[token] && !safeVoiceLexicon[token] {
				return true
			}
			index++
		}
	}
	return false
}

func approvedNumberValues(bundle turn.FactBundle) map[int]bool {
	approved := make(map[int]bool)
	for _, fact := range bundle.Facts {
		for _, field := range []string{fact.Subject, fact.Value, fact.Text} {
			for _, token := range claimTokenPattern.FindAllString(field, -1) {
				if value, err := strconv.Atoi(token); err == nil {
					approved[value] = true
				}
			}
		}
	}
	return approved
}

var cardinalNumberWords = map[string]int{
	"zero": 0,
	"one":  1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
	"ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19,
	"twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90,
}

func parseNumberWords(tokens []string) (int, int, bool) {
	total, current, width := 0, 0, 0
	for _, token := range tokens {
		if value, ok := cardinalNumberWords[token]; ok {
			current += value
			width++
			continue
		}
		switch token {
		case "hundred":
			if current == 0 {
				current = 1
			}
			current *= 100
			width++
		case "thousand":
			if current == 0 {
				current = 1
			}
			total += current * 1000
			current = 0
			width++
		default:
			if width == 0 {
				return 0, 0, false
			}
			return total + current, width, true
		}
	}
	if width == 0 {
		return 0, 0, false
	}
	return total + current, width, true
}

func approvedEntity(candidate string, approved []string) bool {
	candidate = normalizedEntityWords(candidate)
	for _, field := range approved {
		field = normalizedEntityWords(field)
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

func normalizedEntityWords(value string) string {
	return strings.Join(claimTokenPattern.FindAllString(strings.ToLower(value), -1), " ")
}
