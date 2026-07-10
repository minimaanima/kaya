package rungen_test

import (
	"strings"
	"testing"

	"kaya/internal/kaya"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

func TestReplayWitnessReachesPrototypeWinRoom(t *testing.T) {
	definition, placements, witness := provenPrototype(t)

	if err := rungen.Replay(definition, placements, witness); err != nil {
		t.Fatal(err)
	}
}

func TestReplayRejectsAutonomyRefusal(t *testing.T) {
	definition, placements, witness := provenPrototype(t)
	build := definition.Build
	definition.Build = func() *world.State {
		state := build()
		state.Kaya = kaya.State{Stress: 85, Trust: 5, Fear: 80}
		return state
	}

	err := rungen.Replay(definition, placements, witness)

	if err == nil || !strings.Contains(err.Error(), "kaya_refused") {
		t.Fatalf("error = %v, want kaya_refused", err)
	}
}

func provenPrototype(t *testing.T) (rungen.Definition, []rungen.Placement, []rungen.WitnessStep) {
	t.Helper()
	definition := runscenario.PrototypeDefinition()
	placements := []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectReceptionDesk},
		{ItemID: scenario.ItemBrassKey, ObjectID: scenario.ObjectBodyCabinet},
	}
	state := definition.Build()
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatal(err)
	}
	proof, err := rungen.Validate(definition, state)
	if err != nil || !proof.Valid {
		t.Fatalf("proof=%+v err=%v", proof, err)
	}
	return definition, placements, proof.Witness
}
