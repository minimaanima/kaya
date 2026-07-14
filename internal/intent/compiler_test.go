package intent

import (
	"encoding/json"
	"testing"
)

func TestCompileModelPlanBuildsTypedUse(t *testing.T) {
	model := ModelTurnPlan{Actions: []ModelAction{{
		Kind:          "use",
		ItemMention:   "the key",
		TargetMention: "that door",
		Quantity:      "one",
		Evidence:      "use the key to unlock that door",
	}}}

	plan, problems := CompileModelPlan("use the key to unlock that door", model)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(plan.Actions))
	}
	got, ok := plan.Actions[0].(UseAction)
	if !ok {
		t.Fatalf("action type = %T, want UseAction", plan.Actions[0])
	}
	if got.Item != (Reference{Mention: "the key", Quantity: "one"}) {
		t.Fatalf("item = %#v", got.Item)
	}
	if got.Target != (Reference{Mention: "that door", Quantity: "one"}) {
		t.Fatalf("target = %#v", got.Target)
	}
	if got.ActionKind() != ActionUseItem || got.SourceEvidence() != "use the key to unlock that door" {
		t.Fatalf("typed action = %#v", got)
	}
}

func TestCompileModelPlanBuildsCompoundActionsWithDistinctEvidence(t *testing.T) {
	message := "go north, then inspect the desk"
	model := ModelTurnPlan{
		Actions: []ModelAction{
			{Kind: "move", Direction: "north", Quantity: "one", Evidence: "go north"},
			{Kind: "inspect", TargetMention: "the desk", Quantity: "one", Evidence: "inspect the desk"},
		},
		RawText: "raw model text",
	}

	plan, problems := CompileModelPlan(message, model)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(plan.Actions))
	}
	if _, ok := plan.Actions[0].(MoveAction); !ok {
		t.Fatalf("first action type = %T", plan.Actions[0])
	}
	if _, ok := plan.Actions[1].(InspectAction); !ok {
		t.Fatalf("second action type = %T", plan.Actions[1])
	}
	if plan.RawText != model.RawText {
		t.Fatalf("raw text = %q", plan.RawText)
	}
}

func TestCompileModelPlanRejectsMissingRequiredSlots(t *testing.T) {
	tests := []struct {
		name   string
		action ModelAction
		field  string
	}{
		{name: "move direction", action: ModelAction{Kind: "move", Evidence: "move"}, field: "direction"},
		{name: "search target", action: ModelAction{Kind: "search", Evidence: "search"}, field: "targetMention"},
		{name: "take target", action: ModelAction{Kind: "take", Evidence: "take"}, field: "targetMention"},
		{name: "use item", action: ModelAction{Kind: "use", TargetMention: "door", Evidence: "use door"}, field: "itemMention"},
		{name: "use target", action: ModelAction{Kind: "use", ItemMention: "key", Evidence: "use key"}, field: "targetMention"},
		{name: "toggle item", action: ModelAction{Kind: "toggle", State: "on", Evidence: "turn on"}, field: "itemMention"},
		{name: "toggle state", action: ModelAction{Kind: "toggle", ItemMention: "lamp", Evidence: "toggle lamp"}, field: "state"},
		{name: "talk target", action: ModelAction{Kind: "talk", Evidence: "talk"}, field: "targetMention"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, problems := CompileModelPlan(tt.action.Evidence, ModelTurnPlan{Actions: []ModelAction{tt.action}})
			requireProblem(t, problems, tt.field, "required_slot")
		})
	}
}

func TestCompileModelPlanRejectsForbiddenSlots(t *testing.T) {
	tests := []struct {
		name   string
		action ModelAction
		field  string
	}{
		{
			name:   "item on search",
			action: ModelAction{Kind: "search", TargetMention: "desk", ItemMention: "flashlight", Evidence: "search the desk"},
			field:  "itemMention",
		},
		{
			name:   "direction on use",
			action: ModelAction{Kind: "use", TargetMention: "door", ItemMention: "key", Direction: "north", Evidence: "use the key"},
			field:  "direction",
		},
		{
			name:   "target on wait",
			action: ModelAction{Kind: "wait", TargetMention: "desk", Evidence: "wait"},
			field:  "targetMention",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, problems := CompileModelPlan(tt.action.Evidence, ModelTurnPlan{Actions: []ModelAction{tt.action}})
			requireProblem(t, problems, tt.field, "forbidden_slot")
		})
	}
}

