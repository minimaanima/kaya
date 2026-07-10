package world_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"kaya/internal/actions"
	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/scenario"
)

func TestPerceptionSnapshotHidesDarkWorldData(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := actions.NewResolver(state)
	resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})

	snapshot, err := state.PerceptionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.VisibleObjects) != 0 {
		t.Fatalf("visible objects = %#v", snapshot.VisibleObjects)
	}
	if len(snapshot.KnownExits) != 1 || snapshot.KnownExits[0].Direction != "west" {
		t.Fatalf("known exits = %#v", snapshot.KnownExits)
	}
	encoded, _ := json.Marshal(snapshot)
	if bytes.Contains(encoded, []byte("brass_key")) || bytes.Contains(encoded, []byte("north")) {
		t.Fatalf("snapshot leaked hidden state: %s", encoded)
	}
}

func TestPluralPronounUsesLatestPerceivedGroup(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil {
		t.Fatal(err)
	}
	state.RememberObjects([]game.ObjectID{scenario.ObjectBodyCabinet, scenario.ObjectBodyDoor})

	got, err := state.ResolveObjectGroup("them", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 2 || got.Matches[0].ID != scenario.ObjectBodyCabinet || got.Matches[1].ID != scenario.ObjectBodyDoor {
		t.Fatalf("matches = %#v", got.Matches)
	}
}

func TestPerceptionSnapshotSortsInventoryAndBoundsReferents(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	state.AddInventory(scenario.ItemBrassKey)
	state.RememberObjects([]game.ObjectID{scenario.ObjectReceptionDesk})
	state.RememberItems([]game.ItemID{scenario.ItemFlashlight})
	state.RememberObjects([]game.ObjectID{scenario.ObjectReceptionFloor})
	state.RememberObjects([]game.ObjectID{scenario.ObjectCollapsedChair})

	snapshot, err := state.PerceptionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Inventory) != 2 || snapshot.Inventory[0].ID != scenario.ItemBrassKey || snapshot.Inventory[1].ID != scenario.ItemFlashlight {
		t.Fatalf("inventory = %#v", snapshot.Inventory)
	}
	if len(snapshot.RecentReferents) != 3 {
		t.Fatalf("recent referents = %#v", snapshot.RecentReferents)
	}
	if got := snapshot.RecentReferents[0].ItemIDs; len(got) != 1 || got[0] != scenario.ItemFlashlight {
		t.Fatalf("oldest retained referent = %#v", snapshot.RecentReferents[0])
	}
}

func TestResolveObjectGroupPluralizesLastWordAndPreservesRoomOrder(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.ActiveLight = true

	got, err := state.ResolveObjectGroup("doctors", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 2 || got.Matches[0].ID != scenario.ObjectBodyCabinet || got.Matches[1].ID != scenario.ObjectBodyDoor {
		t.Fatalf("matches = %#v", got.Matches)
	}
}
