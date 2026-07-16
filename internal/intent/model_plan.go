package intent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"kaya/internal/game"
)

// ModelPlan is the small wire format requested from Ollama. The game keeps
// confidence, raw text, and execution details; the model only names intent.
type ModelPlan struct {
	Actions               []ModelAction  `json:"actions"`
	Questions             []FactQuestion `json:"questions"`
	NeedsClarification    bool           `json:"needsClarification"`
	ClarificationQuestion string         `json:"clarificationQuestion"`
}

type ModelAction struct {
	Action     Action     `json:"action"`
	Target     string     `json:"target"`
	Item       string     `json:"item"`
	Direction  string     `json:"direction"`
	TargetMode TargetMode `json:"targetMode"`
}

type ActionDefinition struct {
	Action Action `json:"action"`
	Use    string `json:"use"`
}

type ModelRequest struct {
	Player           string             `json:"player"`
	AvailableActions []ActionDefinition `json:"availableActions"`
	Perception       ModelPerception    `json:"perception"`
}

type ModelPerception struct {
	RoomName       string     `json:"roomName"`
	HasUsefulLight bool       `json:"hasUsefulLight"`
	VisibleObjects []string   `json:"visibleObjects"`
	KnownItems     []string   `json:"knownItems"`
	Inventory      []string   `json:"inventory"`
	KnownExits     []string   `json:"knownExits"`
	RecentTargets  [][]string `json:"recentTargets"`
}

var AvailableActions = []ActionDefinition{
	{Action: ActionInspect, Use: "look at a visible object; empty target means inspect the current room"},
	{Action: ActionSearch, Use: "search a visible object"},
	{Action: ActionTakeItem, Use: "pick up a discovered portable item"},
	{Action: ActionMove, Use: "move through a known exit using direction"},
	{Action: ActionTurnOn, Use: "turn on an inventory item"},
	{Action: ActionTurnOff, Use: "turn off an inventory item"},
	{Action: ActionUseItem, Use: "use an inventory item on a target"},
	{Action: ActionExplore, Use: "feel along walls to find an unknown exit"},
	{Action: ActionListen, Use: "listen"},
	{Action: ActionWait, Use: "wait"},
	{Action: ActionTalk, Use: "answer or acknowledge speech"},
	{Action: ActionHide, Use: "hide behind a visible object"},
	{Action: ActionThrow, Use: "throw an inventory item"},
	{Action: ActionForceOpen, Use: "force open a visible door or object"},
}

func NewModelRequest(player string, snapshot game.PerceptionSnapshot) ModelRequest {
	objects := make(map[game.ObjectID]string, len(snapshot.VisibleObjects))
	items := make(map[game.ItemID]string, len(snapshot.KnownItems)+len(snapshot.Inventory))
	perception := ModelPerception{
		RoomName: snapshot.RoomName, HasUsefulLight: snapshot.HasUsefulLight,
		VisibleObjects: make([]string, 0, len(snapshot.VisibleObjects)),
		KnownItems:     make([]string, 0, len(snapshot.KnownItems)),
		Inventory:      make([]string, 0, len(snapshot.Inventory)),
		KnownExits:     make([]string, 0, len(snapshot.KnownExits)),
		RecentTargets:  make([][]string, 0, len(snapshot.RecentReferents)),
	}
	for _, object := range snapshot.VisibleObjects {
		objects[object.ID] = object.Name
		perception.VisibleObjects = append(perception.VisibleObjects, object.Name)
	}
	for _, item := range snapshot.KnownItems {
		items[item.ID] = item.Name
		perception.KnownItems = append(perception.KnownItems, item.Name)
	}
	for _, item := range snapshot.Inventory {
		items[item.ID] = item.Name
		perception.Inventory = append(perception.Inventory, item.Name)
	}
	for _, exit := range snapshot.KnownExits {
		perception.KnownExits = append(perception.KnownExits, exit.Direction)
	}
	for _, group := range snapshot.RecentReferents {
		targets := make([]string, 0, len(group.ObjectIDs)+len(group.ItemIDs))
		for _, id := range group.ObjectIDs {
			if name := objects[id]; name != "" {
				targets = append(targets, name)
			}
		}
		for _, id := range group.ItemIDs {
			if name := items[id]; name != "" {
				targets = append(targets, name)
			}
		}
		if len(targets) > 0 {
			perception.RecentTargets = append(perception.RecentTargets, targets)
		}
	}
	return ModelRequest{Player: strings.TrimSpace(player), AvailableActions: AvailableActions, Perception: perception}
}

var ModelPlanSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"actions", "questions", "needsClarification", "clarificationQuestion"},
	"properties": map[string]any{
		"actions": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"action", "target", "item", "direction", "targetMode"},
			"properties": map[string]any{
				"action": map[string]any{"type": "string", "enum": actionNames},
				"target": map[string]any{"type": "string"}, "item": map[string]any{"type": "string"},
				"direction": map[string]any{"type": "string"}, "targetMode": map[string]any{"type": "string", "enum": []any{"single", "all"}},
			},
		}},
		"questions": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"kind", "target", "targetMode"},
			"properties": map[string]any{
				"kind":   map[string]any{"type": "string", "enum": []any{"life_status"}},
				"target": map[string]any{"type": "string"}, "targetMode": map[string]any{"type": "string", "enum": []any{"single", "all"}},
			},
		}},
		"needsClarification":    map[string]any{"type": "boolean"},
		"clarificationQuestion": map[string]any{"type": "string"},
	},
}

func ParseModelPlanJSON(raw, player string) (TurnPlan, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var model ModelPlan
	if err := decoder.Decode(&model); err != nil {
		return TurnPlan{}, fmt.Errorf("decode model plan json: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return TurnPlan{}, errors.New("model plan json contains multiple objects")
	} else if !errors.Is(err, io.EOF) {
		return TurnPlan{}, fmt.Errorf("decode model plan trailing data: %w", err)
	}
	if len(model.Actions) > 4 || len(model.Questions) > 4 {
		return TurnPlan{}, ErrPlanTooLarge
	}
	if model.NeedsClarification && (len(model.Actions) > 0 || len(model.Questions) > 0) {
		return TurnPlan{}, errors.New("clarification plan must not contain actions or questions")
	}

	plan := TurnPlan{
		Actions:               make([]PlannedAction, 0, len(model.Actions)),
		Questions:             append([]FactQuestion(nil), model.Questions...),
		Confidence:            1,
		NeedsClarification:    model.NeedsClarification,
		ClarificationQuestion: strings.TrimSpace(model.ClarificationQuestion),
		RawText:               strings.TrimSpace(player),
	}
	if plan.NeedsClarification {
		plan.Confidence = 0
		if plan.ClarificationQuestion == "" {
			plan.ClarificationQuestion = defaultClarification
		}
		return plan, nil
	}
	for _, action := range model.Actions {
		if !action.Action.Valid() || action.Action == ActionUnknown {
			return TurnPlan{}, fmt.Errorf("invalid model action %q", action.Action)
		}
		if !validTargetMode(action.TargetMode) {
			return TurnPlan{}, fmt.Errorf("invalid target mode %q", action.TargetMode)
		}
		if action.TargetMode == TargetAll && action.Action != ActionInspect && action.Action != ActionSearch {
			return TurnPlan{}, fmt.Errorf("action %q cannot target all", action.Action)
		}
		if action.Action == ActionMove && strings.TrimSpace(action.Direction) == "" {
			return TurnPlan{}, errors.New("move action requires direction")
		}
		if action.Action == ActionExplore && (strings.TrimSpace(action.Target) != "" || strings.TrimSpace(action.Item) != "" || strings.TrimSpace(action.Direction) != "") {
			return TurnPlan{}, errors.New("explore action must not name a target")
		}
		plan.Actions = append(plan.Actions, PlannedAction{Intent: Intent{
			Action: action.Action, Target: strings.TrimSpace(action.Target), Item: strings.TrimSpace(action.Item),
			Direction: strings.TrimSpace(action.Direction), Modifiers: []string{}, Confidence: 1, RawText: plan.RawText,
		}, TargetMode: action.TargetMode})
	}
	for i := range plan.Questions {
		if plan.Questions[i].Kind != game.FactLifeStatus || !validTargetMode(plan.Questions[i].TargetMode) {
			return TurnPlan{}, errors.New("invalid model question")
		}
	}
	if len(plan.Actions) == 0 && len(plan.Questions) == 0 {
		return TurnPlan{}, errors.New("empty model plan without clarification")
	}
	return plan, nil
}
