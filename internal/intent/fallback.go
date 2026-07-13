package intent

import (
	"strings"
	"unicode"

	"kaya/internal/game"
)

const defaultClarification = "What do you want Kaya to do?"

func FallbackPlan(message string) TurnPlan {
	message = strings.TrimSpace(message)
	if plan, ok := PureConversationPlan(message); ok {
		return plan
	}
	if plan, ok := compoundFallbackPlan(message); ok {
		return plan
	}
	return fallbackSinglePlan(message)
}

// PureConversationPlan recognizes only complete, non-gameplay conversational messages.
func PureConversationPlan(message string) (TurnPlan, bool) {
	raw := strings.TrimSpace(message)
	words := strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	normalized := strings.Join(words, " ")
	switch normalized {
	case "hello", "hello kaya", "hi", "hi kaya", "hey", "hey kaya",
		"okay", "ok", "all right", "alright", "yes", "no", "got it", "understood",
		"thanks", "thank you", "i hear you", "i m here", "im here",
		"are you there", "are you still with me", "can you hear me", "is the line still clear",
		"hello are you there", "hello are you still with me", "hello can you hear me", "hello is the line still clear":
		in := Intent{Action: ActionTalk, Target: "conversation", Confidence: 1, RawText: raw, Modifiers: []string{}}
		return TurnPlan{
			Actions:    []PlannedAction{{Intent: in, TargetMode: TargetSingle}},
			Confidence: 1,
			RawText:    raw,
		}, true
	default:
		return TurnPlan{}, false
	}
}

func fallbackSinglePlan(message string) TurnPlan {
	message = strings.TrimSpace(message)
	low := normalizePlayerText(message)
	intent := Intent{Action: ActionUnknown, Confidence: 0, RawText: message, Modifiers: []string{}, NeedsClarification: true, ClarificationQuestion: defaultClarification}
	targetMode := TargetSingle
	switch {
	case isLifeStatusSearch(low):
		return TurnPlan{
			Actions: []PlannedAction{{
				Intent: Intent{Action: ActionSearch, Target: "doctors", Confidence: 0.8,
					RawText: message, Modifiers: []string{}},
				TargetMode: TargetAll,
			}},
			Questions:  []FactQuestion{{Kind: game.FactLifeStatus, Target: "doctors", TargetMode: TargetAll}},
			Confidence: 0.8,
			RawText:    message,
		}
	case isPluralReferentSelection(low):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionSearch, pluralReferentTarget(low), 0.8, false, ""
		targetMode = TargetAll
	case (strings.Contains(low, "feel") || strings.Contains(low, "run your hands") || strings.Contains(low, "trace")) && strings.Contains(low, "wall"):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionExplore, 0.8, false, ""
	case (strings.Contains(low, "turn on") || strings.Contains(low, "switch on") || strings.Contains(low, "activate")) && (strings.Contains(low, "flashlight") || strings.Contains(low, "torch") || strings.Contains(low, "light")):
		intent.Action, intent.Item, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTurnOn, "flashlight", 0.9, false, ""
	case (strings.Contains(low, "turn off") || strings.Contains(low, "switch off") || strings.Contains(low, "deactivate")) && (strings.Contains(low, "flashlight") || strings.Contains(low, "torch") || strings.Contains(low, "light")):
		intent.Action, intent.Item, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTurnOff, "flashlight", 0.9, false, ""
	case isMovementMessage(low):
		intent.Action, intent.Direction, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionMove, movementDirection(low), 0.8, false, ""
	case isFallbackInventoryQuestion(low):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTalk, 0.8, false, ""
		if item := fallbackMentionedItem(low); item != "" {
			intent.Item = item
		} else {
			intent.Target = "inventory"
		}
	case isContainerContentsQuestion(low):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionSearch, extractContainerQuestionTarget(message), 0.75, false, ""
	case isFallbackItemLocationQuestion(low) || isFallbackItemPresenceQuestion(low):
		intent.Action, intent.Item, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTalk, fallbackMentionedItem(low), 0.75, false, ""
	case isObjectInspectMessage(low) && !strings.Contains(low, "inspect the room"):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionInspect, extractObjectTarget(message), 0.8, false, ""
	case isGeneralRoomAwareness(low, "") || strings.Contains(low, "inspect the room"):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionInspect, 0.8, false, ""
	case strings.Contains(low, "hide"):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionHide, extractHideTarget(message), 0.8, false, ""
	case strings.Contains(low, "throw"):
		intent.Item, intent.Target = extractThrowParts(message)
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionThrow, 0.8, false, ""
	case isUseItemMessage(low):
		intent.Item, intent.Target = extractUseItemParts(message)
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionUseItem, 0.8, false, ""
	case isSearchMessage(low):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionSearch, extractSearchTarget(message), 0.7, false, ""
	case strings.Contains(low, "pick up") || strings.Contains(low, "take ") || strings.HasPrefix(low, "take") || strings.Contains(low, "grab "):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTakeItem, extractTakeTarget(message), 0.8, false, ""
	case strings.Contains(low, "wait") || strings.Contains(low, "stay still") || strings.Contains(low, "pause"):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionWait, 0.9, false, ""
	case strings.Contains(low, "listen") || strings.Contains(low, "hear"):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionListen, extractListenTarget(message), 0.8, false, ""
	}
	return TurnPlan{Actions: []PlannedAction{{Intent: intent, TargetMode: targetMode}}, Confidence: intent.Confidence, NeedsClarification: intent.NeedsClarification, ClarificationQuestion: intent.ClarificationQuestion, RawText: message}
}

