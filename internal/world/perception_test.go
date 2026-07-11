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

func TestPerceptionSnapshotFiltersHiddenStaleAndUndiscoveredReferents(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.DiscoverItem(scenario.ItemFlashlight)
	state.RecentReferents = []game.ReferentGroup{
		{ObjectIDs: []game.ObjectID{scenario.ObjectReceptionDesk, scenario.ObjectBodyCabinet, "missing_object"}},
		{ItemIDs: []game.ItemID{scenario.ItemFlashlight, scenario.ItemBrassKey, "missing_item"}},
	}

	snapshot, err := state.PerceptionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.RecentReferents[0].ObjectIDs; len(got) != 1 || got[0] != scenario.ObjectReceptionDesk {
		t.Fatalf("object referents = %#v", got)
	}
	if got := snapshot.RecentReferents[1].ItemIDs; len(got) != 1 || got[0] != scenario.ItemFlashlight {
		t.Fatalf("item referents = %#v", got)
	}
	encoded, _ := json.Marshal(snapshot.RecentReferents)
	for _, hidden := range [][]byte{[]byte(scenario.ObjectBodyCabinet), []byte(scenario.ItemBrassKey), []byte("missing_object"), []byte("missing_item")} {
		if bytes.Contains(encoded, hidden) {
			t.Fatalf("referents leaked %q: %s", hidden, encoded)
		}
	}
}

func TestPerceptionSnapshotDropsObjectReferentsAfterRoomChange(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.RememberObjects([]game.ObjectID{scenario.ObjectReceptionDesk})
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil {
		t.Fatal(err)
	}

	snapshot, err := state.PerceptionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.RecentReferents) != 0 {
		t.Fatalf("recent referents = %#v", snapshot.RecentReferents)
	}
}

func TestPluralPronounReturnsAuthoredRoomOrder(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.ActiveLight = true
	state.RememberObjects([]game.ObjectID{scenario.ObjectBodyDoor, scenario.ObjectBodyCabinet})

	got, err := state.ResolveObjectGroup("them", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 2 || got.Matches[0].ID != scenario.ObjectBodyCabinet || got.Matches[1].ID != scenario.ObjectBodyDoor {
		t.Fatalf("matches = %#v", got.Matches)
	}
}

func TestRememberReferentCoalescesDuplicateAndMovesItNewest(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.DiscoverItem(scenario.ItemFlashlight)
	state.RememberObjects([]game.ObjectID{scenario.ObjectReceptionDesk, scenario.ObjectReceptionFloor})
	state.RememberItems([]game.ItemID{scenario.ItemFlashlight})
	state.RememberObjects([]game.ObjectID{scenario.ObjectReceptionFloor, scenario.ObjectReceptionDesk})

	if len(state.RecentReferents) != 2 {
		t.Fatalf("recent referents = %#v", state.RecentReferents)
	}
	if got := state.RecentReferents[0].ItemIDs; len(got) != 1 || got[0] != scenario.ItemFlashlight {
		t.Fatalf("oldest referent = %#v", state.RecentReferents[0])
	}
	if got := state.RecentReferents[1].ObjectIDs; len(got) != 2 {
		t.Fatalf("newest referent = %#v", state.RecentReferents[1])
	}
}

func TestResolveObjectGroupDistinguishesAllFromAmbiguous(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.ActiveLight = true

	ambiguous, err := state.ResolveObjectGroup("doctor", false)
	if err != nil {
		t.Fatal(err)
	}
	if !ambiguous.Ambiguous() || ambiguous.Found() {
		t.Fatalf("ambiguous resolution = %#v", ambiguous)
	}

	selected, err := state.ResolveObjectGroup("doctor", true)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Ambiguous() || !selected.Found() || len(selected.Matches) != 2 {
		t.Fatalf("all resolution = %#v", selected)
	}
}
