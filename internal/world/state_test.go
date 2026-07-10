package world

import (
	"errors"
	"testing"

	"kaya/internal/game"
)

func TestCanSeeObjectUsesExplicitLightState(t *testing.T) {
	room := Room{Visibility: VisibilityPitchBlack}
	object := Object{RequiresLight: false}

	if CanSeeObject(room, object, false) {
		t.Fatal("pitch-black object visible without light")
	}
	if !CanSeeObject(room, object, true) {
		t.Fatal("object hidden with active light")
	}
}

const (
	roomReception game.RoomID   = "reception"
	roomStorage   game.RoomID   = "storage"
	roomStairwell game.RoomID   = "stairwell"
	doorStairwell game.DoorID   = "door_stairwell"
	objectTable   game.ObjectID = "table"
	objectBodyA   game.ObjectID = "body_a"
	objectBodyB   game.ObjectID = "body_b"
	itemKey       game.ItemID   = "brass_key"
	itemFlash     game.ItemID   = "flashlight"
)

func TestCurrentRoom(t *testing.T) {
	state := testState()

	got, err := state.CurrentRoom()
	if err != nil {
		t.Fatalf("CurrentRoom returned error: %v", err)
	}
	if got.ID != roomReception {
		t.Fatalf("CurrentRoom ID = %q, want %q", got.ID, roomReception)
	}
}

func TestCurrentRoomMissing(t *testing.T) {
	state := NewState("missing")

	_, err := state.CurrentRoom()
	if !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("CurrentRoom error = %v, want ErrRoomNotFound", err)
	}
}