func compoundFallbackPlan(message string) (TurnPlan, bool) {
	clauses := splitSequentialClauses(message)
	if len(clauses) < 2 || len(clauses) > 4 {
		return TurnPlan{}, false
	}

	actions := make([]PlannedAction, 0, len(clauses))
	confidence := 1.0
	for _, clause := range clauses {
		plan := fallbackSinglePlan(clause)
		if plan.NeedsClarification || len(plan.Actions) != 1 || len(plan.Questions) != 0 {
			return TurnPlan{}, false
		}
		action := plan.Actions[0]
		if action.Intent.Action == ActionUnknown {
			return TurnPlan{}, false
		}
		actions = append(actions, action)
		if plan.Confidence < confidence {
			confidence = plan.Confidence
		}
	}

	return TurnPlan{
		Actions:    actions,
		Confidence: confidence,
		RawText:    strings.TrimSpace(message),
	}, true
}

func splitSequentialClauses(message string) []string {
	words := strings.Fields(normalizePlayerText(message))
	if len(words) == 0 {
		return nil
	}

	clauses := make([]string, 0, 2)
	start := 0
	for i, word := range words {
		if word != "and" && word != "then" {
			continue
		}
		clause := strings.Join(words[start:i], " ")
		if clause != "" {
			clauses = append(clauses, clause)
		}
		start = i + 1
	}
	if tail := strings.Join(words[start:], " "); tail != "" {
		clauses = append(clauses, tail)
	}
	return clauses
}

func isPluralReferentSelection(message string) bool {
	switch stripTrailingPoliteness(message) {
	case "both", "both doctors", "both of them", "them", "all", "all of them":
		return true
	default:
		return false
	}
}

func pluralReferentTarget(message string) string {
	if strings.Contains(message, "both") {
		return "both"
	}
	if strings.Trim(message, " .!?") == "all" {
		return "all"
	}
	return "them"
}

func isFallbackInventoryQuestion(message string) bool {
	return isInventoryQuestion(message) || strings.Contains(message, "what is in your bag") ||
		strings.Contains(message, "what's in your bag") || strings.Contains(message, "what is in your inventory") ||
		strings.Contains(message, "what's in your inventory") ||
		((strings.Contains(message, "your bag") || strings.Contains(message, "your inventory")) &&
			(strings.Contains(message, "anything") || strings.Contains(message, "something") || fallbackMentionedItem(message) != ""))
}

func fallbackMentionedItem(message string) string {
	if strings.Contains(message, "flashlight") || strings.Contains(message, "torch") {
		return "flashlight"
	}
	if strings.Contains(message, "key") {
		return "key"
	}
	return ""
}