func TestCompileModelPlanRejectsUnsupportedKind(t *testing.T) {
	_, problems := CompileModelPlan("hide", ModelTurnPlan{Actions: []ModelAction{{Kind: "hide", Evidence: "hide"}}})
	requireProblem(t, problems, "kind", "unsupported_kind")
}

func TestCompileModelPlanRejectsEvidenceAbsentFromMessage(t *testing.T) {
	_, problems := CompileModelPlan("inspect the desk", ModelTurnPlan{Actions: []ModelAction{{
		Kind: "inspect", TargetMention: "desk", Evidence: "inspect the cabinet",
	}}})
	requireProblem(t, problems, "evidence", "evidence_not_found")
}

func TestCompileModelPlanRejectsDuplicateNormalizedEvidence(t *testing.T) {
	model := ModelTurnPlan{Actions: []ModelAction{
		{Kind: "inspect", TargetMention: "desk", Evidence: "inspect   the desk"},
		{Kind: "search", TargetMention: "desk", Evidence: "inspect the desk"},
	}}

	_, problems := CompileModelPlan("inspect the desk", model)
	requireProblem(t, problems, "evidence", "duplicate_evidence")
}

func TestCompileModelPlanNormalizesWhitespaceForEvidenceCoverage(t *testing.T) {
	model := ModelTurnPlan{Actions: []ModelAction{{
		Kind: "inspect", TargetMention: "desk", Evidence: "inspect   the desk",
	}}}

	plan, problems := CompileModelPlan("please inspect\n\tthe desk", model)
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(plan.Actions))
	}
}

func TestParseModelPlanJSONDecodesModelShape(t *testing.T) {
	raw := `{
		"actions": [{
			"kind": "use",
			"targetMention": "door",
			"itemMention": "key",
			"direction": "",
			"state": "",
			"evidence": "use key on door",
			"quantity": "one"
		}],
		"questions": [],
		"rawText": "use key on door",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`

	got, err := ParseModelPlanJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Actions) != 1 || got.Actions[0].Kind != "use" || got.Actions[0].Quantity != "one" {
		t.Fatalf("decoded plan = %#v", got)
	}
}

