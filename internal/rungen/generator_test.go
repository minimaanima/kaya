package rungen_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

func TestGenerateSameSeedReturnsSameRun(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	config := rungen.RunConfig{Seed: 12345, GeneratorVersion: rungen.CurrentGeneratorVersion}

	a, err := rungen.Generate(config, definition)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rungen.Generate(config, definition)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(a.Placements, b.Placements) || !reflect.DeepEqual(a.Validation.Witness, b.Validation.Witness) {
		t.Fatalf("same seed differed: %+v / %+v", a, b)
	}
}

func TestGenerateReturnsUntouchedInitialState(t *testing.T) {
	run := generatePrototype(t, 7)

	if run.State.CurrentRoomID != scenario.RoomReception {
		t.Fatalf("room = %q, want reception", run.State.CurrentRoomID)
	}
	if run.State.NowSeconds != 0 || len(run.State.Inventory) != 0 || len(run.State.DiscoveredItems) != 0 || run.State.ActiveLight {
		t.Fatalf("returned state was consumed: %+v", run.State)
	}
}

func TestGenerateRejectsUnsupportedVersion(t *testing.T) {
	_, err := rungen.Generate(
		rungen.RunConfig{Seed: 1, GeneratorVersion: 99},
		runscenario.PrototypeDefinition(),
	)

	if !errors.Is(err, rungen.ErrUnsupportedVersion) {
		t.Fatalf("error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestGenerateFailsWhenNoPlacementIsPlayable(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	build := definition.Build
	definition.Build = func() *world.State {
		state := build()
		door := state.Doors[scenario.DoorStairwell]
		door.RequiredKey = "missing_key"
		state.Doors[door.ID] = door
		return state
	}

	_, err := rungen.Generate(
		rungen.RunConfig{Seed: 1, GeneratorVersion: rungen.CurrentGeneratorVersion},
		definition,
	)

	if !errors.Is(err, rungen.ErrNoPlayableRun) {
		t.Fatalf("error = %v, want ErrNoPlayableRun", err)
	}
}

func TestEveryPrototypePlacementCombinationProvesAndReplays(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	for _, flashlight := range definition.ItemRules[0].Candidates {
		for _, key := range definition.ItemRules[1].Candidates {
			placements := []rungen.Placement{
				{ItemID: scenario.ItemFlashlight, ObjectID: flashlight.ObjectID},
				{ItemID: scenario.ItemBrassKey, ObjectID: key.ObjectID},
			}
			state := definition.Build()
			if err := rungen.ApplyPlacements(state, placements); err != nil {
				t.Fatal(err)
			}
			proof, err := rungen.Validate(definition, state)
			if err != nil || !proof.Valid {
				t.Fatalf("placements %+v: proof=%+v err=%v", placements, proof, err)
			}
			if err := rungen.Replay(definition, placements, proof.Witness); err != nil {
				t.Fatalf("placements %+v: %v", placements, err)
			}
		}
	}
}

func TestPrototypeSeedSweep(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	seen := make(map[string]bool)
	for seed := int64(1); seed <= 1000; seed++ {
		run, err := rungen.Generate(
			rungen.RunConfig{Seed: seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
			definition,
		)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		seen[fmt.Sprint(run.Placements)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("1,000 seeds produced %d unique placements", len(seen))
	}
}

func generatePrototype(t *testing.T, seed int64) rungen.GeneratedRun {
	t.Helper()
	run, err := rungen.Generate(
		rungen.RunConfig{Seed: seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
		runscenario.PrototypeDefinition(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return run
}
