package intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"kaya/internal/game"
)

var ErrEmptyMessage = errors.New("empty player message")
var ErrPlanTooLarge = errors.New("turn plan exceeds four entries")

type StructuredGenerator interface {
	GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, schema any) (string, error)
}

type Parser struct {
	generator StructuredGenerator
}

func NewParser(generator StructuredGenerator) Parser {
	return Parser{generator: generator}
}

func (p Parser) Parse(ctx context.Context, message string, snapshot game.PerceptionSnapshot) (TurnPlan, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return TurnPlan{}, ErrEmptyMessage
	}
	if p.generator == nil {
		return FallbackPlan(message), nil
	}

	request := NewModelRequest(message, snapshot)
	payload, err := json.Marshal(request)
	if err != nil {
		return TurnPlan{}, fmt.Errorf("encode parser input: %w", err)
	}
	raw, err := p.generator.GenerateJSON(ctx, SystemPrompt, string(payload), ModelPlanSchema)
	if err != nil {
		return FallbackPlan(message), nil
	}

	plan, parseErr := ParseModelPlanJSON(raw, message)
	if parseErr == nil {
		return plan, nil
	}
	repairPayload, err := json.Marshal(struct {
		Request     ModelRequest `json:"request"`
		InvalidPlan string       `json:"invalidPlan"`
	}{Request: request, InvalidPlan: raw})
	if err != nil {
		return FallbackPlan(message), nil
	}
	repaired, repairErr := p.generator.GenerateJSON(ctx, RepairPrompt, string(repairPayload), ModelPlanSchema)
	if repairErr != nil {
		return FallbackPlan(message), nil
	}
	plan, parseErr = ParseModelPlanJSON(repaired, message)
	if parseErr != nil {
		return FallbackPlan(message), nil
	}
	return plan, nil
}

func ParseTurnPlanJSON(raw string) (TurnPlan, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return TurnPlan{}, errors.New("empty turn plan json")
	}
	raw = trimCodeFence(raw)
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var plan TurnPlan
	if err := decoder.Decode(&plan); err != nil {
		return TurnPlan{}, fmt.Errorf("decode turn plan json: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return TurnPlan{}, errors.New("turn plan json contains multiple objects")
	} else if !errors.Is(err, io.EOF) {
		return TurnPlan{}, fmt.Errorf("decode turn plan trailing data: %w", err)
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &shape); err != nil {
		return TurnPlan{}, fmt.Errorf("decode turn plan shape: %w", err)
	}
	for _, key := range []string{"actions", "questions", "confidence", "needsClarification", "clarificationQuestion", "rawText"} {
		if _, ok := shape[key]; !ok {
			return TurnPlan{}, fmt.Errorf("missing required field %q", key)
		}
	}
	if err := validatePlanRequiredFields(shape); err != nil {
		return TurnPlan{}, err
	}
	if len(plan.Actions) > 4 || len(plan.Questions) > 4 {
		return TurnPlan{}, ErrPlanTooLarge
	}
	if plan.Confidence < 0 || plan.Confidence > 1 {
		return TurnPlan{}, fmt.Errorf("confidence %.2f outside range 0..1", plan.Confidence)
	}
	for i := range plan.Actions {
		if !plan.Actions[i].Intent.Action.Valid() {
			return TurnPlan{}, fmt.Errorf("invalid action %q", plan.Actions[i].Intent.Action)
		}
		if len(plan.Actions[i].Intent.Modifiers) == 0 && plan.Actions[i].Intent.Action == "" {
			return TurnPlan{}, errors.New("missing action intent")
		}
		if !validTargetMode(plan.Actions[i].TargetMode) {
			return TurnPlan{}, fmt.Errorf("invalid target mode %q", plan.Actions[i].TargetMode)
		}
		if plan.Actions[i].Intent.Confidence < 0 || plan.Actions[i].Intent.Confidence > 1 {
			return TurnPlan{}, fmt.Errorf("action confidence %.2f outside range 0..1", plan.Actions[i].Intent.Confidence)
		}
		if plan.Actions[i].Intent.Modifiers == nil {
			plan.Actions[i].Intent.Modifiers = []string{}
		}
		plan.Actions[i].Intent = normalizeIntent(plan.Actions[i].Intent)
	}
	for i := range plan.Questions {
		if plan.Questions[i].Kind != game.FactLifeStatus {
			return TurnPlan{}, fmt.Errorf("invalid question kind %q", plan.Questions[i].Kind)
		}
		if !validTargetMode(plan.Questions[i].TargetMode) {
			return TurnPlan{}, fmt.Errorf("invalid target mode %q", plan.Questions[i].TargetMode)
		}
	}
	if plan.Confidence < 0.40 {
		plan.Actions = []PlannedAction{}
		plan.NeedsClarification = true
		if strings.TrimSpace(plan.ClarificationQuestion) == "" {
			plan.ClarificationQuestion = defaultClarification
		}
	}
	return plan, nil
}

