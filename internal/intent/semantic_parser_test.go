package intent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"kaya/internal/game"
)

type semanticGeneratorResult struct {
	response string
	err      error
}

type semanticGeneratorCall struct {
	systemPrompt string
	userPrompt   string
	schema       any
}

type semanticRecordingGenerator struct {
	results []semanticGeneratorResult
	calls   []semanticGeneratorCall
}

func (f *semanticRecordingGenerator) GenerateJSON(_ context.Context, systemPrompt, userPrompt string, schema any) (string, error) {
	f.calls = append(f.calls, semanticGeneratorCall{
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
		schema:       schema,
	})
	if len(f.results) == 0 {
		return "", errors.New("missing semantic generator result")
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result.response, result.err
}

func TestParseSemanticCompilesValidFirstPassWithOneCall(t *testing.T) {
	message := "search the desk"
	raw := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "search",
		TargetMention: "the desk",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{{response: raw}}}

	plan, provenance, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), message, game.PerceptionSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(generator.calls) != 1 {
		t.Fatalf("generator calls = %d, want 1", len(generator.calls))
	}
	if !reflect.DeepEqual(generator.calls[0].schema, ModelTurnPlanSchema) {
		t.Fatalf("schema = %#v, want ModelTurnPlanSchema", generator.calls[0].schema)
	}
	if provenance.Source != ParseSourceModel || !provenance.HasRawPlan {
		t.Fatalf("provenance = %#v, want first-pass model provenance", provenance)
	}
	if len(provenance.ValidationErrors) != 0 || provenance.RepairReason != nil || provenance.FallbackError != nil {
		t.Fatalf("provenance = %#v, want clean first-pass provenance", provenance)
	}
	search, ok := onlySemanticAction(t, plan).(SearchAction)
	if !ok {
		t.Fatalf("action = %T, want SearchAction", onlySemanticAction(t, plan))
	}
	if search.Target.Mention != "the desk" || search.Target.Quantity != TargetOne {
		t.Fatalf("search = %#v", search)
	}
}

func TestParseSemanticRepairsContractFailureOnce(t *testing.T) {
	message := "search the desk"
	invalid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "search",
		TargetMention: "the desk",
		ItemMention:   "flashlight",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	valid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "search",
		TargetMention: "the desk",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{
		{response: invalid},
		{response: valid},
	}}

	plan, provenance, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), message, game.PerceptionSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(generator.calls) != 2 {
		t.Fatalf("generator calls = %d, want 2", len(generator.calls))
	}
	if provenance.Source != ParseSourceRepair || !provenance.HasRawPlan {
		t.Fatalf("provenance = %#v, want repair provenance", provenance)
	}
	if provenance.RepairReason == nil || provenance.FallbackError != nil {
		t.Fatalf("provenance = %#v, want repair reason without fallback error", provenance)
	}
	if len(provenance.ValidationErrors) != 0 {
		t.Fatalf("terminal validation errors = %#v, want successful repair", provenance.ValidationErrors)
	}
	if !provenance.HasInitialRawPlan || len(provenance.InitialRawPlan.Actions) != 1 {
		t.Fatalf("initial provenance = %#v, want rejected initial DTO", provenance)
	}
	requireSemanticProblem(t, provenance.InitialValidationErrors, "itemMention", "forbidden_slot")
	if _, ok := onlySemanticAction(t, plan).(SearchAction); !ok {
		t.Fatalf("action = %T, want SearchAction", onlySemanticAction(t, plan))
	}
}

func TestParseSemanticRepairPayloadIncludesMessageAndValidationErrors(t *testing.T) {
	message := "search the desk"
	invalid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "search",
		TargetMention: "the desk",
		ItemMention:   "flashlight",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	valid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "search",
		TargetMention: "the desk",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{
		{response: invalid},
		{response: valid},
	}}

	_, _, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), message, game.PerceptionSnapshot{RoomName: "Atrium"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(generator.calls) != 2 {
		t.Fatalf("generator calls = %d, want 2", len(generator.calls))
	}
	if generator.calls[1].systemPrompt != RepairPrompt {
		t.Fatalf("repair system prompt = %q", generator.calls[1].systemPrompt)
	}
	if !reflect.DeepEqual(generator.calls[1].schema, ModelTurnPlanSchema) {
		t.Fatalf("repair schema = %#v, want ModelTurnPlanSchema", generator.calls[1].schema)
	}
	var payload struct {
		Player           string                  `json:"player"`
		Perception       game.PerceptionSnapshot `json:"perception"`
		RejectedPlan     ModelTurnPlan           `json:"rejectedPlan"`
		HasRejectedPlan  bool                    `json:"hasRejectedPlan"`
		ValidationErrors []ValidationError       `json:"validationErrors"`
	}
	if err := json.Unmarshal([]byte(generator.calls[1].userPrompt), &payload); err != nil {
		t.Fatalf("decode repair payload: %v\npayload: %s", err, generator.calls[1].userPrompt)
	}
	if payload.Player != message || payload.Perception.RoomName != "Atrium" {
		t.Fatalf("repair payload = %#v", payload)
	}
	if !payload.HasRejectedPlan || len(payload.RejectedPlan.Actions) != 1 {
		t.Fatalf("repair payload rejected plan = %#v", payload)
	}
	requireSemanticProblem(t, payload.ValidationErrors, "itemMention", "forbidden_slot")
}

