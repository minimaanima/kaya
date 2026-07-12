package playtest

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/turn"
	"kaya/internal/world"
)

var (
	kayaResponsePrefix  = regexp.MustCompile(`(?i)^kaya\b`)
	debugResponseMarker = regexp.MustCompile(`(?i)\bdebug\s*:`)
)

// CheckResponse verifies response provenance and player-visible safety invariants.
func CheckResponse(step Step, state *world.State) []Violation {
	reply := step.Turn.Response
	text := strings.TrimSpace(reply.Text)
	violations := make([]Violation, 0)

	if kayaResponsePrefix.MatchString(text) {
		violations = append(violations, responseViolation("response_kaya_prefix", text, "response starts with Kaya"))
	}
	if !reply.UsedFallback {
		for _, factID := range ungroundedFactIDs(step, reply.UsedFactIDs) {
			violations = append(violations, responseViolation("response_fact_id_ungrounded", text, fmt.Sprintf("used fact ID %q is absent from the stored turn fact bundle", factID)))
		}
	}
	if isClarificationTurn(step.Turn.Result) && step.After.Time != step.Before.Time {
		violations = append(violations, responseViolation("response_clarification_advanced_time", text, fmt.Sprintf("clarification advanced time from %d to %d", step.Before.Time, step.After.Time)))
	}
	if leaksPitchBlackRoomAwareness(step, state, text) {
		violations = append(violations, responseViolation("response_darkness_leak", text, "pitch-black room awareness names a hidden object or direction"))
	}
	if debugResponseMarker.MatchString(text) {
		violations = append(violations, responseViolation("response_debug_marker", text, "response contains a debug marker"))
	}
	return sortViolations(violations)
}

func responseViolation(code, text, detail string) Violation {
	return Violation{Code: code, Detail: fmt.Sprintf("%s; response=%q", detail, text)}
}

func ungroundedFactIDs(step Step, used []game.FactID) []game.FactID {
	bundle := step.Turn.Result.FactBundle(step.Player)
	allowed := make(map[game.FactID]bool, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		allowed[fact.ID] = true
	}

	missing := make([]game.FactID, 0)
	for _, factID := range used {
		if !allowed[factID] {
			missing = append(missing, factID)
		}
	}
	return missing
}

func isClarificationTurn(result turn.Result) bool {
	if result.StopReason == "clarification" || strings.TrimSpace(result.ClarificationQuestion) != "" {
		return true
	}
	for _, outcome := range result.Outcomes {
		if outcome.Result.Status == game.ActionClarification || outcome.Result.NeedsClarification {
			return true
		}
	}
	return false
}

func leaksPitchBlackRoomAwareness(step Step, state *world.State, text string) bool {
	if state == nil || step.Before.ActiveLight || step.After.ActiveLight || !isRoomAwarenessTurn(step.Turn.Result) {
		return false
	}
	room, ok := state.Rooms[step.Before.CurrentRoom]
	if !ok || room.Visibility != world.VisibilityPitchBlack {
		return false
	}
	for _, hiddenName := range hiddenRoomNamesAndDirections(state, room) {
		if containsNormalizedName(text, hiddenName) {
			return true
		}
	}
	return false
}

func isRoomAwarenessTurn(result turn.Result) bool {
	for _, outcome := range result.Outcomes {
		if outcome.Intent.Action == intent.ActionInspect && strings.TrimSpace(outcome.Intent.Target) == "" {
			return true
		}
		if outcome.Result.Outcome == "inspected_room" {
			return true
		}
	}
	return false
}

func hiddenRoomNamesAndDirections(state *world.State, room world.Room) []string {
	names := make([]string, 0, len(room.Objects)+len(room.Exits))
	for _, objectID := range room.Objects {
		if object, ok := state.Objects[objectID]; ok {
			names = append(names, object.Name)
		}
	}
	for _, exit := range room.Exits {
		if !state.KnownExitDirections[room.ID][exit.Direction] {
			names = append(names, exit.Direction)
		}
	}
	return names
}

func containsNormalizedName(text, name string) bool {
	words := normalizedWords(text)
	nameWords := normalizedWords(name)
	if len(words) == 0 || len(nameWords) == 0 || len(nameWords) > len(words) {
		return false
	}
	for start := 0; start+len(nameWords) <= len(words); start++ {
		matches := true
		for index := range nameWords {
			if words[start+index] != nameWords[index] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func normalizedWords(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}