func validatePlanRequiredFields(shape map[string]json.RawMessage) error {
	var actions []json.RawMessage
	if err := decodeNonNull(shape["actions"], &actions, "actions"); err != nil {
		return fmt.Errorf("actions must be an array: %w", err)
	}
	var confidence float64
	var needsClarification bool
	var clarificationQuestion, rawText string
	if err := decodeNonNull(shape["confidence"], &confidence, "confidence"); err != nil {
		return err
	}
	if err := decodeNonNull(shape["needsClarification"], &needsClarification, "needsClarification"); err != nil {
		return err
	}
	if err := decodeNonNull(shape["clarificationQuestion"], &clarificationQuestion, "clarificationQuestion"); err != nil {
		return err
	}
	if err := decodeNonNull(shape["rawText"], &rawText, "rawText"); err != nil {
		return err
	}
	for i, encoded := range actions {
		var action map[string]json.RawMessage
		if err := decodeNonNull(encoded, &action, fmt.Sprintf("action %d", i)); err != nil {
			return fmt.Errorf("action %d must be an object: %w", i, err)
		}
		for _, key := range []string{"intent", "targetMode"} {
			if _, ok := action[key]; !ok {
				return fmt.Errorf("action %d missing required field %q", i, key)
			}
		}
		var targetMode string
		if err := decodeNonNull(action["targetMode"], &targetMode, fmt.Sprintf("action %d targetMode", i)); err != nil {
			return err
		}
		var embedded map[string]json.RawMessage
		if err := decodeNonNull(action["intent"], &embedded, fmt.Sprintf("action %d intent", i)); err != nil {
			return fmt.Errorf("action %d intent must be an object: %w", i, err)
		}
		for _, key := range []string{"action", "target", "item", "direction", "modifiers", "confidence", "rawText", "needsClarification", "clarificationQuestion"} {
			if _, ok := embedded[key]; !ok {
				return fmt.Errorf("action %d intent missing required field %q", i, key)
			}
		}
		var actionName, target, item, direction, embeddedRaw, embeddedQuestion string
		var modifiers []string
		var embeddedConfidence float64
		var embeddedNeedsClarification bool
		for key, out := range map[string]any{"action": &actionName, "target": &target, "item": &item, "direction": &direction, "rawText": &embeddedRaw, "clarificationQuestion": &embeddedQuestion, "modifiers": &modifiers, "confidence": &embeddedConfidence, "needsClarification": &embeddedNeedsClarification} {
			if err := decodeNonNull(embedded[key], out, fmt.Sprintf("action %d intent %s", i, key)); err != nil {
				return err
			}
		}
	}
	var questions []json.RawMessage
	if err := decodeNonNull(shape["questions"], &questions, "questions"); err != nil {
		return fmt.Errorf("questions must be an array: %w", err)
	}
	for i, encoded := range questions {
		var question map[string]json.RawMessage
		if err := decodeNonNull(encoded, &question, fmt.Sprintf("question %d", i)); err != nil {
			return fmt.Errorf("question %d must be an object: %w", i, err)
		}
		for _, key := range []string{"kind", "target", "targetMode"} {
			if _, ok := question[key]; !ok {
				return fmt.Errorf("question %d missing required field %q", i, key)
			}
		}
		var kind, target, targetMode string
		for key, out := range map[string]any{"kind": &kind, "target": &target, "targetMode": &targetMode} {
			if err := decodeNonNull(question[key], out, fmt.Sprintf("question %d %s", i, key)); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeNonNull(raw json.RawMessage, target any, field string) error {
	if strings.TrimSpace(string(raw)) == "null" {
		return fmt.Errorf("%s must not be null", field)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%s has invalid type: %w", field, err)
	}
	return nil
}

func validTargetMode(mode TargetMode) bool {
	return mode == TargetSingle || mode == TargetAll
}

/*
ParseJSON remains the single-intent compatibility parser for callers that
still consume the legacy shape.
*/
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

/* old implementation removed */
/*
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

*/

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
	if parsed.Item == "" && (containsToken(raw, "flashlight") || containsToken(raw, "torch")) {
		parsed.Item = "flashlight"
	}
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
	if (parsed.Action == ActionInspect || parsed.Action == ActionSearch) && parsed.Target != "" && parsed.Direction != "" {
		parsed.Target = strings.TrimSpace(parsed.Target + " " + parsed.Direction)
		parsed.Direction = ""
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

func containsToken(value, wanted string) bool {
	words := strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, word := range words {
		if word == wanted {
			return true
		}
	}
	return false
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
