package intent

import (
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