func isFallbackItemLocationQuestion(message string) bool {
	if fallbackMentionedItem(message) == "" {
		return false
	}
	return strings.Contains(message, "where is") ||
		strings.Contains(message, "where's") ||
		strings.Contains(message, "where did") ||
		strings.Contains(message, "where was") ||
		strings.Contains(message, "where can") ||
		strings.Contains(message, "where would")
}

func isFallbackItemPresenceQuestion(message string) bool {
	if fallbackMentionedItem(message) == "" {
		return false
	}
	return strings.Contains(message, "is there") ||
		strings.Contains(message, "are there") ||
		strings.Contains(message, "do you see") ||
		strings.Contains(message, "can you see") ||
		strings.Contains(message, "have you found") ||
		strings.Contains(message, "did you find")
}

func isContainerContentsQuestion(message string) bool {
	if strings.Contains(message, "your bag") || strings.Contains(message, "your inventory") {
		return false
	}
	if !(strings.Contains(message, " inside ") || strings.Contains(message, " in ")) {
		return false
	}
	if isRoomLikeContainerTarget(extractContainerQuestionTarget(message)) {
		return false
	}
	return strings.Contains(message, "what is inside") ||
		strings.Contains(message, "what's inside") ||
		strings.Contains(message, "what is in") ||
		strings.Contains(message, "what's in") ||
		strings.Contains(message, "is something") ||
		strings.Contains(message, "is anything") ||
		strings.Contains(message, "is there something") ||
		strings.Contains(message, "is there anything") ||
		strings.Contains(message, "anything inside") ||
		strings.Contains(message, "something inside")
}

func isRoomLikeContainerTarget(target string) bool {
	target = strings.TrimSpace(target)
	return target == "" ||
		target == "room" ||
		target == "current room" ||
		target == "here" ||
		target == "area" ||
		target == "around you" ||
		strings.Contains(target, " room")
}

func isObjectInspectMessage(message string) bool {
	return strings.Contains(message, "what is on ") || strings.Contains(message, "what's on ") ||
		strings.Contains(message, "what is in ") || strings.Contains(message, "what's in ") ||
		strings.Contains(message, "what is inside ") || strings.Contains(message, "what's inside ") ||
		strings.Contains(message, "inspect ") || strings.Contains(message, "look at ") ||
		strings.Contains(message, "look on ") || strings.Contains(message, "look over ")
}

func isSearchMessage(message string) bool {
	return findCue(message, "search") >= 0 ||
		findCue(message, "rummage") >= 0 ||
		findCue(message, "check") >= 0 ||
		strings.Contains(message, "look through") ||
		strings.Contains(message, "look inside") ||
		strings.Contains(message, "look in")
}

func extractSearchTarget(message string) string {
	return extractTarget(message, []string{"look through", "look inside", "look in", "search", "rummage", "check"})
}

func extractContainerQuestionTarget(message string) string {
	return extractTarget(message, []string{"inside", "in"})
}

func extractObjectTarget(message string) string {
	return extractTarget(message, []string{"what is on", "what's on", "what is inside", "what's inside", "what is in", "what's in", "inspect", "look at", "look on", "look over"})
}

func extractTakeTarget(message string) string {
	target := extractTarget(message, []string{"pick up", "take", "grab"})
	if from := strings.Index(target, " from "); from >= 0 {
		target = target[:from]
	}
	return target
}

func extractListenTarget(message string) string {
	return extractTarget(message, []string{"listen at"})
}

func extractHideTarget(message string) string {
	target := extractTarget(message, []string{"get behind", "hide behind"})
	if split := strings.Index(target, " and hide"); split >= 0 {
		target = target[:split]
	}
	return strings.TrimSpace(target)
}

func extractThrowParts(message string) (item string, target string) {
	low := normalizePlayerText(message)
	index := findCue(low, "throw")
	if index < 0 {
		return "", ""
	}
	rest := strings.TrimSpace(low[index+len("throw"):])
	for _, separator := range []string{" down ", " at "} {
		if split := strings.Index(rest, separator); split >= 0 {
			item = cleanTargetPrefix(rest[:split])
			target = cleanTargetPrefix(rest[split+len(separator):])
			return item, target
		}
	}
	return "", ""
}

