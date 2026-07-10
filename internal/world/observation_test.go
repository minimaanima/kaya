package world_test

import (
	"testing"

	"kaya/internal/actions"
	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

func TestObserveObjectReturnsAuthoredFactOnceAndStoresIt(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.ActiveLight = true

	facts := state.ObserveObject(scenario.ObjectBodyCabinet, world.ObservationInspect)
	if len(facts) != 1 || facts[0].Kind != game.FactLifeStatus || facts[0].Value != "dead" || !facts[0].Required {
		t.Fatalf("facts = %#v", facts)
	}
	if repeated := state.ObserveObject(scenario.ObjectBodyCabinet, world.ObservationSearch); len(repeated) != 0 {
		t.Fatalf("repeated facts = %#v", repeated)
	}
	stored, ok := state.ObservedFact(scenario.ObjectBodyCabinet, game.FactLifeStatus)
	if !ok || stored.Value != "dead" {
		t.Fatalf("stored fact = %#v, %v", stored, ok)
	}
}

func TestInspectObjectReturnsObservationAndRemembersTarget(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	resolver := actions.NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionInspect, Target: "doctor near cabinet"})
	if got.Status != game.ActionSucceeded {
		t.Fatalf("status = %q", got.Status)
	}
	if len(got.TargetObjectIDs) != 1 || got.TargetObjectIDs[0] != scenario.ObjectBodyCabinet {
		t.Fatalf("target ids = %#v", got.TargetObjectIDs)
	}
	if _, ok := state.ObservedFact(scenario.ObjectBodyCabinet, game.FactLifeStatus); !ok {
		t.Fatal("life status was not observed")
	}
	resolved, err := state.ResolveObjectGroup("it", false)
	if err != nil || !resolved.Found() || resolved.Matches[0].ID != scenario.ObjectBodyCabinet {
		t.Fatalf("resolved = %#v, err = %v", resolved, err)
	}
}