func TestParseModelPlanJSONRejectsInvalidShape(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid JSON", raw: `{"actions": [`},
		{name: "unknown field", raw: `{"actions":[],"questions":[],"rawText":"","needsClarification":false,"clarificationQuestion":"","worldID":"secret"}`},
		{name: "trailing JSON", raw: `{"actions":[],"questions":[],"rawText":"","needsClarification":false,"clarificationQuestion":""} {}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseModelPlanJSON(tt.raw); err == nil {
				t.Fatal("expected decode error")
			}
		})
	}
}

func TestParseModelPlanJSONRejectsMissingRequiredProperties(t *testing.T) {
	for _, field := range []string{"actions", "questions", "rawText", "needsClarification", "clarificationQuestion"} {
		t.Run("top level "+field, func(t *testing.T) {
			document := validModelPlanDocument()
			delete(document, field)
			requireModelPlanDecodeError(t, document)
		})
	}

	for _, field := range []string{"kind", "targetMention", "itemMention", "direction", "state", "evidence", "quantity"} {
		t.Run("action "+field, func(t *testing.T) {
			document := validModelPlanDocument()
			action := document["actions"].([]any)[0].(map[string]any)
			delete(action, field)
			requireModelPlanDecodeError(t, document)
		})
	}

	for _, field := range []string{"kind", "targetMention", "quantity"} {
		t.Run("question "+field, func(t *testing.T) {
			document := validModelPlanDocument()
			document["questions"] = []any{validModelQuestionDocument()}
			question := document["questions"].([]any)[0].(map[string]any)
			delete(question, field)
			requireModelPlanDecodeError(t, document)
		})
	}
}

func TestParseModelPlanJSONRejectsNullAndWrongShapes(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		mutate func(map[string]any)
	}{
		{name: "null document", raw: "null"},
		{name: "null actions", mutate: func(document map[string]any) { document["actions"] = nil }},
		{name: "null questions", mutate: func(document map[string]any) { document["questions"] = nil }},
		{name: "actions object", mutate: func(document map[string]any) { document["actions"] = map[string]any{} }},
		{name: "questions object", mutate: func(document map[string]any) { document["questions"] = map[string]any{} }},
		{name: "null action", mutate: func(document map[string]any) { document["actions"] = []any{nil} }},
		{name: "null question", mutate: func(document map[string]any) { document["questions"] = []any{nil} }},
		{name: "null raw text", mutate: func(document map[string]any) { document["rawText"] = nil }},
		{name: "numeric raw text", mutate: func(document map[string]any) { document["rawText"] = 7 }},
		{name: "string clarification flag", mutate: func(document map[string]any) { document["needsClarification"] = "false" }},
		{name: "numeric action evidence", mutate: func(document map[string]any) {
			document["actions"].([]any)[0].(map[string]any)["evidence"] = 7
		}},
		{name: "array question target", mutate: func(document map[string]any) {
			document["questions"] = []any{validModelQuestionDocument()}
			document["questions"].([]any)[0].(map[string]any)["targetMention"] = []any{}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.raw != "" {
				if _, err := ParseModelPlanJSON(tt.raw); err == nil {
					t.Fatal("expected decode error")
				}
				return
			}
			document := validModelPlanDocument()
			tt.mutate(document)
			requireModelPlanDecodeError(t, document)
		})
	}
}

func TestParseModelPlanJSONRejectsInvalidEnumsAndQuestionTargets(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "action quantity", mutate: func(document map[string]any) {
			document["actions"].([]any)[0].(map[string]any)["quantity"] = "banana"
		}},
		{name: "action state", mutate: func(document map[string]any) {
			document["actions"].([]any)[0].(map[string]any)["state"] = "maybe"
		}},
		{name: "question kind", mutate: func(document map[string]any) {
			document["questions"] = []any{validModelQuestionDocument()}
			document["questions"].([]any)[0].(map[string]any)["kind"] = "weather"
		}},
		{name: "question quantity", mutate: func(document map[string]any) {
			document["questions"] = []any{validModelQuestionDocument()}
			document["questions"].([]any)[0].(map[string]any)["quantity"] = "banana"
		}},
		{name: "empty question target", mutate: func(document map[string]any) {
			document["questions"] = []any{validModelQuestionDocument()}
			document["questions"].([]any)[0].(map[string]any)["targetMention"] = ""
		}},
		{name: "whitespace question target", mutate: func(document map[string]any) {
			document["questions"] = []any{validModelQuestionDocument()}
			document["questions"].([]any)[0].(map[string]any)["targetMention"] = "  \t"
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := validModelPlanDocument()
			tt.mutate(document)
			requireModelPlanDecodeError(t, document)
		})
	}
}

func TestParseModelPlanJSONRejectsTooManyActionsAndQuestions(t *testing.T) {
	t.Run("actions", func(t *testing.T) {
		document := validModelPlanDocument()
		actions := make([]any, 5)
		for index := range actions {
			actions[index] = validModelActionDocument()
		}
		document["actions"] = actions
		requireModelPlanDecodeError(t, document)
	})

	t.Run("questions", func(t *testing.T) {
		document := validModelPlanDocument()
		questions := make([]any, 5)
		for index := range questions {
			questions[index] = validModelQuestionDocument()
		}
		document["questions"] = questions
		requireModelPlanDecodeError(t, document)
	})
}

func FuzzCompileModelPlanNeverPanics(f *testing.F) {
	f.Add("look around", "inspect", "look around")
	f.Fuzz(func(t *testing.T, message, kind, evidence string) {
		CompileModelPlan(message, ModelTurnPlan{Actions: []ModelAction{{Kind: kind, Evidence: evidence}}})
	})
}

func requireProblem(t *testing.T, problems []ValidationError, field, code string) {
	t.Helper()
	for _, problem := range problems {
		if problem.Field == field && problem.Code == code {
			return
		}
	}
	t.Fatalf("missing problem field=%q code=%q in %#v", field, code, problems)
}

func validModelPlanDocument() map[string]any {
	return map[string]any{
		"actions":               []any{validModelActionDocument()},
		"questions":             []any{},
		"rawText":               "inspect the desk",
		"needsClarification":    false,
		"clarificationQuestion": "",
	}
}

func validModelActionDocument() map[string]any {
	return map[string]any{
		"kind":          "inspect",
		"targetMention": "the desk",
		"itemMention":   "",
		"direction":     "",
		"state":         "",
		"evidence":      "inspect the desk",
		"quantity":      "one",
	}
}

func validModelQuestionDocument() map[string]any {
	return map[string]any{
		"kind":          "life_status",
		"targetMention": "the doctors",
		"quantity":      "all",
	}
}

func requireModelPlanDecodeError(t *testing.T, document map[string]any) {
	t.Helper()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseModelPlanJSON(string(raw)); err == nil {
		t.Fatalf("expected decode error for %s", raw)
	}
}