func TestParseSemanticInvalidRepairBecomesClarification(t *testing.T) {
	message := "search the desk"
	initialInvalid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "search",
		TargetMention: "the desk",
		ItemMention:   "flashlight",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	repairInvalid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "move",
		TargetMention: "the hall",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{
		{response: initialInvalid},
		{response: repairInvalid},
		{response: semanticModelPlanJSON(t, message, ModelAction{Kind: "wait", Evidence: message, Quantity: TargetOne})},
	}}

	plan, provenance, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), message, game.PerceptionSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(generator.calls) != 2 {
		t.Fatalf("generator calls = %d, want hard maximum of 2", len(generator.calls))
	}
	if len(plan.Actions) != 0 || !plan.NeedsClarification || plan.ClarificationQuestion == "" {
		t.Fatalf("plan = %#v, want safe clarification", plan)
	}
	if provenance.Source != ParseSourceFallback || provenance.RepairReason == nil || provenance.FallbackError == nil {
		t.Fatalf("provenance = %#v, want failed-repair fallback provenance", provenance)
	}
	if !provenance.HasRawPlan || len(provenance.RawPlan.Actions) != 1 || provenance.RawPlan.Actions[0].Kind != "move" {
		t.Fatalf("terminal raw plan = %#v, want invalid repair DTO", provenance.RawPlan)
	}
	requireSemanticProblem(t, provenance.ValidationErrors, "direction", "required_slot")
	if !provenance.HasInitialRawPlan || len(provenance.InitialRawPlan.Actions) != 1 || provenance.InitialRawPlan.Actions[0].Kind != "search" {
		t.Fatalf("initial raw plan = %#v, want invalid first DTO", provenance.InitialRawPlan)
	}
	requireSemanticProblem(t, provenance.InitialValidationErrors, "itemMention", "forbidden_slot")
}

func TestParseSemanticRepairsMalformedFirstResponse(t *testing.T) {
	message := "wait"
	valid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:     "wait",
		Evidence: message,
		Quantity: TargetOne,
	})
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{
		{response: `{"actions":`},
		{response: valid},
	}}

	plan, provenance, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), message, game.PerceptionSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(generator.calls) != 2 {
		t.Fatalf("generator calls = %d, want 2", len(generator.calls))
	}
	for index, call := range generator.calls {
		if !reflect.DeepEqual(call.schema, ModelTurnPlanSchema) {
			t.Fatalf("call %d schema = %#v, want ModelTurnPlanSchema", index, call.schema)
		}
	}
	if provenance.Source != ParseSourceRepair || !provenance.HasRawPlan || len(provenance.ValidationErrors) != 0 {
		t.Fatalf("provenance = %#v, want successful terminal repair attempt", provenance)
	}
	if provenance.HasInitialRawPlan {
		t.Fatalf("provenance = %#v, malformed initial response cannot have a raw DTO", provenance)
	}
	requireSemanticProblem(t, provenance.InitialValidationErrors, "plan", "decode_error")
	if _, ok := onlySemanticAction(t, plan).(WaitAction); !ok {
		t.Fatalf("action = %T, want WaitAction", onlySemanticAction(t, plan))
	}

	var payload struct {
		HasRejectedPlan  bool              `json:"hasRejectedPlan"`
		ValidationErrors []ValidationError `json:"validationErrors"`
	}
	if err := json.Unmarshal([]byte(generator.calls[1].userPrompt), &payload); err != nil {
		t.Fatalf("decode repair payload: %v", err)
	}
	if payload.HasRejectedPlan {
		t.Fatalf("repair payload = %#v, malformed response cannot have a rejected DTO", payload)
	}
	requireSemanticProblem(t, payload.ValidationErrors, "plan", "decode_error")
}

func TestParseSemanticMalformedRepairClarifiesWithoutThirdCall(t *testing.T) {
	message := "search the desk"
	initialInvalid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "search",
		TargetMention: "the desk",
		ItemMention:   "flashlight",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{
		{response: initialInvalid},
		{response: `{"actions":`},
		{response: semanticModelPlanJSON(t, message, ModelAction{Kind: "wait", Evidence: message, Quantity: TargetOne})},
	}}

	plan, provenance, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), message, game.PerceptionSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(generator.calls) != 2 {
		t.Fatalf("generator calls = %d, want hard maximum of 2", len(generator.calls))
	}
	for index, call := range generator.calls {
		if !reflect.DeepEqual(call.schema, ModelTurnPlanSchema) {
			t.Fatalf("call %d schema = %#v, want ModelTurnPlanSchema", index, call.schema)
		}
	}
	if len(plan.Actions) != 0 || !plan.NeedsClarification || plan.ClarificationQuestion == "" {
		t.Fatalf("plan = %#v, want safe clarification", plan)
	}
	if provenance.Source != ParseSourceFallback || provenance.HasRawPlan {
		t.Fatalf("provenance = %#v, malformed terminal repair cannot have a raw DTO", provenance)
	}
	requireSemanticProblem(t, provenance.ValidationErrors, "plan", "decode_error")
	if !provenance.HasInitialRawPlan || len(provenance.InitialRawPlan.Actions) != 1 {
		t.Fatalf("initial provenance = %#v, want rejected initial DTO", provenance)
	}
	requireSemanticProblem(t, provenance.InitialValidationErrors, "itemMention", "forbidden_slot")
	if provenance.RepairReason == nil || provenance.FallbackError == nil {
		t.Fatalf("provenance = %#v, want both lifecycle errors", provenance)
	}
}

