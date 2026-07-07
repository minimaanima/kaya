package scenario

import "testing"

func TestNewPrototypeWorldLoads(t *testing.T) {
	state := NewPrototypeWorld()

	room, err := state.CurrentRoom()
	if err != nil {
		t.Fatalf("CurrentRoom returned error: %v", err)
	}
	if room.ID != RoomReception {
		t.Fatalf("current room = %q, want %q", room.ID, RoomReception)
	}
	if len(state.Rooms) != 3 {
		t.Fatalf("rooms len = %d, want 3", len(state.Rooms))
	}
	if len(state.Doors) != 1 {
		t.Fatalf("doors len = %d, want 1", len(state.Doors))
	}
	if len(state.Objects) != 3 {
		t.Fatalf("objects len = %d, want 3", len(state.Objects))
	}
	if len(state.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(state.Items))
	}
}