func extractUseItemParts(message string) (item string, target string) {
	low := normalizePlayerText(message)
	index := findCue(low, "use")
	verb := "use"
	if index < 0 {
		index = findCue(low, "try")
		verb = "try"
	}
	if index < 0 {
		return "", ""
	}
	rest := strings.TrimSpace(low[index+len(verb):])
	split := strings.Index(rest, " on ")
	if split < 0 {
		return "", ""
	}
	return cleanTargetPrefix(rest[:split]), cleanTargetPrefix(rest[split+len(" on "):])
}

func isUseItemMessage(message string) bool {
	return (findCue(message, "use") >= 0 || strings.HasPrefix(message, "try ")) && strings.Contains(message, " on ")
}

func isLifeStatusSearch(message string) bool {
	return strings.Contains(message, "search the doctors are they dead")
}

func extractTarget(message string, cues []string) string {
	low := normalizePlayerText(message)
	for _, cue := range cues {
		if index := findCue(low, cue); index >= 0 {
			target := strings.TrimSpace(low[index+len(cue):])
			target = strings.Trim(target, " \t.,!?;:'\"")
			target = cleanTargetPrefix(target)
			if split := strings.Index(target, " are they"); split >= 0 {
				target = target[:split]
			}
			return strings.TrimSpace(strings.Trim(target, ".,!?;:"))
		}
	}
	return ""
}

func cleanTargetPrefix(target string) string {
	target = strings.TrimSpace(target)
	for {
		before := target
		for _, prefix := range []string{"through the ", "through ", "the ", "a ", "an ", "your ", "for "} {
			target = strings.TrimPrefix(target, prefix)
		}
		if target == before {
			return stripTrailingPoliteness(target)
		}
	}
}

func stripTrailingPoliteness(value string) string {
	value = strings.TrimSpace(strings.Trim(value, ".,!?;:"))
	for _, suffix := range []string{", please", " please"} {
		if strings.HasSuffix(value, suffix) {
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
			break
		}
	}
	return strings.TrimSpace(strings.Trim(value, ".,!?;:"))
}

func findCue(value string, cue string) int {
	start := 0
	for {
		index := strings.Index(value[start:], cue)
		if index < 0 {
			return -1
		}
		index += start
		if cueAtBoundary(value, index, cue) {
			return index
		}
		start = index + 1
	}
}

func cueAtBoundary(value string, index int, cue string) bool {
	before := index == 0 || isBoundaryByte(value[index-1])
	afterIndex := index + len(cue)
	after := afterIndex >= len(value) || isBoundaryByte(value[afterIndex])
	return before && after
}

func isBoundaryByte(value byte) bool {
	return (value < 'a' || value > 'z') && (value < '0' || value > '9')
}

func normalizePlayerText(message string) string {
	low := strings.ToLower(strings.TrimSpace(message))
	for _, replacement := range [][2]string{
		{"ypou", "you"},
		{"look ath ", "look at "},
		{"tun on", "turn on"},
		{"searxch", "search"},
		{"serach", "search"},
		{"seach", "search"},
		{"soemthing", "something"},
		{"soemthjing", "something"},
		{"isnide", "inside"},
		{"cabiner", "cabinet"},
		{"took ", "take "},
	} {
		low = strings.ReplaceAll(low, replacement[0], replacement[1])
	}
	return strings.Join(strings.Fields(low), " ")
}

func isMovementMessage(message string) bool {
	direction := movementDirection(message)
	if direction == "" {
		return false
	}
	if containsToken(message, direction) && len(strings.Fields(message)) == 1 {
		return true
	}
	for _, verb := range []string{"go", "move", "walk", "head", "step", "back up", "turn"} {
		if strings.Contains(verb, " ") && strings.Contains(message, verb) {
			return true
		}
		if !strings.Contains(verb, " ") && containsToken(message, verb) {
			return true
		}
	}
	return false
}

func movementDirection(message string) string {
	for _, direction := range []string{"north", "south", "east", "west", "left", "right", "forward", "backward", "back", "ahead", "up", "down"} {
		if containsToken(message, direction) {
			return direction
		}
	}
	return ""
}