func TestAvailableExits(t *testing.T) {
	state := testState()

	got, err := state.AvailableExits()
	if err != nil {
		t.Fatalf("AvailableExits returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("AvailableExits len = %d, want 1", len(got))
	}
	if got[0].Direction != "east" {
		t.Fatalf("exit direction = %q, want east", got[0].Direction)
	}
}

func TestVisibleObjectsInLitRoom(t *testing.T) {
	state := testState()

	got, err := state.VisibleObjects()
	if err != nil {
		t.Fatalf("VisibleObjects returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("VisibleObjects len = %d, want 1", len(got))
	}
	if got[0].ID != objectTable {
		t.Fatalf("visible object = %q, want %q", got[0].ID, objectTable)
	}
}

func TestVisibleObjectsInPitchBlackRoomWithoutLight(t *testing.T) {
	state := testState()
	state.CurrentRoomID = roomStorage

	got, err := state.VisibleObjects()
	if err != nil {
		t.Fatalf("VisibleObjects returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("VisibleObjects len = %d, want 0", len(got))
	}
}

func TestVisibleObjectsInDarkRoomWithLight(t *testing.T) {
	state := testState()
	state.CurrentRoomID = roomStorage
	state.ActiveLight = true

	got, err := state.VisibleObjects()
	if err != nil {
		t.Fatalf("VisibleObjects returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("VisibleObjects len = %d, want 2", len(got))
	}
}

func TestInventory(t *testing.T) {
	state := testState()

	if state.HasItem(itemKey) {
		t.Fatal("HasItem(key) = true before item was added")
	}

	state.AddInventory(itemKey)
	if !state.HasItem(itemKey) {
		t.Fatal("HasItem(key) = false after item was added")
	}

	state.RemoveInventory(itemKey)
	if state.HasItem(itemKey) {
		t.Fatal("HasItem(key) = true after item was removed")
	}
}

func TestResolveObjectSingleMatch(t *testing.T) {
	state := testState()

	got, err := state.ResolveObject("table")
	if err != nil {
		t.Fatalf("ResolveObject returned error: %v", err)
	}
	if !got.Found() {
		t.Fatalf("ResolveObject Found = false; matches = %+v", got.Matches)
	}
	if got.Matches[0].ID != objectTable {
		t.Fatalf("matched object = %q, want %q", got.Matches[0].ID, objectTable)
	}
}

func TestResolveObjectAmbiguousMatch(t *testing.T) {
	state := testState()
	state.CurrentRoomID = roomStorage
	state.ActiveLight = true

	got, err := state.ResolveObject("doctor")
	if err != nil {
		t.Fatalf("ResolveObject returned error: %v", err)
	}
	if !got.Ambiguous() {
		t.Fatalf("ResolveObject Ambiguous = false; matches = %+v", got.Matches)
	}
	if len(got.Matches) != 2 {
		t.Fatalf("matches len = %d, want 2", len(got.Matches))
	}
}

func TestResolveObjectRespectsVisibility(t *testing.T) {
	state := testState()
	state.CurrentRoomID = roomStorage

	got, err := state.ResolveObject("doctor near cabinet")
	if err != nil {
		t.Fatalf("ResolveObject returned error: %v", err)
	}
	if !got.Missing() {
		t.Fatalf("ResolveObject Missing = false; matches = %+v", got.Matches)
	}
}

func TestResolveDoorByNameAndDirection(t *testing.T) {
	state := testState()
	state.CurrentRoomID = roomStorage

	byName, err := state.ResolveDoor("stairwell door")
	if err != nil {
		t.Fatalf("ResolveDoor by name returned error: %v", err)
	}
	if !byName.Found() {
		t.Fatalf("ResolveDoor by name Found = false; matches = %+v", byName.Matches)
	}

	byDirection, err := state.ResolveDoor("north")
	if err != nil {
		t.Fatalf("ResolveDoor by direction returned error: %v", err)
	}
	if !byDirection.Found() {
		t.Fatalf("ResolveDoor by direction Found = false; matches = %+v", byDirection.Matches)
	}
}

func testState() *State {
	state := NewState(roomReception)
	state.Rooms[roomReception] = Room{
		ID:          roomReception,
		Name:        "Reception",
		Description: "A damaged reception area.",
		Visibility:  VisibilityLit,
		Exits: []Exit{{
			Direction: "east",
			To:        roomStorage,
		}},
		Objects: []game.ObjectID{objectTable},
	}
	state.Rooms[roomStorage] = Room{
		ID:          roomStorage,
		Name:        "Storage Room",
		Description: "A dark storage room with overturned cabinets.",
		Visibility:  VisibilityPitchBlack,
		Exits: []Exit{{
			Direction: "west",
			To:        roomReception,
		}, {
			Direction: "north",
			To:        roomStairwell,
			Door:      doorStairwell,
		}},
		Objects: []game.ObjectID{objectBodyA, objectBodyB},
	}
	state.Rooms[roomStairwell] = Room{
		ID:          roomStairwell,
		Name:        "Emergency Stairwell",
		Description: "A concrete stairwell beyond a locked fire door.",
		Visibility:  VisibilityDim,
	}

	state.Doors[doorStairwell] = Door{
		ID:          doorStairwell,
		Name:        "Emergency Stairwell Door",
		Aliases:     []string{"stairwell door", "fire door", "emergency door"},
		From:        roomStorage,
		To:          roomStairwell,
		State:       DoorLocked,
		RequiredKey: itemKey,
	}

	state.Objects[objectTable] = Object{
		ID:             objectTable,
		Name:           "Reception Desk",
		Aliases:        []string{"desk", "table", "front desk", "drawer", "drawers"},
		Description:    "A cracked laminate desk with drawers hanging open.",
		Kind:           ObjectSurface,
		Searchable:     true,
		ContainedItems: []game.ItemID{itemFlash},
	}
	state.Objects[objectBodyA] = Object{
		ID:             objectBodyA,
		Name:           "Doctor Near Cabinet",
		Aliases:        []string{"doctor", "body", "corpse", "doctor near cabinet", "coat pockets"},
		Description:    "A dead doctor slumped beside a metal cabinet.",
		Kind:           ObjectBody,
		RequiresLight:  true,
		Searchable:     true,
		ContainedItems: []game.ItemID{itemKey},
	}
	state.Objects[objectBodyB] = Object{
		ID:            objectBodyB,
		Name:          "Doctor Near Door",
		Aliases:       []string{"doctor", "body", "corpse", "doctor near door"},
		Description:   "A dead doctor collapsed near the stairwell door.",
		Kind:          ObjectBody,
		RequiresLight: false,
		Searchable:    true,
	}

	state.Items[itemKey] = Item{
		ID:          itemKey,
		Name:        "Brass Key",
		Aliases:     []string{"key", "small key", "brass key"},
		Description: "A small brass key.",
		Portable:    true,
	}
	state.Items[itemFlash] = Item{
		ID:          itemFlash,
		Name:        "Flashlight",
		Aliases:     []string{"torch", "light"},
		Description: "A heavy flashlight with a weak beam.",
		Portable:    true,
	}

	return state
}
