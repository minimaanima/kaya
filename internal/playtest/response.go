package playtest

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/response"
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
	for _, factID := range ungroundedFactIDs(step, responseFactIDs(reply)) {
		violations = append(violations, responseViolation("response_fact_id_ungrounded", text, fmt.Sprintf("used fact ID %q is absent from the stored turn fact bundle", factID)))
	}
	if isClarificationTurn(step.Turn.Result) && step.After.Time != step.Before.Time {
		violations = append(violations, responseViolation("response_clarification_advanced_time", text, fmt.Sprintf("clarification advanced time from %d to %d", step.Before.Time, step.After.Time)))
	}
	if leaksPitchBlackRoomAwareness(step, state) {
		violations = append(violations, responseViolation("response_darkness_leak", text, "pitch-black room awareness names a hidden object or direction"))
	}
	if debugResponseMarker.MatchString(text) {
		violations = append(violations, responseViolation("response_debug_marker", text, "response contains a debug marker"))
	}
	return sortViolations(violations)
}

func responseFactIDs(reply response.Response) []game.FactID {
	ids := append([]game.FactID(nil), reply.UsedFactIDs...)
	for _, sentence := range reply.Sentences {
		ids = append(ids, sentence.FactIDs...)
	}
	return ids
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

func leaksPitchBlackRoomAwareness(step Step, state *world.State) bool {
	if state == nil {
		return false
	}
	groups := outcomeFactIDGroups(step.Turn.Result)
	hasSentenceEvidence := len(step.Turn.Response.Sentences) > 0
	location := outcomeLocation{
		room:     step.Before.CurrentRoom,
		previous: step.Before.PreviousRoom,
		light:    step.Before.ActiveLight,
	}
	for index, outcome := range step.Turn.Result.Outcomes {
		if isRoomAwarenessOutcome(outcome) {
			if !hasSentenceEvidence {
				if leaksPitchBlackRoomTerms(state, location.room, location.light, step.Turn.Response.Text) {
					return true
				}
			} else {
				for _, sentence := range step.Turn.Response.Sentences {
					if citesAny(sentence.FactIDs, groups[index]) && leaksPitchBlackRoomTerms(state, location.room, location.light, sentence.Text) {
						return true
					}
				}
			}
		}
		location.advance(state, outcome)
	}
	return false
}

func outcomeFactIDGroups(result turn.Result) [][]game.FactID {
	bundle := result.FactBundle("")
	groups := make([][]game.FactID, len(result.Outcomes))
	cursor := 0
	for index, outcome := range result.Outcomes {
		count := outcomeFactCount(outcome)
		end := cursor + count
		if end > len(bundle.Facts) {
			end = len(bundle.Facts)
		}
		for _, fact := range bundle.Facts[cursor:end] {
			groups[index] = append(groups[index], fact.ID)
		}
		cursor = end
	}
	return groups
}

func outcomeFactCount(outcome turn.ActionOutcome) int {
	count := len(outcome.Result.VisibleFacts)
	hasFailure, hasClarification := false, false
	for _, fact := range outcome.Result.VisibleFacts {
		hasFailure = hasFailure || fact.Kind == game.FactFailure
		hasClarification = hasClarification || fact.Kind == game.FactClarification
	}
	if (outcome.Result.Status == game.ActionFailed || outcome.Result.Status == game.ActionRefused) && !hasFailure {
		count++
	}
	if (outcome.Result.Status == game.ActionClarification || outcome.Result.NeedsClarification) && !hasClarification {
		count++
	}
	if outcome.Result.DurationSeconds > 0 {
		count++
	}
	return count + len(outcome.Result.Events)
}

func citesAny(cited, outcomeFacts []game.FactID) bool {
	for _, citedID := range cited {
		for _, outcomeID := range outcomeFacts {
			if citedID == outcomeID {
				return true
			}
		}
	}
	return false
}

type outcomeLocation struct {
	room, previous game.RoomID
	light          bool
}

func (location *outcomeLocation) advance(state *world.State, outcome turn.ActionOutcome) {
	if outcome.Result.Outcome == "moved" {
		if next, ok := movedRoom(state, location.room, location.previous, outcome.Intent); ok {
			location.previous, location.room = location.room, next
		}
	}
	switch outcome.Result.Outcome {
	case "flashlight_on":
		location.light = true
	case "flashlight_off":
		location.light = false
	}
}

func movedRoom(state *world.State, current, previous game.RoomID, in intent.Intent) (game.RoomID, bool) {
	room, ok := state.Rooms[current]
	if !ok {
		return "", false
	}
	direction := strings.TrimSpace(in.Direction)
	if direction == "" {
		direction = strings.TrimSpace(in.Target)
	}
	if world.MatchesTarget(direction, "back", []string{"backward", "previous room", "where you came from"}) && previous != "" {
		for _, exit := range room.Exits {
			if exit.To == previous {
				return exit.To, true
			}
		}
	}
	for _, exit := range room.Exits {
		if world.MatchesTarget(direction, exit.Direction, nil) {
			return exit.To, true
		}
	}
	return "", false
}

func isRoomAwarenessOutcome(outcome turn.ActionOutcome) bool {
	return (outcome.Intent.Action == intent.ActionInspect && strings.TrimSpace(outcome.Intent.Target) == "") || outcome.Result.Outcome == "inspected_room"
}

func leaksPitchBlackRoomTerms(state *world.State, roomID game.RoomID, activeLight bool, text string) bool {
	if activeLight {
		return false
	}
	room, ok := state.Rooms[roomID]
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
