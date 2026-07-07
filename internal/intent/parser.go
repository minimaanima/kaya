package intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrEmptyMessage = errors.New("empty player message")

type TextGenerator interface {
	Generate(ctx context.Context, systemPrompt string, userPrompt string) (string, error)
}

type Parser struct {
	generator TextGenerator
}

func NewParser(generator TextGenerator) Parser {
	return Parser{generator: generator}
}

func (p Parser) Parse(ctx context.Context, message string) (Intent, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return Intent{}, ErrEmptyMessage
	}
	if p.generator == nil {
		return Intent{}, errors.New("intent parser missing text generator")
	}

	raw, err := p.generator.Generate(ctx, SystemPrompt, "Player message:\n"+message)
	if err != nil {
		return Intent{}, fmt.Errorf("generate intent: %w", err)
	}

	parsed, err := ParseJSON(raw)
	if err == nil {
		return parsed, nil
	}

	repaired, repairErr := p.generator.Generate(ctx, RepairPrompt, raw)
	if repairErr != nil {
		return Intent{}, fmt.Errorf("parse intent json: %w", err)
	}

	parsed, err = ParseJSON(repaired)
	if err != nil {
		return Intent{}, fmt.Errorf("parse repaired intent json: %w", err)
	}

	return parsed, nil
}

func ParseJSON(raw string) (Intent, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Intent{}, errors.New("empty intent json")
	}

	raw = trimCodeFence(raw)

	var parsed Intent
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return Intent{}, fmt.Errorf("decode intent json: %w", err)
	}
	if !parsed.Action.Valid() {
		return Intent{}, fmt.Errorf("invalid action %q", parsed.Action)
	}
	if parsed.Confidence < 0 || parsed.Confidence > 1 {
		return Intent{}, fmt.Errorf("confidence %.2f outside range 0..1", parsed.Confidence)
	}
	if parsed.Modifiers == nil {
		parsed.Modifiers = []string{}
	}

	return normalizeIntent(parsed), nil
}

func trimCodeFence(raw string) string {
	if !strings.HasPrefix(raw, "```") {
		return raw
	}

	lines := strings.Split(raw, "\n")
	if len(lines) < 3 {
		return raw
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return raw
	}

	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

func normalizeIntent(parsed Intent) Intent {
	parsed.Target = normalizeEmptyField(parsed.Target)
	parsed.Item = normalizeEmptyField(parsed.Item)
	parsed.Direction = normalizeEmptyField(parsed.Direction)
	parsed.ClarificationQuestion = normalizeEmptyField(parsed.ClarificationQuestion)

	raw := strings.ToLower(strings.TrimSpace(parsed.RawText))
	if isVagueFollowUp(raw) {
		parsed.Action = ActionUnknown
		parsed.Target = ""
		parsed.Item = ""
		parsed.Direction = ""
		parsed.Modifiers = []string{}
		parsed.Confidence = minConfidence(parsed.Confidence, 0.25)
		parsed.NeedsClarification = true
		if parsed.ClarificationQuestion == "" {
			parsed.ClarificationQuestion = "What do you want Kaya to do?"
		}
		return parsed
	}

	if isInventoryQuestion(raw) {
		parsed.Action = ActionTalk
		parsed.Target = normalizeInventoryTarget(parsed.Target)
	}

	if parsed.Action == ActionMove && parsed.Direction == "" && isDirection(parsed.Target) {
		parsed.Direction = strings.ToLower(parsed.Target)
		parsed.Target = ""
	}

	if (parsed.Action == ActionInspect || parsed.Action == ActionSearch) && isGeneralRoomAwareness(raw, parsed.Target) {
		parsed.Action = ActionInspect
		parsed.Target = ""
	}

	if parsed.Action == ActionForceOpen && isKeyUse(raw, parsed.Item) {
		parsed.Action = ActionUseItem
	}

	return parsed
}

func normalizeEmptyField(value string) string {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "empty string", "none", "null", "n/a":
		return ""
	default:
		return trimmed
	}
}

func isVagueFollowUp(raw string) bool {
	raw = strings.Trim(raw, " .!?")
	switch raw {
	case "do it", "do that", "try it", "try that", "yes", "yeah", "ok", "okay", "go ahead":
		return true
	default:
		return false
	}
}

func isDirection(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "left", "right", "north", "south", "east", "west", "up", "down", "forward", "back", "backward", "ahead":
		return true
	default:
		return false
	}
}

func isGeneralRoomAwareness(raw string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target != "" &&
		target != "room" &&
		target != "the room" &&
		target != "current room" &&
		target != "here" &&
		target != "area" &&
		target != "around you" &&
		target != "around me" &&
		target != "surroundings" {
		return false
	}

	return strings.Contains(raw, "look around") ||
		strings.Contains(raw, "what's in") ||
		strings.Contains(raw, "what is in") ||
		strings.Contains(raw, "anything around") ||
		strings.Contains(raw, "see anything") ||
		strings.Contains(raw, "anything useful")
}

func isKeyUse(raw string, item string) bool {
	item = strings.ToLower(strings.TrimSpace(item))
	return strings.Contains(raw, "key") || strings.Contains(item, "key")
}

func isInventoryQuestion(raw string) bool {
	return strings.Contains(raw, "do you have") ||
		strings.Contains(raw, "do ypou have") ||
		strings.Contains(raw, "are you carrying") ||
		strings.Contains(raw, "what do you have") ||
		strings.Contains(raw, "what are you carrying") ||
		strings.Contains(raw, "inventory")
}

func normalizeInventoryTarget(target string) string {
	target = strings.TrimSpace(target)
	switch strings.ToLower(target) {
	case "inventory", "items", "what do you have", "what are you carrying":
		return ""
	default:
		return target
	}
}

func minConfidence(current float64, maximum float64) float64 {
	if current < maximum {
		return current
	}
	return maximum
}
