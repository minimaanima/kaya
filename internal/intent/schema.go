package intent

var actionNames = []any{"unknown", "move", "inspect", "search", "take_item", "use_item", "talk", "wait", "hide", "listen", "throw", "force_open", "turn_on", "turn_off", "explore"}

var embeddedIntentSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"action", "target", "item", "direction", "modifiers", "confidence", "rawText", "needsClarification", "clarificationQuestion"},
	"properties": map[string]any{
		"action": map[string]any{"type": "string", "enum": actionNames},
		"target": map[string]any{"type": "string"}, "item": map[string]any{"type": "string"}, "direction": map[string]any{"type": "string"},
		"modifiers":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "rawText": map[string]any{"type": "string"},
		"needsClarification": map[string]any{"type": "boolean"}, "clarificationQuestion": map[string]any{"type": "string"},
	},
}

var TurnPlanSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"actions", "questions", "confidence", "needsClarification", "clarificationQuestion", "rawText"},
	"properties": map[string]any{
		"actions":    map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"intent", "targetMode"}, "properties": map[string]any{"intent": embeddedIntentSchema, "targetMode": map[string]any{"type": "string", "enum": []any{"single", "all"}}}}},
		"questions":  map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "target", "targetMode"}, "properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []any{"life_status"}}, "target": map[string]any{"type": "string"}, "targetMode": map[string]any{"type": "string", "enum": []any{"single", "all"}}}}},
		"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "needsClarification": map[string]any{"type": "boolean"},
		"clarificationQuestion": map[string]any{"type": "string"}, "rawText": map[string]any{"type": "string"},
	},
}

var modelActionKinds = []any{"move", "inspect", "search", "take", "use", "toggle", "wait", "talk", "listen", "explore"}

var ModelTurnPlanSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"actions", "questions", "rawText", "needsClarification", "clarificationQuestion"},
	"properties": map[string]any{
		"actions": map[string]any{
			"type": "array", "maxItems": 4,
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"kind", "targetMention", "itemMention", "direction", "state", "evidence", "quantity"},
				"properties": map[string]any{
					"kind":          map[string]any{"type": "string", "enum": modelActionKinds},
					"targetMention": map[string]any{"type": "string"},
					"itemMention":   map[string]any{"type": "string"},
					"direction":     map[string]any{"type": "string"},
					"state":         map[string]any{"type": "string", "enum": []any{"", "on", "off"}},
					"evidence":      map[string]any{"type": "string"},
					"quantity":      map[string]any{"type": "string", "enum": []any{"one", "all"}},
				},
			},
		},
		"questions": map[string]any{
			"type": "array", "maxItems": 4,
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"kind", "targetMention", "quantity"},
				"properties": map[string]any{
					"kind":          map[string]any{"type": "string", "enum": []any{"life_status"}},
					"targetMention": map[string]any{"type": "string"},
					"quantity":      map[string]any{"type": "string", "enum": []any{"one", "all"}},
				},
			},
		},
		"rawText":               map[string]any{"type": "string"},
		"needsClarification":    map[string]any{"type": "boolean"},
		"clarificationQuestion": map[string]any{"type": "string"},
	},
}
