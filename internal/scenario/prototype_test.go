package scenario

import (
	"testing"

	"kaya/internal/game"
	"kaya/internal/rungen"
)

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
	if len(state.Objects) != 6 {
		t.Fatalf("objects len = %d, want 6", len(state.Objects))
	}
	if len(state.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(state.Items))
	}
}

func TestNewPrototypeTemplateHasSixEmptyCandidateObjects(t *testing.T) {
	state := NewPrototypeTemplate()

	if len(state.Objects) != 6 {
		t.Fatalf("objects = %d, want 6", len(state.Objects))
	}
	for _, object := range state.Objects {
		if containsItem(object.ContainedItems, ItemFlashlight) || containsItem(object.ContainedItems, ItemBrassKey) {
			t.Fatalf("template object %q contains randomized item: %v", object.ID, object.ContainedItems)
		}
	}
}

func TestNewPrototypeWorldKeepsFixedPlacements(t *testing.T) {
	state := NewPrototypeWorld()

	if !containsItem(state.Objects[ObjectReceptionDesk].ContainedItems, ItemFlashlight) {
		t.Fatal("fixed world flashlight not in reception desk")
	}
	if !containsItem(state.Objects[ObjectBodyCabinet].ContainedItems, ItemBrassKey) {
		t.Fatal("fixed world key not on doctor near cabinet")
	}
}

func TestPrototypeRunDefinitionHasThreeCandidatesPerItem(t *testing.T) {
	definition := PrototypeRunDefinition()

	if err := rungen.ValidateDefinition(definition); err != nil {
		t.Fatal(err)
	}
	if len(definition.ItemRules) != 2 {
		t.Fatalf("rules = %d, want 2", len(definition.ItemRules))
	}
	for _, rule := range definition.ItemRules {
		if len(rule.Candidates) != 3 {
			t.Fatalf("item %q candidates = %d, want 3", rule.ItemID, len(rule.Candidates))
		}
	}
}

func containsItem(items []game.ItemID, wanted game.ItemID) bool {
	for _, itemID := range items {
		if itemID == wanted {
			return true
		}
	}
	return false
}
