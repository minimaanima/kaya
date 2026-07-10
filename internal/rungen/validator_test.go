package rungen_test

import (
	"errors"
	"testing"

	"kaya/internal/game"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

func TestValidateReturnsWitnessForPrototypePlacement(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	state := definition.Build()
	placements := []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectReceptionDesk},
		{ItemID: scenario.ItemBrassKey, ObjectID: scenario.ObjectBodyCabinet},
	}
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatal(err)
	}

	result, err := rungen.Validate(definition, state)

	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || len(result.Witness) == 0 {
		t.Fatalf("validation = %+v", result)
	}
	if result.Witness[len(result.Witness)-1].ExpectedOutcome != "moved" {
		t.Fatalf("last witness step = %+v", result.Witness[len(result.Witness)-1])
	}
}

func TestValidateRejectsKeyBehindWinDoor(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	state := definition.Build()
	const hiddenObject game.ObjectID = "stairwell_locker"
	room := state.Rooms[scenario.RoomStairwell]
	room.Objects = append(room.Objects, hiddenObject)
	state.Rooms[room.ID] = room
	state.Objects[hiddenObject] = world.Object{ID: hiddenObject, Name: "Stairwell Locker", Searchable: true}
	placements := []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectReceptionDesk},
		{ItemID: scenario.ItemBrassKey, ObjectID: hiddenObject},
	}
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatal(err)
	}

	result, err := rungen.Validate(definition, state)

	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("key behind win door validated: %+v", result)
	}
}

func TestValidateRejectsMissingRequiredItem(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	state := definition.Build()
	if err := rungen.ApplyPlacements(state, []rungen.Placement{{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectReceptionDesk}}); err != nil {
		t.Fatal(err)
	}

	_, err := rungen.Validate(definition, state)

	if !errors.Is(err, rungen.ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateRejectsDoorWithUnavailableKey(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	state := definition.Build()
	placements := []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectReceptionDesk},
		{ItemID: scenario.ItemBrassKey, ObjectID: scenario.ObjectBodyCabinet},
	}
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatal(err)
	}
	door := state.Doors[scenario.DoorStairwell]
	door.RequiredKey = "missing_key"
	state.Doors[door.ID] = door

	result, err := rungen.Validate(definition, state)

	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("unavailable door key validated: %+v", result)
	}
}

func TestValidateRejectsFlashlightInDarkStorage(t *testing.T) {
	definition := runscenario.PrototypeDefinition()
	state := definition.Build()
	placements := []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectStorageCabinet},
		{ItemID: scenario.ItemBrassKey, ObjectID: scenario.ObjectBodyCabinet},
	}
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatal(err)
	}

	result, err := rungen.Validate(definition, state)

	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.Reason != "win room unreachable" {
		t.Fatalf("validation = %+v", result)
	}
}
