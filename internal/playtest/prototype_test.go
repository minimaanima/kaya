package playtest

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
)

func TestPrototypeThousandPhraseVariedSessionsReachObjective(t *testing.T) {
	placementsSeen := map[string]bool{}
	phrases := PrototypePhrases()
	unlockSeen := map[string]bool{}
	unlockSucceeded := map[string]bool{}
	for seed := int64(1); seed <= 1000; seed++ {
		run := prototypeGeneratedRun(t, seed)
		placementsSeen[placementKey(run.Placements)] = true
		runner := NewOfflineRunner(runscenario.PrototypeDefinition(), run)
		messages, err := PrototypeWinningMessages(run, seed)
		if err != nil {
			t.Fatalf("seed %d placements=%#v: %v", seed, run.Placements, err)
		}
		for _, message := range messages {
			step, err := runner.Step(context.Background(), message)
			if err != nil {
				t.Fatalf("seed %d placements=%#v message %q: %v\nsession=%#v", seed, run.Placements, message, err, runner.Session())
			}
			for _, phrase := range phrases.unlock {
				if message != phrase {
					continue
				}
				unlockSeen[phrase] = true
				if len(step.Turn.Result.Outcomes) != 1 || step.Turn.Result.Outcomes[0].Result.Outcome != "door_unlocked" {
					t.Fatalf("seed %d placements=%#v unlock %q did not execute successfully\nstep=%#v\nsession=%#v", seed, run.Placements, message, step, runner.Session())
				}
				unlockSucceeded[phrase] = true
			}
		}
		got := runner.Session()
		if runner.State().CurrentRoomID != scenario.RoomStairwell || got.ObjectiveEmissions != 1 {
			t.Fatalf("seed %d placements=%#v did not finish\nsession=%#v", seed, run.Placements, got)
		}
	}
	wantPlacements := prototypePlacementKeys()
	if !reflect.DeepEqual(placementsSeen, wantPlacements) {
		t.Fatalf("placement combinations = %s, want %s", fmt.Sprint(placementsSeen), fmt.Sprint(wantPlacements))
	}
	wantUnlocks := map[string]bool{
		"use the key on the emergency stairwell door": true,
		"try the key on the stairwell door":           true,
	}
	if !reflect.DeepEqual(unlockSeen, wantUnlocks) || !reflect.DeepEqual(unlockSucceeded, wantUnlocks) {
		t.Fatalf("unlock variants seen=%s succeeded=%s, want=%s", fmt.Sprint(unlockSeen), fmt.Sprint(unlockSucceeded), fmt.Sprint(wantUnlocks))
	}
}

func TestRunPrototypeSessionRejectsIncompleteObjective(t *testing.T) {
	run := mustGeneratedRun(t, 1)
	runner := NewRunner(runscenario.PrototypeDefinition(), run, intent.NewParser(nil), fallbackComposer{})

	err := RunPrototypeSession(context.Background(), runner, run, 1)

	if err == nil || !strings.Contains(err.Error(), "objective") {
		t.Fatalf("error = %v, want objective completion failure", err)
	}
}

func TestPrototypePhrasesReturnsIndependentCopy(t *testing.T) {
	first := PrototypePhrases()
	first.unlock[0] = "mutated"
	second := PrototypePhrases()
	if second.unlock[0] == "mutated" {
		t.Fatalf("prototype phrase accessor exposes canonical data: %#v", second)
	}
}

func TestRunPrototypeSessionPreservesAndExecutesQuarterSeedCompoundTurns(t *testing.T) {
	tests := []struct {
		name         string
		seed         int64
		wantActions  []intent.Action
		wantOutcomes []string
	}{
		{
			name:         "take flashlight then move east",
			seed:         4,
			wantActions:  []intent.Action{intent.ActionTakeItem, intent.ActionMove},
			wantOutcomes: []string{"item_taken", "moved"},
		},
		{
			name:         "turn on flashlight then inspect storage",
			seed:         1,
			wantActions:  []intent.Action{intent.ActionTurnOn, intent.ActionInspect},
			wantOutcomes: []string{"flashlight_on", "inspected_room"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := mustGeneratedRun(t, test.seed)
			runner := NewRunner(runscenario.PrototypeDefinition(), run, fallbackParser{}, fallbackComposer{})
			if err := RunPrototypeSession(context.Background(), runner, run, test.seed); err != nil {
				t.Fatalf("seed %d placements=%#v: %v\nsession=%#v", test.seed, run.Placements, err, runner.Session())
			}

			compound := prototypeCompoundStep(t, runner.Session())
			if len(compound.Turn.SemanticPlan.Actions) != 2 || len(compound.Turn.Result.Outcomes) != 2 {
				t.Fatalf("seed %d compound step=%#v", test.seed, compound)
			}
			for index, want := range test.wantActions {
				if got := compound.Turn.SemanticPlan.Actions[index].ActionKind(); got != want {
					t.Fatalf("seed %d action %d = %q, want %q\nstep=%#v", test.seed, index, got, want, compound)
				}
				if got := compound.Turn.Result.Outcomes[index].Result.Outcome; got != test.wantOutcomes[index] {
					t.Fatalf("seed %d outcome %d = %q, want %q\nstep=%#v", test.seed, index, got, test.wantOutcomes[index], compound)
				}
			}
		})
	}
}

func TestPrototypeWinningMessagesAreDeterministicAndDoNotMutatePhraseBank(t *testing.T) {
	run := mustGeneratedRun(t, 47)
	before := PrototypePhrases()
	first, err := PrototypeWinningMessages(run, 47)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PrototypeWinningMessages(run, 47)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("messages differ for the same generated run and seed: first=%#v second=%#v", first, second)
	}
	if after := PrototypePhrases(); !reflect.DeepEqual(after, before) {
		t.Fatalf("phrase bank mutated: before=%#v after=%#v", before, after)
	}
}

func prototypeCompoundStep(t *testing.T, session Session) Step {
	t.Helper()
	for _, step := range session.Steps {
		if strings.Contains(step.Player, " then ") {
			return step
		}
	}
	t.Fatalf("session has no compound step: %#v", session)
	return Step{}
}

func placementKey(placements []rungen.Placement) string {
	objects := make(map[game.ItemID]game.ObjectID, len(placements))
	for _, placement := range placements {
		objects[placement.ItemID] = placement.ObjectID
	}
	return fmt.Sprintf("flashlight=%s,key=%s", objects[scenario.ItemFlashlight], objects[scenario.ItemBrassKey])
}

func prototypeGeneratedRun(t *testing.T, seed int64) rungen.GeneratedRun {
	t.Helper()
	run, err := rungen.Generate(
		rungen.RunConfig{Seed: seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
		runscenario.PrototypeDefinition(),
	)
	if err != nil {
		t.Fatalf("seed %d: generate prototype run: %v", seed, err)
	}
	return run
}

func prototypePlacementKeys() map[string]bool {
	keys := map[string]bool{}
	for _, flashlightObjectID := range []game.ObjectID{
		scenario.ObjectReceptionDesk,
		scenario.ObjectReceptionFloor,
		scenario.ObjectCollapsedChair,
	} {
		for _, keyObjectID := range []game.ObjectID{
			scenario.ObjectBodyCabinet,
			scenario.ObjectBodyDoor,
			scenario.ObjectStorageCabinet,
		} {
			keys[placementKey([]rungen.Placement{
				{ItemID: scenario.ItemFlashlight, ObjectID: flashlightObjectID},
				{ItemID: scenario.ItemBrassKey, ObjectID: keyObjectID},
			})] = true
		}
	}
	return keys
}
