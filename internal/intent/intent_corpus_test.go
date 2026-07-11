package intent_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
)

type corpusAction struct {
	Action     intent.Action
	Target     string
	Item       string
	Direction  string
	Modifiers  []string
	TargetMode intent.TargetMode
}

type corpusQuestion struct {
	Kind       game.FactKind
	Target     string
	TargetMode intent.TargetMode
}

type corpusPlan struct {
	Actions            []corpusAction
	Questions          []corpusQuestion
	NeedsClarification bool
}

type intentCorpusCase struct {
	Name    string
	Message string
	Want    corpusPlan
}

type corpusGenerator func(context.Context, string, string, any) (string, error)

func (generate corpusGenerator) GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, schema any) (string, error) {
	return generate(ctx, systemPrompt, userPrompt, schema)
}

type corpusEvaluation struct {
	Total              int
	RawMatches         int
	ResolvedMatches    int
	FallbackAssisted   int
	Repairs            int
	FallbackErrorCount int
	RawMismatches      []string
	ResolvedMismatches []string
	Errors             []string
}

func (e *corpusEvaluation) Record(tc intentCorpusCase, resolved intent.TurnPlan, provenance intent.ParseProvenance) {
	label := fmt.Sprintf("%s (%q)", tc.Name, tc.Message)
	if provenance.Canonicalized || provenance.Source == intent.ParseSourceFallback {
		e.FallbackAssisted++
	}
	if provenance.Source == intent.ParseSourceRepair {
		e.Repairs++
	}
	if provenance.FallbackError != nil {
		e.FallbackErrorCount++
		e.Errors = append(e.Errors, fmt.Sprintf("%s: generator/decoding fallback: %v", label, provenance.FallbackError))
	}

	if provenance.HasRawPlan {
		raw := semanticPlanFrom(provenance.RawPlan)
		if diff := compareSemanticPlans(tc.Want, raw); diff != "" {
			e.RawMismatches = append(e.RawMismatches, fmt.Sprintf("%s:\n%s", label, diff))
		} else {
			e.RawMatches++
		}
	} else {
		e.RawMismatches = append(e.RawMismatches, fmt.Sprintf("%s: no decoded raw model plan", label))
	}

	got := semanticPlanFrom(resolved)
	if err := validateSemanticPlan(got); err != nil {
		e.Errors = append(e.Errors, fmt.Sprintf("%s: invalid resolved plan: %v", label, err))
	} else if diff := compareSemanticPlans(tc.Want, got); diff != "" {
		e.ResolvedMismatches = append(e.ResolvedMismatches, fmt.Sprintf("%s:\n%s", label, diff))
	} else {
		e.ResolvedMatches++
	}
}

func (e corpusEvaluation) RawAccuracy() float64 {
	if e.Total == 0 {
		return 0
	}
	return 100 * float64(e.RawMatches) / float64(e.Total)
}

func (e corpusEvaluation) ResolvedAccuracy() float64 {
	if e.Total == 0 {
		return 0
	}
	return 100 * float64(e.ResolvedMatches) / float64(e.Total)
}

func (e corpusEvaluation) Fails(threshold float64) bool {
	return len(e.Errors) > 0 || e.ResolvedAccuracy() < threshold
}

func action(kind intent.Action, target string) corpusAction {
	return corpusAction{Action: kind, Target: target, TargetMode: intent.TargetSingle}
}

func itemAction(kind intent.Action, item string) corpusAction {
	return corpusAction{Action: kind, Item: item, TargetMode: intent.TargetSingle}
}

func move(direction string) corpusAction {
	return corpusAction{Action: intent.ActionMove, Direction: direction, TargetMode: intent.TargetSingle}
}

func semanticPlanFrom(plan intent.TurnPlan) corpusPlan {
	got := corpusPlan{NeedsClarification: plan.NeedsClarification}
	for _, planned := range plan.Actions {
		got.Actions = append(got.Actions, corpusAction{
			Action: planned.Intent.Action, Target: planned.Intent.Target,
			Item: planned.Intent.Item, Direction: planned.Intent.Direction,
			Modifiers:  append([]string(nil), planned.Intent.Modifiers...),
			TargetMode: planned.TargetMode,
		})
	}
	for _, question := range plan.Questions {
		got.Questions = append(got.Questions, corpusQuestion{
			Kind: question.Kind, Target: question.Target, TargetMode: question.TargetMode,
		})
	}
	return got
}

