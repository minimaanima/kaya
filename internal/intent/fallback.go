package intent

import "strings"

const defaultClarification = "What do you want Kaya to do?"

func FallbackPlan(message string) TurnPlan {
	message = strings.TrimSpace(message)
	low := strings.ToLower(message)
	intent := Intent{Action: ActionUnknown, Confidence: 0, RawText: message, Modifiers: []string{}, NeedsClarification: true, ClarificationQuestion: defaultClarification}
	switch {
	case (strings.Contains(low, "feel") || strings.Contains(low, "run your hands") || strings.Contains(low, "trace")) && strings.Contains(low, "wall"):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionExplore, 0.8, false, ""
	case (strings.Contains(low, "turn on") || strings.Contains(low, "switch on") || strings.Contains(low, "activate")) && (strings.Contains(low, "flashlight") || strings.Contains(low, "torch") || strings.Contains(low, "light")):
		intent.Action, intent.Item, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTurnOn, "flashlight", 0.9, false, ""
	case (strings.Contains(low, "turn off") || strings.Contains(low, "switch off") || strings.Contains(low, "deactivate")) && (strings.Contains(low, "flashlight") || strings.Contains(low, "torch") || strings.Contains(low, "light")):
		intent.Action, intent.Item, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTurnOff, "flashlight", 0.9, false, ""
	case isMovementMessage(low):
		intent.Action, intent.Direction, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionMove, movementDirection(low), 0.8, false, ""
	case isGreeting(low):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTalk, 0.9, false, ""
	case isFallbackInventoryQuestion(low):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTalk, 0.8, false, ""
		if item := fallbackInventoryItem(low); item != "" {
			intent.Item = item
		} else {
			intent.Target = "inventory"
		}
	case isObjectInspectMessage(low):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionInspect, extractObjectTarget(message), 0.8, false, ""
	case isGeneralRoomAwareness(low, "") || strings.Contains(low, "inspect the room"):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionInspect, 0.8, false, ""
	case strings.Contains(low, "search") || strings.Contains(low, "rummage") || strings.Contains(low, "look through") || strings.Contains(low, "look inside") || strings.Contains(low, "look in") || strings.Contains(low, "check"):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionSearch, extractSearchTarget(message), 0.7, false, ""
	case strings.Contains(low, "pick up") || strings.Contains(low, "take ") || strings.HasPrefix(low, "take") || strings.Contains(low, "grab "):
		intent.Action, intent.Target, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionTakeItem, extractTakeTarget(message), 0.8, false, ""
	case strings.Contains(low, "wait") || strings.Contains(low, "stay still") || strings.Contains(low, "pause"):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionWait, 0.9, false, ""
	case strings.Contains(low, "listen") || strings.Contains(low, "hear"):
		intent.Action, intent.Confidence, intent.NeedsClarification, intent.ClarificationQuestion = ActionListen, 0.8, false, ""
	}
	return TurnPlan{Actions: []PlannedAction{{Intent: intent, TargetMode: TargetSingle}}, Confidence: intent.Confidence, NeedsClarification: intent.NeedsClarification, ClarificationQuestion: intent.ClarificationQuestion, RawText: message}
}

func isFallbackInventoryQuestion(message string) bool {
	return isInventoryQuestion(message) || strings.Contains(message, "what is in your bag") ||
		strings.Contains(message, "what's in your bag") || strings.Contains(message, "what is in your inventory") ||
		strings.Contains(message, "what's in your inventory")
}

func fallbackInventoryItem(message string) string {
	if strings.Contains(message, "flashlight") || strings.Contains(message, "torch") {
		return "flashlight"
	}
	if strings.Contains(message, "key") {
		return "key"
	}
	return ""
}

func isObjectInspectMessage(message string) bool {
	return strings.Contains(message, "what is on ") || strings.Contains(message, "what's on ") || strings.Contains(message, "whats on ") ||
		strings.Contains(message, "what is in ") || strings.Contains(message, "what's in ") || strings.Contains(message, "whats in ") ||
		strings.Contains(message, "inspect ") || strings.Contains(message, "look at ")
}

func extractSearchTarget(message string) string {
	return extractTarget(message, []string{"look through", "look inside", "look in", "search", "rummage", "check"})
}

func extractObjectTarget(message string) string {
	return extractTarget(message, []string{"what is on", "what's on", "whats on", "what is in", "what's in", "whats in", "inspect", "look at"})
}

func extractTakeTarget(message string) string {
	target := extractTarget(message, []string{"pick up", "take", "grab"})
	if from := strings.Index(target, " from "); from >= 0 {
		target = target[:from]
	}
	return target
}

func extractTarget(message string, cues []string) string {
	low := strings.ToLower(strings.TrimSpace(message))
	for _, cue := range cues {
		if index := strings.Index(low, cue); index >= 0 {
			target := strings.TrimSpace(low[index+len(cue):])
			target = strings.Trim(target, " \t.,!?;:'\"")
			for {
				before := target
				for _, article := range []string{"the ", "a ", "an ", "your ", "for "} {
					target = strings.TrimPrefix(target, article)
				}
				if target == before {
					break
				}
			}
			if split := strings.Index(target, " are they"); split >= 0 {
				target = target[:split]
			}
			return strings.TrimSpace(strings.Trim(target, ".,!?;:"))
		}
	}
	return ""
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

func isGreeting(message string) bool {
	switch strings.TrimSpace(message) {
	case "hi", "hello", "hey", "yo":
		return true
	default:
		return false
	}
}

func movementDirection(message string) string {
	for _, direction := range []string{"north", "south", "east", "west", "left", "right", "forward", "backward", "back", "ahead", "up", "down"} {
		if containsToken(message, direction) {
			return direction
		}
	}
	return ""
}
