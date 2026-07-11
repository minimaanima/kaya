package rungen

import (
	"errors"
	"reflect"
	"testing"

	"kaya/internal/game"
)

func TestApplyPlacementsPlacesEachItemOnce(t *testing.T) {
	state := validTestState()

	err := ApplyPlacements(state, []Placement{{ItemID: "flashlight", ObjectID: "desk"}})

	if err != nil {
		t.Fatal(err)
	}
	if got := state.Objects["desk"].ContainedItems; !reflect.DeepEqual(got, []game.ItemID{"flashlight"}) {
		t.Fatalf("contained items = %v", got)
	}
}

func TestApplyPlacementsRejectsDuplicateItem(t *testing.T) {
	state := validTestState()

	err := ApplyPlacements(state, []Placement{
		{ItemID: "flashlight", ObjectID: "desk"},
		{ItemID: "flashlight", ObjectID: "floor"},
	})

	if !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}

func TestApplyPlacementsIsAtomicWhenLaterPlacementFails(t *testing.T) {
	state := validTestState()

	err := ApplyPlacements(state, []Placement{
		{ItemID: "flashlight", ObjectID: "desk"},
		{ItemID: "missing", ObjectID: "floor"},
	})

	if !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
	if got := state.Objects["desk"].ContainedItems; len(got) != 0 {
		t.Fatalf("atomic placement mutated desk: %v", got)
	}
}

func TestApplyPlacementsRejectsAlreadyPlacedItem(t *testing.T) {
	state := validTestState()
	desk := state.Objects["desk"]
	desk.ContainedItems = []game.ItemID{"flashlight"}
	state.Objects["desk"] = desk

	err := ApplyPlacements(state, []Placement{{ItemID: "flashlight", ObjectID: "floor"}})

	if !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}
