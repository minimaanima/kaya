package intent_test

import (
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

type corpusEvaluation struct {
	Total      int
	Matches    int
	Mismatches []string
	Errors     []string
}

func (e *corpusEvaluation) RecordMatch() { e.Matches++ }

func (e *corpusEvaluation) RecordMismatch(message string) {
	e.Mismatches = append(e.Mismatches, message)
}

func (e *corpusEvaluation) RecordError(message string) {
	e.Errors = append(e.Errors, message)
}

func (e corpusEvaluation) Accuracy() float64 {
	if e.Total == 0 {
		return 0
	}
	return 100 * float64(e.Matches) / float64(e.Total)
}

func (e corpusEvaluation) Fails(threshold float64) bool {
	return len(e.Errors) > 0 || e.Accuracy() < threshold
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
	{Name: "01", Message: "Look around.", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "02", Message: "whats around you", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "03", Message: "What do you see?", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "04", Message: "Is there anything around you?", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "05", Message: "Inspect the room.", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "")}}},
	{Name: "06", Message: "look at the reception desk", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "reception desk")}}},
	{Name: "07", Message: "look on the desk", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "desk")}}},
	{Name: "08", Message: "inspect the storage cabinet", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "storage cabinet")}}},
	{Name: "09", Message: "look over the floor", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "floor")}}},
	{Name: "10", Message: "what is on the desk", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionInspect, "desk")}}},
	{Name: "11", Message: "search the desk", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "desk")}}},
	{Name: "12", Message: "check the drawers", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "drawers")}}},
	{Name: "13", Message: "rummage through the cabinet", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "cabinet")}}},
	{Name: "14", Message: "look through the doctor's coat", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "doctor's coat")}}},
	{Name: "15", Message: "is something inside the drawers", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "drawers")}}},
	{Name: "16", Message: "is there anything in the cabinet", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "cabinet")}}},
	{Name: "17", Message: "what's in your bag", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTalk, "inventory")}}},
	{Name: "18", Message: "do you have anything useful on you", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTalk, "inventory")}}},
	{Name: "19", Message: "what are you carrying", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTalk, "inventory")}}},
	{Name: "20", Message: "do ypou have flashlight", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTalk, "flashlight")}}},
	{Name: "21", Message: "is there a flashlight", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTalk, "flashlight")}}},
	{Name: "22", Message: "where is the key", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTalk, "key")}}},
	{Name: "23", Message: "have you found the key", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTalk, "key")}}},
	{Name: "24", Message: "take the flashlight", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "flashlight")}}},
	{Name: "25", Message: "grab the key", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "key")}}},
	{Name: "26", Message: "pick up the brick", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "brick")}}},
	{Name: "27", Message: "took the key", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "key")}}},
	{Name: "28", Message: "turn on the flashlight", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTurnOn, "flashlight")}}},
	{Name: "29", Message: "switch on your torch", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTurnOn, "flashlight")}}},
	{Name: "30", Message: "turn off the light", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTurnOff, "flashlight")}}},
	{Name: "31", Message: "go east", Want: corpusPlan{Actions: []corpusAction{move("east")}}},
	{Name: "32", Message: "move north", Want: corpusPlan{Actions: []corpusAction{move("north")}}},
	{Name: "33", Message: "head west", Want: corpusPlan{Actions: []corpusAction{move("west")}}},
	{Name: "34", Message: "walk back", Want: corpusPlan{Actions: []corpusAction{move("back")}}},
	{Name: "35", Message: "north", Want: corpusPlan{Actions: []corpusAction{move("north")}}},
	{Name: "36", Message: "stay still", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionWait, "")}}},
	{Name: "37", Message: "wait here", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionWait, "")}}},
	{Name: "38", Message: "pause for a moment", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionWait, "")}}},
	{Name: "39", Message: "listen at the door", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionListen, "door")}}},
	{Name: "40", Message: "get behind the cabinet and hide", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionHide, "cabinet")}}},
	{Name: "41", Message: "throw the brick down the hallway", Want: corpusPlan{Actions: []corpusAction{{Action: intent.ActionThrow, Target: "hallway", Item: "brick", TargetMode: intent.TargetSingle}}}},
	{Name: "42", Message: "use the key on the emergency stairwell door", Want: corpusPlan{Actions: []corpusAction{{Action: intent.ActionUseItem, Target: "emergency stairwell door", Item: "key", TargetMode: intent.TargetSingle}}}},
	{Name: "43", Message: "feel along the walls for another exit", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionExplore, "")}}},
	{Name: "44", Message: "run your hands along the wall", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionExplore, "")}}},
	{Name: "45", Message: "both", Want: corpusPlan{Actions: []corpusAction{{Action: intent.ActionSearch, Target: "both", TargetMode: intent.TargetAll}}}},
	{Name: "46", Message: "search the floor and take the flashlight", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "floor"), action(intent.ActionTakeItem, "flashlight")}}},
	{Name: "47", Message: "take the flashlight and go east", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionTakeItem, "flashlight"), move("east")}}},
	{Name: "48", Message: "turn on the flashlight and look around", Want: corpusPlan{Actions: []corpusAction{itemAction(intent.ActionTurnOn, "flashlight"), action(intent.ActionInspect, "")}}},
	{Name: "49", Message: "search the doctors are they dead", Want: corpusPlan{Actions: []corpusAction{{Action: intent.ActionSearch, Target: "doctors", TargetMode: intent.TargetAll}}, Questions: []corpusQuestion{{Kind: game.FactLifeStatus, Target: "doctors", TargetMode: intent.TargetAll}}}},
	{Name: "50", Message: "what do you have in mind", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionUnknown, "")}, NeedsClarification: true}},
	{Name: "51", Message: "do it", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionUnknown, "")}, NeedsClarification: true}},
	{Name: "52", Message: "search the doctor near the cabiner", Want: corpusPlan{Actions: []corpusAction{action(intent.ActionSearch, "doctor near the cabinet")}}},
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

func TestCorpusEvaluationCountsMatchesAndErrors(t *testing.T) {
	eval := corpusEvaluation{Total: 3}
	eval.RecordMatch()
	eval.RecordMismatch("wrong action")
	eval.RecordError("timeout")

	if eval.Matches != 1 || len(eval.Mismatches) != 1 || len(eval.Errors) != 1 {
		t.Fatalf("evaluation = %#v", eval)
	}
	if got := eval.Accuracy(); got != 100.0/3.0 {
		t.Fatalf("accuracy = %f, want %f", got, 100.0/3.0)
	}
}

func TestCorpusEvaluationFailsBelowThreshold(t *testing.T) {
	eval := corpusEvaluation{Total: 10, Matches: 8}
	if !eval.Fails(90) {
		t.Fatal("80 percent evaluation should fail a 90 percent threshold")
	}
	eval.Matches = 9
	if eval.Fails(90) {
		t.Fatal("90 percent evaluation should pass a 90 percent threshold")
	}
	eval.Errors = []string{"timeout"}
	if !eval.Fails(90) {
		t.Fatal("evaluation with errors should fail even at the threshold")
	}
}
