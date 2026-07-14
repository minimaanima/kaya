package intent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"kaya/internal/game"
)

type fakeGenerator struct {
	responses []string
	err       error
	calls     int
}

func (f *fakeGenerator) Generate(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	if len(f.responses) == 0 {
		return "", errors.New("missing fake response")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func (f *fakeGenerator) GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, schema any) (string, error) {
	return f.Generate(ctx, systemPrompt, userPrompt)
}

func TestParserProvenanceReportsGeneratorFallback(t *testing.T) {
	parser := NewParser(&fakeGenerator{err: errors.New("ollama unreachable")})
	plan, provenance, err := parser.ParseWithProvenance(context.Background(), "go east", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatalf("ParseWithProvenance returned gameplay error: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != ActionMove {
		t.Fatalf("plan = %#v, want deterministic gameplay fallback", plan)
	}
	if provenance.Source != ParseSourceFallback || provenance.FallbackError == nil {
		t.Fatalf("provenance = %#v, want failing generator fallback", provenance)
	}
	if provenance.HasRawPlan {
		t.Fatalf("provenance = %#v, failed generation cannot have a raw model plan", provenance)
	}
}

func TestParserProvenanceSeparatesRawAndResolvedPlans(t *testing.T) {
	message := "go east"
	wrong := modelAction(ActionExplore, "room", "", "")
	parser := NewParser(&fakeGenerator{responses: []string{modelPlanJSON(t, message, wrong)}})

	resolved, provenance, err := parser.ParseWithProvenance(context.Background(), message, game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.Source != ParseSourceModel || !provenance.HasRawPlan || provenance.FallbackError != nil {
		t.Fatalf("provenance = %#v, want successfully decoded model source", provenance)
	}
	if len(provenance.RawPlan.Actions) != 1 || provenance.RawPlan.Actions[0].Intent.Action != ActionExplore {
		t.Fatalf("raw plan = %#v, want the model's wrong explore action", provenance.RawPlan)
	}
	if len(resolved.Actions) != 1 || resolved.Actions[0].Intent.Action != ActionMove || resolved.Actions[0].Intent.Direction != "east" {
		t.Fatalf("resolved plan = %#v, want canonical move east", resolved)
	}
	if !provenance.Canonicalized {
		t.Fatalf("provenance = %#v, want canonicalization recorded", provenance)
	}
}

func TestParserParsesPluralCompoundTurn(t *testing.T) {
	generator := &fakeGenerator{responses: []string{`{
		"actions":[{"intent":{"action":"search","target":"doctors","item":"","direction":"","modifiers":[],"confidence":0.96,"rawText":"search the doctors","needsClarification":false,"clarificationQuestion":""},"targetMode":"all"}],
		"questions":[{"kind":"life_status","target":"they","targetMode":"all"}],
		"confidence":0.96,"needsClarification":false,"clarificationQuestion":"","rawText":"search the doctors are they dead"
	}`}}
	parser := NewParser(generator)
	plan, err := parser.Parse(context.Background(), "search the doctors are they dead", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].TargetMode != TargetAll || len(plan.Questions) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestFallbackPlanExploresWalls(t *testing.T) {
	plan := FallbackPlan("feel along the walls for another exit")
	if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != ActionExplore {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestFallbackPlanParsesThrowAtTarget(t *testing.T) {
	plan := FallbackPlan("throw the brick at the window")
	if len(plan.Actions) != 1 {
		t.Fatalf("actions = %#v, want one throw action", plan.Actions)
	}
	got := plan.Actions[0].Intent
	if got.Action != ActionThrow || got.Item != "brick" || got.Target != "window" {
		t.Fatalf("intent = %#v, want throw item=brick target=window", got)
	}
}

func TestCanonicalFallbackRequiresCompleteThrowFields(t *testing.T) {
	if !isCanonicalFallback(FallbackPlan("throw the brick at the window"), "throw the brick at the window") {
		t.Fatal("complete throw fallback should be canonical")
	}
	if isCanonicalFallback(FallbackPlan("throw the brick toward the window"), "throw the brick toward the window") {
		t.Fatal("throw fallback without item and target should not be canonical")
	}
}

func TestFallbackPlanExtractsObjectTargets(t *testing.T) {
	tests := []struct {
		message string
		action  Action
		target  string
	}{
		{message: "search the desk", action: ActionSearch, target: "desk"},
		{message: "look through the drawers", action: ActionSearch, target: "drawers"},
		{message: "look inside the drawers", action: ActionSearch, target: "drawers"},
		{message: "search for the desk", action: ActionSearch, target: "desk"},
		{message: "searxch the desk", action: ActionSearch, target: "desk"},
		{message: "grab the flashlight", action: ActionTakeItem, target: "flashlight"},
		{message: "take the flashlight", action: ActionTakeItem, target: "flashlight"},
		{message: "took the key", action: ActionTakeItem, target: "key"},
		{message: "what is on the desk", action: ActionInspect, target: "desk"},
		{message: "look on the desk", action: ActionInspect, target: "desk"},
		{message: "inspect the cabinet", action: ActionInspect, target: "cabinet"},
		{message: "look ath the drawers", action: ActionInspect, target: "drawers"},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			plan := FallbackPlan(tt.message)
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != tt.action {
				t.Fatalf("plan = %#v", plan)
			}
			if got := plan.Actions[0].Intent.Target; got != tt.target {
				t.Fatalf("Target = %q, want %q", got, tt.target)
			}
		})
	}
}

func TestFallbackPlanParsesTryKeyOnDoorAsUseItem(t *testing.T) {
	supported := FallbackPlan("use the key on the emergency stairwell door")
	attempt := FallbackPlan("try the key on the stairwell door")
	if len(supported.Actions) != 1 || len(attempt.Actions) != 1 {
		t.Fatalf("supported=%#v attempt=%#v", supported, attempt)
	}
	want := supported.Actions[0]
	got := attempt.Actions[0]
	if got.Intent.Action != want.Intent.Action || got.Intent.Item != want.Intent.Item || got.Intent.Target != "stairwell door" || got.TargetMode != want.TargetMode {
		t.Fatalf("attempt action=%#v, want use-item target relationship from %#v", got, want)
	}
	if attempt.NeedsClarification {
		t.Fatalf("attempt needs clarification: %#v", attempt)
	}
}

func TestFallbackPlanRoutesNaturalQuestions(t *testing.T) {
	tests := []struct {
		message string
		action  Action
		target  string
		item    string
	}{
		{message: "whats around you", action: ActionInspect},
		{message: "what do you see", action: ActionInspect},
		{message: "what is in the room", action: ActionInspect, target: "room"},
		{message: "is something inside the drawers", action: ActionSearch, target: "drawers"},
		{message: "is there anything in the drawers", action: ActionSearch, target: "drawers"},
		{message: "is there a flashlight", action: ActionTalk, item: "flashlight"},
		{message: "where is the flashlight", action: ActionTalk, item: "flashlight"},
		{message: "tun on the flashlight", action: ActionTurnOn, item: "flashlight"},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			plan := FallbackPlan(tt.message)
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != tt.action {
				t.Fatalf("plan = %#v, want action %s", plan, tt.action)
			}
			if got := plan.Actions[0].Intent.Target; got != tt.target {
				t.Fatalf("Target = %q, want %q", got, tt.target)
			}
			if got := plan.Actions[0].Intent.Item; got != tt.item {
				t.Fatalf("Item = %q, want %q", got, tt.item)
			}
		})
	}
}

func TestFallbackPlanRoutesBothToRememberedPluralSearch(t *testing.T) {
	plan := FallbackPlan("both")

	if len(plan.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1", len(plan.Actions))
	}
	if got := plan.Actions[0].Intent.Action; got != ActionSearch {
		t.Fatalf("Action = %q, want %q", got, ActionSearch)
	}
	if got := plan.Actions[0].Intent.Target; got != "both" {
		t.Fatalf("Target = %q, want both", got)
	}
	if got := plan.Actions[0].TargetMode; got != TargetAll {
		t.Fatalf("TargetMode = %q, want %q", got, TargetAll)
	}
}

func TestFallbackPlanParsesSequentialActions(t *testing.T) {
	tests := []struct {
		message string
		want    []struct {
			action Action
			target string
			item   string
		}
	}{
		{
			message: "search the floor and take the flashlight",
			want: []struct {
				action Action
				target string
				item   string
			}{
				{action: ActionSearch, target: "floor"},
				{action: ActionTakeItem, target: "flashlight"},
			},
		},
		{
			message: "look on the desk then search the drawers",
			want: []struct {
				action Action
				target string
				item   string
			}{
				{action: ActionInspect, target: "desk"},
				{action: ActionSearch, target: "drawers"},
			},
		},
		{
			message: "tun on the flashlight and look around",
			want: []struct {
				action Action
				target string
				item   string
			}{
				{action: ActionTurnOn, item: "flashlight"},
				{action: ActionInspect},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			plan := FallbackPlan(tt.message)
			if len(plan.Actions) != len(tt.want) {
				t.Fatalf("Actions len = %d, want %d: %#v", len(plan.Actions), len(tt.want), plan)
			}
			for i, want := range tt.want {
				got := plan.Actions[i].Intent
				if got.Action != want.action {
					t.Fatalf("action %d = %q, want %q", i, got.Action, want.action)
				}
				if got.Target != want.target {
					t.Fatalf("target %d = %q, want %q", i, got.Target, want.target)
				}
				if got.Item != want.item {
					t.Fatalf("item %d = %q, want %q", i, got.Item, want.item)
				}
			}
		})
	}
}

func TestFallbackPlanRoutesInventoryQuestionsToTalk(t *testing.T) {
	for _, message := range []string{"what is in your bag", "what's in your inventory", "do you have anything"} {
		t.Run(message, func(t *testing.T) {
			plan := FallbackPlan(message)
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != ActionTalk {
				t.Fatalf("plan = %#v, want talk", plan)
			}
			if plan.Actions[0].Intent.Target != "inventory" {
				t.Fatalf("Target = %q, want inventory", plan.Actions[0].Intent.Target)
			}
		})
	}
}

func TestFallbackPlanDoesNotTreatMindQuestionAsInventory(t *testing.T) {
	plan := FallbackPlan("what do you have in mind")
	if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action == ActionTalk || !plan.NeedsClarification {
		t.Fatalf("plan = %#v, want clarification instead of inventory talk", plan)
	}
}

func TestParserNormalizesApprovedContextualPhrases(t *testing.T) {
	valid := func(raw string) string {
		return `{"actions":[{"intent":{"action":"explore","target":"","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"` + raw + `","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"` + raw + `"}`
	}
	tests := []struct {
		name string
		msg  string
		want Action
	}{
		{name: "singular doctor", msg: "inspect the doctor", want: ActionInspect},
		{name: "both doctors", msg: "inspect both doctors", want: ActionInspect},
		{name: "them", msg: "search them", want: ActionSearch},
		{name: "compound", msg: "search the doctors are they dead", want: ActionSearch},
		{name: "walls", msg: "feel along the walls for another exit", want: ActionExplore},
		{name: "cabinet typo", msg: "what is isnide the storage cabiner", want: ActionSearch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := NewParser(&fakeGenerator{responses: []string{valid(tt.msg)}}).Parse(context.Background(), tt.msg, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != tt.want {
				t.Fatalf("plan = %#v", plan)
			}
		})
	}
}

func TestParserUsesFallbackWhenModelClarifiesObviousLocalPhrase(t *testing.T) {
	clarification := func(raw string) string {
		return `{"actions":[],"questions":[],"confidence":0.9,"needsClarification":true,"clarificationQuestion":"What do you want Kaya to do?","rawText":"` + raw + `"}`
	}
	tests := []struct {
		message string
		action  Action
		target  string
		item    string
	}{
		{message: "look ath the drawers", action: ActionInspect, target: "drawers"},
		{message: "is something inside the drawers", action: ActionSearch, target: "drawers"},
		{message: "is there a flashlight", action: ActionTalk, item: "flashlight"},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			plan, err := NewParser(&fakeGenerator{responses: []string{clarification(tt.message)}}).Parse(context.Background(), tt.message, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if plan.NeedsClarification || len(plan.Actions) != 1 {
				t.Fatalf("plan = %#v, want one executable action", plan)
			}
			if got := plan.Actions[0].Intent.Action; got != tt.action {
				t.Fatalf("Action = %q, want %q", got, tt.action)
			}
			if got := plan.Actions[0].Intent.Target; got != tt.target {
				t.Fatalf("Target = %q, want %q", got, tt.target)
			}
			if got := plan.Actions[0].Intent.Item; got != tt.item {
				t.Fatalf("Item = %q, want %q", got, tt.item)
			}
		})
	}
}

func TestParserNormalizesNonObjectTargetModeToSingle(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "move",
			raw:  `{"actions":[{"intent":{"action":"move","target":"","item":"","direction":"north","modifiers":[],"confidence":0.9,"rawText":"go north","needsClarification":false,"clarificationQuestion":""},"targetMode":"all"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"go north"}`,
		},
		{
			name: "turn on",
			raw:  `{"actions":[{"intent":{"action":"turn_on","target":"","item":"flashlight","direction":"","modifiers":[],"confidence":0.9,"rawText":"turn on the flashlight","needsClarification":false,"clarificationQuestion":""},"targetMode":"all"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"turn on the flashlight"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := NewParser(&fakeGenerator{responses: []string{tt.raw}}).Parse(context.Background(), tt.name, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Actions) != 1 {
				t.Fatalf("Actions len = %d, want 1", len(plan.Actions))
			}
			if got := plan.Actions[0].TargetMode; got != TargetSingle {
				t.Fatalf("TargetMode = %q, want %q", got, TargetSingle)
			}
		})
	}
}

func TestParserLocalCommandsOverrideWrongModelPlan(t *testing.T) {
	wrongExplore := func(raw string) string {
		action := `{"intent":{"action":"explore","target":"","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"` + raw + `","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}`
		return `{"actions":[` + action + `,` + action + `,` + action + `,` + action + `],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"` + raw + `"}`
	}
	tests := []struct {
		message string
		action  Action
		target  string
	}{
		{message: "look on the desk", action: ActionInspect, target: "desk"},
		{message: "searxch the desk", action: ActionSearch, target: "desk"},
		{message: "search the desk", action: ActionSearch, target: "desk"},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			plan, err := NewParser(&fakeGenerator{responses: []string{wrongExplore(tt.message)}}).Parse(context.Background(), tt.message, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Actions) != 1 {
				t.Fatalf("Actions len = %d, want 1: %#v", len(plan.Actions), plan)
			}
			if got := plan.Actions[0].Intent.Action; got != tt.action {
				t.Fatalf("Action = %q, want %q", got, tt.action)
			}
			if got := plan.Actions[0].Intent.Target; got != tt.target {
				t.Fatalf("Target = %q, want %q", got, tt.target)
			}
		})
	}
}

func TestParserLocalCommandsOverrideWrongModelCompoundPlan(t *testing.T) {
	action := `{"intent":{"action":"explore","target":"","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"search the floor and take the flashlight","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}`
	raw := `{"actions":[` + action + `,` + action + `,` + action + `,` + action + `],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"search the floor and take the flashlight"}`

	plan, err := NewParser(&fakeGenerator{responses: []string{raw}}).Parse(context.Background(), "search the floor and take the flashlight", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("Actions len = %d, want 2: %#v", len(plan.Actions), plan)
	}
	if plan.Actions[0].Intent.Action != ActionSearch || plan.Actions[0].Intent.Target != "floor" {
		t.Fatalf("first action = %#v, want search floor", plan.Actions[0].Intent)
	}
	if plan.Actions[1].Intent.Action != ActionTakeItem || plan.Actions[1].Intent.Target != "flashlight" {
		t.Fatalf("second action = %#v, want take flashlight", plan.Actions[1].Intent)
	}
}

func TestParserCanonicalizesTranscriptRoomPluralAndUnlockPhrases(t *testing.T) {
	tests := []struct {
		name    string
		message string
		model   []PlannedAction
		want    []PlannedAction
	}{
		{
			name:    "room search in compound",
			message: "turn on the flashlight and search the room",
			model: []PlannedAction{
				modelAction(ActionTurnOn, "", "flashlight", ""),
				modelAction(ActionSearch, "room", "", ""),
			},
			want: []PlannedAction{
				modelAction(ActionTurnOn, "", "flashlight", ""),
				modelAction(ActionInspect, "", "", ""),
			},
		},
		{
			name:    "explicit both doctors",
			message: "search the both doctors",
			model:   []PlannedAction{modelAction(ActionSearch, "both doctors", "", "")},
			want:    []PlannedAction{allTargets(modelAction(ActionSearch, "doctors", "", ""))},
		},
		{
			name:    "unlock demonstrative door",
			message: "use the key to unlock that door",
			model: []PlannedAction{
				modelAction(ActionUseItem, "body_door", "Brass Key", ""),
				modelAction(ActionExplore, "body_door", "", ""),
				modelAction(ActionExplore, "body_door", "", ""),
				modelAction(ActionExplore, "body_door", "", ""),
			},
			want: []PlannedAction{modelAction(ActionUseItem, "door", "key", "")},
		},
		{
			name:    "unlock named door with echoed state",
			message: "unlock the The Emergency Stairwell Door is locked",
			model: []PlannedAction{
				modelAction(ActionUseItem, "The Emergency Stairwell Door", "Brass Key", ""),
				modelAction(ActionExplore, "", "", ""),
			},
			want: []PlannedAction{modelAction(ActionUseItem, "emergency stairwell door", "key", "")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewParser(&fakeGenerator{responses: []string{modelPlanJSON(t, tt.message, tt.model...)}}).Parse(context.Background(), tt.message, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if got.NeedsClarification || len(got.Actions) != len(tt.want) {
				t.Fatalf("plan = %#v, want %d executable actions", got, len(tt.want))
			}
			for i := range tt.want {
				actual, want := got.Actions[i], tt.want[i]
				if actual.Intent.Action != want.Intent.Action || actual.Intent.Target != want.Intent.Target || actual.Intent.Item != want.Intent.Item || actual.Intent.Direction != want.Intent.Direction || actual.TargetMode != want.TargetMode {
					t.Fatalf("action %d = %#v, want %#v", i, actual, want)
				}
			}
		})
	}
}

func TestParserNormalizesPluralDoctorTargetAsAll(t *testing.T) {
	raw := `{"actions":[{"intent":{"action":"inspect","target":"doctors","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"inspect the doctors","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"inspect the doctors"}`
	plan, err := NewParser(&fakeGenerator{responses: []string{raw}}).Parse(context.Background(), "inspect the doctors", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != ActionInspect || plan.Actions[0].TargetMode != TargetAll {
		t.Fatalf("plan = %#v, want plural inspect with targetMode all", plan)
	}
}

func TestParserPreservesRepeatedGenericActions(t *testing.T) {
	raw := `{"actions":[{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"wait twice","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"wait twice","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"wait twice"}`
	plan, err := NewParser(&fakeGenerator{responses: []string{raw}}).Parse(context.Background(), "wait twice", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 || plan.Actions[0].Intent.Action != ActionWait || plan.Actions[1].Intent.Action != ActionWait {
		t.Fatalf("plan = %#v, want two ordered wait actions", plan)
	}
}

func TestParserNormalizesUnsupportedQuestionToClarification(t *testing.T) {
	raw := `{"actions":[{"intent":{"action":"search","target":"room","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"do they have anything","needsClarification":false,"clarificationQuestion":""},"targetMode":"all"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"do they have anything"}`
	plan, err := NewParser(&fakeGenerator{responses: []string{raw}}).Parse(context.Background(), "do they have anything", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 || !plan.NeedsClarification || plan.ClarificationQuestion == "" {
		t.Fatalf("plan = %#v, want clarification without action", plan)
	}
}

func TestParserCanonicalizesRecognizedFallbackCommands(t *testing.T) {
	tests := []struct {
		name    string
		message string
		model   PlannedAction
	}{
		{
			name:    "room awareness wrong action",
			message: "whats around you",
			model:   modelAction(ActionExplore, "room", "", "all"),
		},
		{
			name:    "room inspection canonical target",
			message: "Inspect the room.",
			model:   modelAction(ActionInspect, "room", "", ""),
		},
		{
			name:    "take item canonical fields",
			message: "take the flashlight",
			model:   modelAction(ActionTakeItem, "all", "flashlight", ""),
		},
		{
			name:    "movement wrong action",
			message: "go east",
			model:   modelAction(ActionExplore, "all", "", "east"),
		},
		{
			name:    "throw target canonical field",
			message: "throw the brick down the hallway",
			model:   modelAction(ActionThrow, "", "brick", ""),
		},
		{
			name:    "ambiguous intent question",
			message: "what do you have in mind",
			model:   modelAction(ActionTalk, "inventory", "", ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewParser(&fakeGenerator{responses: []string{modelPlanJSON(t, tt.message, tt.model)}}).Parse(context.Background(), tt.message, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if want := FallbackPlan(tt.message); !reflect.DeepEqual(got, want) {
				t.Fatalf("plan = %#v, want canonical fallback %#v", got, want)
			}
		})
	}
}

func TestParserKeepsCompatibleModelModifiersWithCanonicalFallbackFields(t *testing.T) {
	message := "take the flashlight"
	model := modelAction(ActionTakeItem, "all", "flashlight", "", "carefully")
	got, err := NewParser(&fakeGenerator{responses: []string{modelPlanJSON(t, message, model)}}).Parse(context.Background(), message, game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}

	want := FallbackPlan(message)
	want.Actions[0].Intent.Modifiers = []string{"carefully"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plan = %#v, want canonical fallback with modifier %#v", got, want)
	}
}

func TestParserPreservesRicherThrowWhenFallbackIsIncomplete(t *testing.T) {
	message := "throw the brick toward the window"
	model := modelAction(ActionThrow, "window", "brick", "")
	got, err := NewParser(&fakeGenerator{responses: []string{modelPlanJSON(t, message, model)}}).Parse(context.Background(), message, game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	resolved := got.Actions[0]
	if resolved.Intent.Action != ActionThrow || resolved.Intent.Item != "brick" || resolved.Intent.Target != "window" || resolved.TargetMode != TargetSingle {
		t.Fatalf("action = %#v, want richer model throw semantics", resolved)
	}
}

func TestMergeCanonicalActionSemantics(t *testing.T) {
	tests := []struct {
		name      string
		canonical PlannedAction
		model     PlannedAction
		want      PlannedAction
	}{
		{
			name:      "preserves compatible search item",
			canonical: modelAction(ActionSearch, "pockets", "", ""),
			model:     modelAction(ActionSearch, "pockets", "flashlight", ""),
			want:      modelAction(ActionSearch, "pockets", "flashlight", ""),
		},
		{
			name:      "preserves compatible move direction",
			canonical: modelAction(ActionMove, "", "", ""),
			model:     modelAction(ActionMove, "", "", "east"),
			want:      modelAction(ActionMove, "", "", "east"),
		},
		{
			name:      "rejects conflicting item",
			canonical: modelAction(ActionSearch, "desk", "key", ""),
			model:     modelAction(ActionSearch, "desk", "flashlight", ""),
			want:      modelAction(ActionSearch, "desk", "key", ""),
		},
		{
			name:      "rejects conflicting direction",
			canonical: modelAction(ActionMove, "", "", "east"),
			model:     modelAction(ActionMove, "", "", "west"),
			want:      modelAction(ActionMove, "", "", "east"),
		},
		{
			name:      "enriches incomplete throw fields",
			canonical: modelAction(ActionThrow, "", "", ""),
			model:     modelAction(ActionThrow, "window", "brick", ""),
			want:      modelAction(ActionThrow, "window", "brick", ""),
		},
		{
			name:      "rejects conflicting throw fields",
			canonical: modelAction(ActionThrow, "window", "brick", ""),
			model:     modelAction(ActionThrow, "door", "stone", ""),
			want:      modelAction(ActionThrow, "window", "brick", ""),
		},
		{
			name:      "preserves compatible all target mode",
			canonical: allTargets(modelAction(ActionSearch, "both", "", "")),
			model:     allTargets(modelAction(ActionSearch, "both", "", "")),
			want:      allTargets(modelAction(ActionSearch, "both", "", "")),
		},
		{
			name:      "rejects conflicting all target mode",
			canonical: modelAction(ActionSearch, "desk", "", ""),
			model:     allTargets(modelAction(ActionSearch, "desk", "", "")),
			want:      modelAction(ActionSearch, "desk", "", ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeCanonicalAction(tt.canonical, tt.model); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("action = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func allTargets(action PlannedAction) PlannedAction {
	action.TargetMode = TargetAll
	return action
}

func modelAction(action Action, target, item, direction string, modifiers ...string) PlannedAction {
	return PlannedAction{
		Intent: Intent{
			Action:     action,
			Target:     target,
			Item:       item,
			Direction:  direction,
			Modifiers:  append([]string{}, modifiers...),
			Confidence: 0.9,
		},
		TargetMode: TargetSingle,
	}
}

func modelPlanJSON(t *testing.T, message string, actions ...PlannedAction) string {
	t.Helper()
	for i := range actions {
		actions[i].Intent.RawText = message
	}
	encoded, err := json.Marshal(TurnPlan{
		Actions:               actions,
		Questions:             []FactQuestion{},
		Confidence:            0.9,
		NeedsClarification:    false,
		ClarificationQuestion: "",
		RawText:               message,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestParseTurnPlanRejectsMoreThanFourActions(t *testing.T) {
	_, err := ParseTurnPlanJSON(fiveActionPlanJSON)
	if !errors.Is(err, ErrPlanTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestParseTurnPlanRejectsNullSchemaFields(t *testing.T) {
	base := `{"actions":[],"questions":[],"confidence":1,"needsClarification":false,"clarificationQuestion":"","rawText":"wait"}`
	for _, field := range []string{"actions", "questions", "confidence", "needsClarification", "clarificationQuestion", "rawText"} {
		t.Run(field, func(t *testing.T) {
			raw := strings.Replace(base, `"`+field+`":`+fieldValue(base, field), `"`+field+`":null`, 1)
			if _, err := ParseTurnPlanJSON(raw); err == nil {
				t.Fatalf("expected null %s to fail", field)
			}
		})
	}
}

func TestParseTurnPlanRejectsNullEmbeddedModifiers(t *testing.T) {
	raw := `{"actions":[{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":null,"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":1,"needsClarification":false,"clarificationQuestion":"","rawText":"wait"}`
	if _, err := ParseTurnPlanJSON(raw); err == nil {
		t.Fatal("expected null modifiers to fail")
	}
}

func fieldValue(raw, field string) string {
	marker := `"` + field + `":`
	start := strings.Index(raw, marker) + len(marker)
	rest := raw[start:]
	if strings.HasPrefix(rest, `"`) {
		end := strings.Index(rest[1:], `"`) + 2
		return rest[:end]
	}
	for i, r := range rest {
		if r == ',' || r == '}' {
			return rest[:i]
		}
	}
	return rest
}

const fiveActionPlanJSON = `{
	"actions":[
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}
	],"questions":[],"confidence":1,"needsClarification":false,"clarificationQuestion":"","rawText":"wait five times"
}`

func TestParserParseValidIntent(t *testing.T) {
	generator := &fakeGenerator{responses: []string{`{"actions":[{"intent":{"action":"search","target":"dead doctor coat pockets","item":"flashlight","direction":"","modifiers":["carefully","keep_light_low"],"confidence":0.93,"rawText":"check the pockets","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.93,"needsClarification":false,"clarificationQuestion":"","rawText":"check the pockets"}`}}

	parser := NewParser(generator)
	got, err := parser.Parse(context.Background(), "check the pockets", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if got.Actions[0].Intent.Action != ActionSearch {
		t.Fatalf("Action = %q, want %q", got.Actions[0].Intent.Action, ActionSearch)
	}
	if got.Actions[0].Intent.Target != "dead doctor coat pockets" {
		t.Fatalf("Target = %q, want dead doctor coat pockets", got.Actions[0].Intent.Target)
	}
	if got.Actions[0].Intent.Item != "flashlight" {
		t.Fatalf("Item = %q, want flashlight", got.Actions[0].Intent.Item)
	}
	if generator.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", generator.calls)
	}
}

func TestParserRepairsInvalidJSON(t *testing.T) {
	generator := &fakeGenerator{responses: []string{
		`not json`,
		`{"actions":[{"intent":{"action":"unknown","target":"","item":"","direction":"","modifiers":[],"confidence":0.18,"rawText":"Do it.","needsClarification":true,"clarificationQuestion":"What do you want Kaya to do?"},"targetMode":"single"}],"questions":[],"confidence":0.18,"needsClarification":true,"clarificationQuestion":"What do you want Kaya to do?","rawText":"Do it."}`,
	}}

	parser := NewParser(generator)
	got, err := parser.Parse(context.Background(), "Do it.", game.PerceptionSnapshot{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if len(got.Actions) != 0 {
		t.Fatalf("Actions = %#v, want no executable actions", got.Actions)
	}
	if !got.NeedsClarification {
		t.Fatal("NeedsClarification = false, want true")
	}
	if generator.calls != 2 {
		t.Fatalf("generator calls = %d, want 2", generator.calls)
	}
}

func TestParseJSONRejectsInvalidAction(t *testing.T) {
	_, err := ParseJSON(`{
		"action": "teleport",
		"target": "door",
		"item": "",
		"direction": "",
		"modifiers": [],
		"confidence": 0.8,
		"rawText": "teleport",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err == nil {
		t.Fatal("ParseJSON returned nil error for invalid action")
	}
}

func TestParseJSONNormalizesMoveDirectionFromTarget(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "move",
		"target": "left",
		"item": "",
		"direction": "",
		"modifiers": ["quietly"],
		"confidence": 0.95,
		"rawText": "Maybe go left, but quietly.",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Direction != "left" {
		t.Fatalf("Direction = %q, want left", got.Direction)
	}
	if got.Target != "" {
		t.Fatalf("Target = %q, want empty", got.Target)
	}
}

func TestParseJSONNormalizesGeneralRoomAwareness(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "search",
		"target": "room",
		"item": "",
		"direction": "",
		"modifiers": [],
		"confidence": 0.95,
		"rawText": "What's in the room?",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionInspect {
		t.Fatalf("Action = %q, want %q", got.Action, ActionInspect)
	}
	if got.Target != "" {
		t.Fatalf("Target = %q, want empty", got.Target)
	}
}

func TestParseJSONNormalizesAroundYouAwareness(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "inspect",
		"target": "around you",
		"item": "",
		"direction": "",
		"modifiers": [],
		"confidence": 0.95,
		"rawText": "is there anything around you",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionInspect {
		t.Fatalf("Action = %q, want %q", got.Action, ActionInspect)
	}
	if got.Target != "" {
		t.Fatalf("Target = %q, want empty", got.Target)
	}
}

func TestParseJSONNormalizesVagueFollowUp(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "move",
		"target": "empty string",
		"item": "empty string",
		"direction": "empty string",
		"modifiers": [],
		"confidence": 1,
		"rawText": "Do it.",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionUnknown {
		t.Fatalf("Action = %q, want %q", got.Action, ActionUnknown)
	}
	if !got.NeedsClarification {
		t.Fatal("NeedsClarification = false, want true")
	}
	if got.Confidence > 0.25 {
		t.Fatalf("Confidence = %.2f, want <= 0.25", got.Confidence)
	}
}

func TestParseJSONNormalizesInventoryQuestion(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "unknown",
		"target": "",
		"item": "flashlight",
		"direction": "",
		"modifiers": [],
		"confidence": 0.8,
		"rawText": "do ypou have flashlight",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionTalk {
		t.Fatalf("Action = %q, want %q", got.Action, ActionTalk)
	}
	if got.Item != "flashlight" {
		t.Fatalf("Item = %q, want flashlight", got.Item)
	}
}

func TestParseJSONNormalizesKeyUse(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "force_open",
		"target": "emergency stairwell door",
		"item": "key",
		"direction": "",
		"modifiers": [],
		"confidence": 1,
		"rawText": "Try the key on the emergency stairwell door.",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Action != ActionUseItem {
		t.Fatalf("Action = %q, want %q", got.Action, ActionUseItem)
	}
}

func TestParseJSONRestoresExplicitFlashlightItem(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "search",
		"target": "dead doctor's coat pockets",
		"item": "",
		"direction": "",
		"modifiers": ["keep_light_low"],
		"confidence": 0.95,
		"rawText": "Can you check the dead doctor's coat pockets but keep the flashlight low?",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Item != "flashlight" {
		t.Fatalf("Item = %q, want flashlight", got.Item)
	}
}

func TestParseJSONMergesNonMovementDirectionIntoSearchTarget(t *testing.T) {
	got, err := ParseJSON(`{
		"action": "search",
		"target": "the doctor",
		"item": "",
		"direction": "near cabinet",
		"modifiers": [],
		"confidence": 0.95,
		"rawText": "search the doctor near cabinet",
		"needsClarification": false,
		"clarificationQuestion": ""
	}`)
	if err != nil {
		t.Fatalf("ParseJSON returned error: %v", err)
	}

	if got.Target != "the doctor near cabinet" {
		t.Fatalf("Target = %q, want the doctor near cabinet", got.Target)
	}
	if got.Direction != "" {
		t.Fatalf("Direction = %q, want empty", got.Direction)
	}
}