func normalizeCorpusPlan(plan corpusPlan) corpusPlan {
	if plan.Actions == nil {
		plan.Actions = []corpusAction{}
	}
	if plan.Questions == nil {
		plan.Questions = []corpusQuestion{}
	}
	for i := range plan.Actions {
		if plan.Actions[i].Modifiers == nil {
			plan.Actions[i].Modifiers = []string{}
		}
	}
	return plan
}

func compareSemanticPlans(want, got corpusPlan) string {
	want = normalizeCorpusPlan(want)
	got = normalizeCorpusPlan(got)
	if reflect.DeepEqual(want, got) {
		return ""
	}
	return fmt.Sprintf("want: %#v\n got: %#v", want, got)
}

func validateSemanticPlan(plan corpusPlan) error {
	if len(plan.Actions) > 4 {
		return fmt.Errorf("too many actions: %d", len(plan.Actions))
	}
	if len(plan.Questions) > 4 {
		return fmt.Errorf("too many questions: %d", len(plan.Questions))
	}
	for _, action := range plan.Actions {
		if !action.Action.Valid() {
			return fmt.Errorf("invalid action: %q", action.Action)
		}
		if action.TargetMode != intent.TargetSingle && action.TargetMode != intent.TargetAll {
			return fmt.Errorf("invalid action target mode: %q", action.TargetMode)
		}
		if action.TargetMode == intent.TargetAll && action.Action != intent.ActionInspect && action.Action != intent.ActionSearch {
			return fmt.Errorf("target mode all is not supported for %q", action.Action)
		}
	}
	for _, question := range plan.Questions {
		if question.TargetMode != intent.TargetSingle && question.TargetMode != intent.TargetAll {
			return fmt.Errorf("invalid question target mode: %q", question.TargetMode)
		}
	}
	if plan.NeedsClarification {
		if len(plan.Actions) != 1 || plan.Actions[0].Action != intent.ActionUnknown {
			return fmt.Errorf("clarification plan must contain exactly one unknown action")
		}
		return nil
	}
	if len(plan.Actions) == 0 {
		return fmt.Errorf("executable plan has no actions")
	}
	return nil
}