func TestParseSemanticRepairGenerationErrorHasNoTerminalAttempt(t *testing.T) {
	message := "search the desk"
	initialInvalid := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "search",
		TargetMention: "the desk",
		ItemMention:   "flashlight",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{
		{response: initialInvalid},
		{err: errors.New("repair model unavailable")},
		{response: semanticModelPlanJSON(t, message, ModelAction{Kind: "wait", Evidence: message, Quantity: TargetOne})},
	}}

	plan, provenance, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), message, game.PerceptionSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(generator.calls) != 2 {
		t.Fatalf("generator calls = %d, want hard maximum of 2", len(generator.calls))
	}
	for index, call := range generator.calls {
		if !reflect.DeepEqual(call.schema, ModelTurnPlanSchema) {
			t.Fatalf("call %d schema = %#v, want ModelTurnPlanSchema", index, call.schema)
		}
	}
	if len(plan.Actions) != 0 || !plan.NeedsClarification || plan.ClarificationQuestion == "" {
		t.Fatalf("plan = %#v, want safe clarification", plan)
	}
	if provenance.Source != ParseSourceFallback || provenance.HasRawPlan || len(provenance.ValidationErrors) != 0 {
		t.Fatalf("terminal provenance = %#v, repair generation produced no terminal attempt", provenance)
	}
	if !provenance.HasInitialRawPlan || len(provenance.InitialRawPlan.Actions) != 1 {
		t.Fatalf("initial provenance = %#v, want rejected initial DTO", provenance)
	}
	requireSemanticProblem(t, provenance.InitialValidationErrors, "itemMention", "forbidden_slot")
	if provenance.RepairReason == nil || provenance.FallbackError == nil {
		t.Fatalf("provenance = %#v, want initial rejection and repair generation errors", provenance)
	}
}

func TestParseSemanticGeneratorFailureDoesNotAttemptRepair(t *testing.T) {
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{
		{err: errors.New("model unavailable")},
		{response: semanticModelPlanJSON(t, "wait", ModelAction{Kind: "wait", Evidence: "wait", Quantity: TargetOne})},
	}}

	plan, provenance, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), "wait", game.PerceptionSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(generator.calls) != 1 {
		t.Fatalf("generator calls = %d, want 1 because generation failures are not repairable", len(generator.calls))
	}
	if !plan.NeedsClarification || provenance.Source != ParseSourceFallback || provenance.FallbackError == nil {
		t.Fatalf("plan = %#v, provenance = %#v", plan, provenance)
	}
}

func TestParseSemanticDoesNotUseContextualPhraseNormalization(t *testing.T) {
	message := "go east"
	raw := semanticModelPlanJSON(t, message, ModelAction{
		Kind:          "explore",
		TargetMention: "room",
		Evidence:      message,
		Quantity:      TargetOne,
	})
	generator := &semanticRecordingGenerator{results: []semanticGeneratorResult{{response: raw}}}

	plan, _, err := NewParser(generator).ParseSemanticWithProvenance(
		context.Background(), message, game.PerceptionSnapshot{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := onlySemanticAction(t, plan).(ExploreAction); !ok {
		t.Fatalf("action = %T, want model's typed ExploreAction unchanged", onlySemanticAction(t, plan))
	}
}

func semanticModelPlanJSON(t *testing.T, message string, actions ...ModelAction) string {
	t.Helper()
	raw, err := json.Marshal(ModelTurnPlan{
		Actions:               actions,
		Questions:             []ModelFactQuestion{},
		RawText:               message,
		NeedsClarification:    false,
		ClarificationQuestion: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func onlySemanticAction(t *testing.T, plan SemanticPlan) SemanticAction {
	t.Helper()
	if len(plan.Actions) != 1 {
		t.Fatalf("actions = %#v, want exactly one", plan.Actions)
	}
	return plan.Actions[0]
}

func requireSemanticProblem(t *testing.T, problems []ValidationError, field, code string) {
	t.Helper()
	for _, problem := range problems {
		if problem.Field == field && problem.Code == code {
			return
		}
	}
	t.Fatalf("problems = %#v, want field=%q code=%q", problems, field, code)
}