var intentCorpus = []intentCorpusCase{
	{Name: "room-awareness-look-around", Message: "Look around.", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "room-awareness-whats-around", Message: "whats around you", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "room-awareness-what-you-see", Message: "What do you see?", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "room-awareness-anything-around", Message: "Is there anything around you?", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "room-inspect-explicit", Message: "Inspect the room.", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "inspect-reception-desk", Message: "look at the reception desk", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "reception desk")}}},
	{Name: "inspect-desk", Message: "look on the desk", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "desk")}}},
	{Name: "inspect-storage-cabinet", Message: "inspect the storage cabinet", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "storage cabinet")}}},
	{Name: "inspect-floor", Message: "look over the floor", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "floor")}}},
	{Name: "inspect-desk-question", Message: "what is on the desk", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "desk")}}},
	{Name: "search-desk", Message: "search the desk", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "desk")}}},
	{Name: "search-drawers-check", Message: "check the drawers", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "drawers")}}},
	{Name: "search-cabinet-rummage", Message: "rummage through the cabinet", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "cabinet")}}},
	{Name: "search-doctor-coat", Message: "look through the doctor's coat", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "doctor's coat")}}},
	{Name: "search-drawers-question", Message: "is something inside the drawers", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "drawers")}}},
	{Name: "search-cabinet-question", Message: "is there anything in the cabinet", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "cabinet")}}},
	{Name: "inventory-bag", Message: "what's in your bag", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTalk, "inventory")}}},
	{Name: "inventory-useful", Message: "do you have anything useful on you", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTalk, "inventory")}}},
	{Name: "inventory-carrying", Message: "what are you carrying", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTalk, "inventory")}}},
	{Name: "inventory-flashlight-typo", Message: "do ypou have flashlight", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTalk, "flashlight")}}},
	{Name: "item-presence-flashlight", Message: "is there a flashlight", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTalk, "flashlight")}}},
	{Name: "item-location-key", Message: "where is the key", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTalk, "key")}}},
	{Name: "item-found-key", Message: "have you found the key", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTalk, "key")}}},
	{Name: "take-flashlight", Message: "take the flashlight", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "flashlight")}}},
	{Name: "take-key", Message: "grab the key", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "key")}}},
	{Name: "take-brick", Message: "pick up the brick", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "brick")}}},
	{Name: "take-key-past-tense", Message: "took the key", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "key")}}},
	{Name: "turn-on-flashlight", Message: "turn on the flashlight", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTurnOn, "flashlight")}}},
	{Name: "turn-on-torch", Message: "switch on your torch", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTurnOn, "flashlight")}}},
	{Name: "turn-off-light", Message: "turn off the light", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTurnOff, "flashlight")}}},
	{Name: "move-east", Message: "go east", Want: corpusPlan{Actions: []corpusAction{move("east")}}},
	{Name: "move-north", Message: "move north", Want: corpusPlan{Actions: []corpusAction{move("north")}}},
	{Name: "move-west", Message: "head west", Want: corpusPlan{Actions: []corpusAction{move("west")}}},
	{Name: "move-back", Message: "walk back", Want: corpusPlan{Actions: []corpusAction{move("back")}}},
	{Name: "move-north-bare", Message: "north", Want: corpusPlan{Actions: []corpusAction{move("north")}}},
	{Name: "wait-stay-still", Message: "stay still", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionWait, "")}}},
	{Name: "wait-here", Message: "wait here", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionWait, "")}}},
	{Name: "wait-pause", Message: "pause for a moment", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionWait, "")}}},
	{Name: "listen-door", Message: "listen at the door", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionListen, "door")}}},
	{Name: "hide-cabinet", Message: "get behind the cabinet and hide", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionHide, "cabinet")}}},
	{Name: "throw-brick-hallway", Message: "throw the brick down the hallway", Want: corpusPlan{Actions: []corpusAction{{Action: intent.ActionThrow, Target: "hallway", Item: "brick", TargetMode: intent.TargetSingle}}}},
	{Name: "use-key-stairwell-door", Message: "use the key on the emergency stairwell door", Want: corpusPlan{Actions: []corpusAction{{Action: intent.ActionUseItem, Target: "emergency stairwell door", Item: "key", TargetMode: intent.TargetSingle}}}},
	{Name: "explore-walls", Message: "feel along the walls for another exit", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionExplore, "")}}},
	{Name: "explore-wall-touch", Message: "run your hands along the wall", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionExplore, "")}}},
	{Name: "select-both", Message: "both", Want: corpusPlan{Actions: []corpusAction{{Action: intent.ActionSearch, Target: "both", TargetMode: intent.TargetAll}}}},
	{Name: "compound-search-take", Message: "search the floor and take the flashlight", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "floor"), action(intent.ActionTakeItem, "flashlight")}}},
	{Name: "compound-take-move", Message: "take the flashlight and go east", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "flashlight"), move("east")}}},
	{Name: "compound-light-inspect", Message: "turn on the flashlight and look around", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTurnOn, "flashlight"), action(intent.ActionInspect, "")}}},
	{Name: "search-doctors-life-status", Message: "search the doctors are they dead", Want: corpusPlan{Actions: []corpusAction{{Action: intent.ActionSearch, Target: "doctors", TargetMode: intent.TargetAll}}, Questions: []corpusQuestion{{Kind: game.FactLifeStatus, Target: "doctors", TargetMode: intent.TargetAll}}}},
	{Name: "clarify-open-ended", Message: "what do you have in mind", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionUnknown, "")}, NeedsClarification: true}},
	{Name: "clarify-do-it", Message: "do it", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionUnknown, "")}, NeedsClarification: true}},
	{Name: "search-doctor-cabinet-typo", Message: "search the doctor near the cabiner", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "doctor near the cabinet")}}},
}

func TestDeterministicIntentCorpus(t *testing.T) {
	for _, tc := range intentCorpus {
		t.Run(tc.Name, func(t *testing.T) {
			got := semanticPlanFrom(intent.FallbackPlan(tc.Message))
			if err := validateSemanticPlan(got); err != nil {
				t.Fatalf("invalid semantic plan: %v; plan: %#v", err, got)
			}
			if diff := compareSemanticPlans(tc.Want, got); diff != "" {
				t.Fatalf("message %q mismatch:\n%s", tc.Message, diff)
			}
		})
	}
}

func TestCorpusEvaluationReportsRawAndResolvedAccuracy(t *testing.T) {
	eval := corpusEvaluation{Total: 4, RawMatches: 1, ResolvedMatches: 3}
	if got := eval.RawAccuracy(); got != 25 {
		t.Fatalf("raw accuracy = %f, want 25", got)
	}
	if got := eval.ResolvedAccuracy(); got != 75 {
		t.Fatalf("resolved accuracy = %f, want 75", got)
	}
}

func TestCorpusEvaluationFailsBelowThreshold(t *testing.T) {
	eval := corpusEvaluation{Total: 10, ResolvedMatches: 8}
	if !eval.Fails(90) {
		t.Fatal("80 percent evaluation should fail a 90 percent threshold")
	}
	eval.ResolvedMatches = 9
	if eval.Fails(90) {
		t.Fatal("90 percent evaluation should pass a 90 percent threshold")
	}
	eval.Errors = []string{"timeout"}
	if !eval.Fails(90) {
		t.Fatal("evaluation with errors should fail even at the threshold")
	}
}

func TestCorpusEvaluationFailsGeneratorFallbackDespiteResolvedMatch(t *testing.T) {
	parser := intent.NewParser(corpusGenerator(func(context.Context, string, string, any) (string, error) {
		return "", errors.New("ollama unreachable")
	}))
	tc := intentCorpusCase{Name: "move-east", Message: "go east", Want: corpusPlan{Actions: []corpusAction{move("east")}}}
	resolved, provenance, err := parser.ParseWithProvenance(context.Background(), tc.Message, game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}

	eval := corpusEvaluation{Total: 1}
	eval.Record(tc, resolved, provenance)
	if eval.ResolvedMatches != 1 || eval.RawMatches != 0 || eval.FallbackAssisted != 1 || len(eval.Errors) != 1 {
		t.Fatalf("evaluation = %#v, want resolved fallback match with a recorded generator error", eval)
	}
	if !eval.Fails(90) {
		t.Fatal("generator fallback must fail evaluation despite a 100% resolved score")
	}
}

func TestCorpusEvaluationFailsRepairFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		generate corpusGenerator
	}{
		{
			name: "repair generation fails",
			generate: func() corpusGenerator {
				calls := 0
				return func(context.Context, string, string, any) (string, error) {
					calls++
					if calls == 1 {
						return "not json", nil
					}
					return "", errors.New("repair unavailable")
				}
			}(),
		},
		{
			name: "repaired output cannot decode",
			generate: func(context.Context, string, string, any) (string, error) {
				return "still not json", nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := intent.NewParser(tt.generate)
			tc := intentCorpusCase{Name: "move-east", Message: "go east", Want: corpusPlan{Actions: []corpusAction{move("east")}}}
			resolved, provenance, err := parser.ParseWithProvenance(context.Background(), tc.Message, game.PerceptionSnapshot{})
			if err != nil {
				t.Fatal(err)
			}

			eval := corpusEvaluation{Total: 1}
			eval.Record(tc, resolved, provenance)
			if provenance.Source != intent.ParseSourceFallback || provenance.RepairReason == nil || provenance.FallbackError == nil {
				t.Fatalf("provenance = %#v, want failed required repair", provenance)
			}
			if eval.ResolvedMatches != 1 || eval.RawMatches != 0 || eval.FallbackErrorCount != 1 || !eval.Fails(90) {
				t.Fatalf("evaluation = %#v, repair fallback must fail despite resolved match", eval)
			}
		})
	}
}

func TestCorpusEvaluationSeparatesWrongRawFromResolvedMatch(t *testing.T) {
	const wrong = `{"actions":[{"intent":{"action":"explore","target":"room","item":"","direction":"","modifiers":[],"confidence":0.9,"rawText":"go east","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}],"questions":[],"confidence":0.9,"needsClarification":false,"clarificationQuestion":"","rawText":"go east"}`
	parser := intent.NewParser(corpusGenerator(func(context.Context, string, string, any) (string, error) {
		return wrong, nil
	}))
	tc := intentCorpusCase{Name: "move-east", Message: "go east", Want: corpusPlan{Actions: []corpusAction{move("east")}}}
	resolved, provenance, err := parser.ParseWithProvenance(context.Background(), tc.Message, game.PerceptionSnapshot{})
	if err != nil {
		t.Fatal(err)
	}

	eval := corpusEvaluation{Total: 1}
	eval.Record(tc, resolved, provenance)
	if eval.RawMatches != 0 || eval.ResolvedMatches != 1 || eval.FallbackAssisted != 1 || len(eval.Errors) != 0 {
		t.Fatalf("evaluation = %#v, want 0%% raw and 100%% resolved with canonicalization", eval)
	}
	if eval.Fails(90) {
		t.Fatal("resolved parser accuracy at threshold with no fallback errors should pass")
	}
}
