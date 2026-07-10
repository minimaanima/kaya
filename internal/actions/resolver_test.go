package actions

import (
	"strings"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/kaya"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

func TestInspectRoom(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionInspect})

	if got.Outcome != "inspected_room" {
		t.Fatalf("Outcome = %q, want inspected_room", got.Outcome)
	}
	if !hasFactContaining(got, "Reception Desk") {
		t.Fatalf("facts = %+v, want Reception Desk", got.VisibleFacts)
	}
}

func TestMoveToStorage(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})

	if got.Outcome != "moved" {
		t.Fatalf("Outcome = %q, want moved", got.Outcome)
	}
	if got.StartedAtSeconds != 0 {
		t.Fatalf("StartedAtSeconds = %d, want 0", got.StartedAtSeconds)
	}
	if got.DurationSeconds != 20 {
		t.Fatalf("DurationSeconds = %d, want 20", got.DurationSeconds)
	}
	if state.NowSeconds != 20 {
		t.Fatalf("NowSeconds = %d, want 20", state.NowSeconds)
	}
	if state.CurrentRoomID != scenario.RoomStorage {
		t.Fatalf("CurrentRoomID = %q, want %q", state.CurrentRoomID, scenario.RoomStorage)
	}
}

func TestHighStressCanBlockRiskyMoveIntoDarkRoom(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.Kaya = kaya.State{
		Stress: 85,
		Trust:  5,
		Fear:   80,
	}
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})

	if got.Outcome != "kaya_refused" {
		t.Fatalf("Outcome = %q, want kaya_refused", got.Outcome)
	}
	if state.CurrentRoomID != scenario.RoomReception {
		t.Fatalf("CurrentRoomID = %q, want reception", state.CurrentRoomID)
	}
	if state.NowSeconds != 0 {
		t.Fatalf("NowSeconds = %d, want 0", state.NowSeconds)
	}
}

func TestHighTrustCanAskConfirmationForRiskyMoveIntoDarkRoom(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.Kaya = kaya.State{
		Stress: 55,
		Trust:  90,
		Fear:   55,
	}
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})

	if got.Outcome != "kaya_needs_confirmation" {
		t.Fatalf("Outcome = %q, want kaya_needs_confirmation", got.Outcome)
	}
	if !got.NeedsClarification {
		t.Fatal("NeedsClarification = false, want true")
	}
	if state.CurrentRoomID != scenario.RoomReception {
		t.Fatalf("CurrentRoomID = %q, want reception", state.CurrentRoomID)
	}
}

func TestMoveBlockedByLockedDoor(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "north"})

	if got.Outcome != "door_blocked" {
		t.Fatalf("Outcome = %q, want door_blocked", got.Outcome)
	}
	if state.NowSeconds != 2 {
		t.Fatalf("NowSeconds = %d, want 2", state.NowSeconds)
	}
	if state.CurrentRoomID != scenario.RoomStorage {
		t.Fatalf("CurrentRoomID = %q, want storage", state.CurrentRoomID)
	}
}

func TestSearchFindsContainedItems(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "desk"})

	if got.Outcome != "searched_found_items" {
		t.Fatalf("Outcome = %q, want searched_found_items", got.Outcome)
	}
	if !hasFactContaining(got, "Flashlight") {
		t.Fatalf("facts = %+v, want Flashlight", got.VisibleFacts)
	}
	if !state.IsItemDiscovered(scenario.ItemFlashlight) {
		t.Fatal("flashlight was not marked discovered")
	}
	if state.HasItem(scenario.ItemFlashlight) {
		t.Fatal("search added flashlight to inventory; search should only discover")
	}
	if state.LastMentionedItemID != scenario.ItemFlashlight {
		t.Fatalf("LastMentionedItemID = %q, want %q", state.LastMentionedItemID, scenario.ItemFlashlight)
	}
}

func TestInspectDrawersDescribesDesk(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionInspect, Target: "drawers"})

	if got.Outcome != "inspected_object" {
		t.Fatalf("Outcome = %q, want inspected_object", got.Outcome)
	}
	if !hasFactContaining(got, "drawers hanging open") {
		t.Fatalf("facts = %+v, want drawer description", got.VisibleFacts)
	}
}

func TestSearchDrawersFindsFlashlight(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "drawers"})

	if got.Outcome != "searched_found_items" {
		t.Fatalf("Outcome = %q, want searched_found_items", got.Outcome)
	}
	if !hasFactContaining(got, "Flashlight") {
		t.Fatalf("facts = %+v, want Flashlight", got.VisibleFacts)
	}
}

func TestTakeItemRequiresDiscovery(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionTakeItem, Target: "flashlight"})

	if got.Outcome != "item_not_discovered" {
		t.Fatalf("Outcome = %q, want item_not_discovered", got.Outcome)
	}
	if state.HasItem(scenario.ItemFlashlight) {
		t.Fatal("flashlight was added to inventory before discovery")
	}
}

func TestTakeItemAddsInventory(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "desk"})
	got := resolver.Resolve(intent.Intent{Action: intent.ActionTakeItem, Target: "flashlight"})

	if got.Outcome != "item_taken" {
		t.Fatalf("Outcome = %q, want item_taken", got.Outcome)
	}
	if !state.HasItem(scenario.ItemFlashlight) {
		t.Fatal("flashlight was not added to inventory")
	}
}

func TestTakeItUsesLastMentionedDiscoveredItem(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "front desk"})
	got := resolver.Resolve(intent.Intent{Action: intent.ActionTakeItem, Target: "it"})

	if got.Outcome != "item_taken" {
		t.Fatalf("Outcome = %q, want item_taken", got.Outcome)
	}
	if !state.HasItem(scenario.ItemFlashlight) {
		t.Fatal("flashlight was not added to inventory")
	}
}

func TestTakeItAfterMultipleFoundAsksClarification(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	const badgeItem game.ItemID = "security_badge"
	state.Items[badgeItem] = world.Item{
		ID:          badgeItem,
		Name:        "Security Badge",
		Aliases:     []string{"badge"},
		Description: "A scratched access badge.",
		Portable:    true,
	}
	desk := state.Objects[scenario.ObjectReceptionDesk]
	desk.ContainedItems = append(desk.ContainedItems, badgeItem)
	state.Objects[scenario.ObjectReceptionDesk] = desk
	resolver := NewResolver(state)

	resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "front desk"})
	got := resolver.Resolve(intent.Intent{Action: intent.ActionTakeItem, Target: "it"})

	if !got.NeedsClarification {
		t.Fatal("NeedsClarification = false, want true")
	}
	if state.HasItem(scenario.ItemFlashlight) || state.HasItem(badgeItem) {
		t.Fatal("take it should not choose from multiple newly found items")
	}
}

func TestTakeNonPortableItemFails(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	const heavyItem game.ItemID = "heavy_safe"
	state.Items[heavyItem] = world.Item{
		ID:          heavyItem,
		Name:        "Heavy Safe",
		Aliases:     []string{"safe"},
		Description: "A heavy bolted safe.",
		Portable:    false,
	}
	desk := state.Objects[scenario.ObjectReceptionDesk]
	desk.ContainedItems = append(desk.ContainedItems, heavyItem)
	state.Objects[scenario.ObjectReceptionDesk] = desk
	resolver := NewResolver(state)

	resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "desk"})
	got := resolver.Resolve(intent.Intent{Action: intent.ActionTakeItem, Target: "safe"})

	if got.Outcome != "item_not_portable" {
		t.Fatalf("Outcome = %q, want item_not_portable", got.Outcome)
	}
	if state.HasItem(heavyItem) {
		t.Fatal("non-portable item was added to inventory")
	}
}

func TestTakeUndiscoveredNonPortableItemDoesNotLeakPortability(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	const heavyItem game.ItemID = "heavy_safe"
	state.Items[heavyItem] = world.Item{
		ID:          heavyItem,
		Name:        "Heavy Safe",
		Aliases:     []string{"safe"},
		Description: "A heavy bolted safe.",
		Portable:    false,
	}
	desk := state.Objects[scenario.ObjectReceptionDesk]
	desk.ContainedItems = append(desk.ContainedItems, heavyItem)
	state.Objects[scenario.ObjectReceptionDesk] = desk
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionTakeItem, Target: "safe"})

	if got.Outcome != "item_not_discovered" {
		t.Fatalf("Outcome = %q, want item_not_discovered", got.Outcome)
	}
}

func TestTurnOnFlashlightRevealsDarkRoomObjects(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.AddInventory(scenario.ItemFlashlight)
	resolver := NewResolver(state)

	before, err := state.VisibleObjects()
	if err != nil {
		t.Fatalf("VisibleObjects before returned error: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("visible before len = %d, want 0", len(before))
	}

	got := resolver.Resolve(intent.Intent{Action: intent.ActionTurnOn, Target: "flashlight"})

	if got.Outcome != "flashlight_on" {
		t.Fatalf("Outcome = %q, want flashlight_on", got.Outcome)
	}
	after, err := state.VisibleObjects()
	if err != nil {
		t.Fatalf("VisibleObjects after returned error: %v", err)
	}
	if len(after) != 3 {
		t.Fatalf("visible after len = %d, want 3", len(after))
	}
}

func TestTurnOnFlashlightRequiresInventory(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionTurnOn, Target: "flashlight"})

	if got.Outcome != "missing_item" {
		t.Fatalf("Outcome = %q, want missing_item", got.Outcome)
	}
	if state.ActiveLight {
		t.Fatal("ActiveLight = true without flashlight in inventory")
	}
}

func TestUseYourFlashlightTurnsOnInventoryFlashlight(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.AddInventory(scenario.ItemFlashlight)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionUseItem, Target: "your flashlight"})

	if got.Outcome != "flashlight_on" {
		t.Fatalf("Outcome = %q, want flashlight_on", got.Outcome)
	}
	if !state.ActiveLight {
		t.Fatal("ActiveLight = false, want true")
	}
}

func TestTurnItOnInfersOnlyCarriedFlashlight(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionTurnOn, Target: "it"})

	if got.Outcome != "flashlight_on" {
		t.Fatalf("Outcome = %q, want flashlight_on", got.Outcome)
	}
	if !state.ActiveLight {
		t.Fatal("ActiveLight = false, want true")
	}
}

func TestAmbiguousDoctorSearchAsksClarification(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.ActiveLight = true
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "doctor"})

	if !got.NeedsClarification {
		t.Fatal("NeedsClarification = false, want true")
	}
	if state.NowSeconds != 0 {
		t.Fatalf("NowSeconds = %d, want 0", state.NowSeconds)
	}
	if !strings.Contains(got.ClarificationQuestion, "Doctor Near Cabinet") {
		t.Fatalf("ClarificationQuestion = %q, want Doctor Near Cabinet", got.ClarificationQuestion)
	}
	if !strings.Contains(got.ClarificationQuestion, "Doctor Near Door") {
		t.Fatalf("ClarificationQuestion = %q, want Doctor Near Door", got.ClarificationQuestion)
	}
}

func TestHighStressCanBlockRiskyBodySearch(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.ActiveLight = true
	state.Kaya = kaya.State{
		Stress: 80,
		Trust:  5,
		Fear:   80,
	}
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "doctor near cabinet"})

	if got.Outcome != "kaya_refused" {
		t.Fatalf("Outcome = %q, want kaya_refused", got.Outcome)
	}
	if state.IsItemDiscovered(scenario.ItemBrassKey) {
		t.Fatal("brass key was discovered after refused search")
	}
}

func TestUseKeyUnlocksDoor(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.AddInventory(scenario.ItemBrassKey)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action: intent.ActionUseItem,
		Item:   "key",
		Target: "stairwell door",
	})

	if got.Outcome != "door_unlocked" {
		t.Fatalf("Outcome = %q, want door_unlocked", got.Outcome)
	}
	if state.Doors[scenario.DoorStairwell].State != world.DoorClosed {
		t.Fatalf("door state = %q, want closed", state.Doors[scenario.DoorStairwell].State)
	}
}

func TestUseKeyInfersOnlyMatchingNearbyDoor(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.AddInventory(scenario.ItemBrassKey)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action: intent.ActionUseItem,
		Item:   "key",
	})

	if got.Outcome != "door_unlocked" {
		t.Fatalf("Outcome = %q, want door_unlocked", got.Outcome)
	}
	if state.Doors[scenario.DoorStairwell].State != world.DoorClosed {
		t.Fatalf("door state = %q, want closed", state.Doors[scenario.DoorStairwell].State)
	}
}

func TestUseKeyWhenParserPutsKeyInTarget(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.AddInventory(scenario.ItemBrassKey)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action: intent.ActionUseItem,
		Target: "key",
	})

	if got.Outcome != "door_unlocked" {
		t.Fatalf("Outcome = %q, want door_unlocked", got.Outcome)
	}
	if state.Doors[scenario.DoorStairwell].State != world.DoorClosed {
		t.Fatalf("door state = %q, want closed", state.Doors[scenario.DoorStairwell].State)
	}
}

func TestMoveThroughUnlockedDoor(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	door := state.Doors[scenario.DoorStairwell]
	door.State = world.DoorClosed
	state.Doors[scenario.DoorStairwell] = door
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "north"})

	if got.Outcome != "moved" {
		t.Fatalf("Outcome = %q, want moved", got.Outcome)
	}
	if state.CurrentRoomID != scenario.RoomStairwell {
		t.Fatalf("CurrentRoomID = %q, want %q", state.CurrentRoomID, scenario.RoomStairwell)
	}
}

func TestMoveBackUsesPreviousRoom(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})
	got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "back"})

	if got.Outcome != "moved" {
		t.Fatalf("Outcome = %q, want moved", got.Outcome)
	}
	if state.CurrentRoomID != scenario.RoomReception {
		t.Fatalf("CurrentRoomID = %q, want %q", state.CurrentRoomID, scenario.RoomReception)
	}
}

func TestInspectCurrentRoomByName(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionInspect, Target: "storage room"})

	if got.Outcome != "inspected_room" {
		t.Fatalf("Outcome = %q, want inspected_room", got.Outcome)
	}
	if !hasFactContaining(got, "pitch-black storage room") {
		t.Fatalf("facts = %+v, want storage room description", got.VisibleFacts)
	}
}

func TestInspectPitchBlackStorageShowsNoObjectsWithoutLight(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionInspect})

	if got.Outcome != "inspected_room" {
		t.Fatalf("Outcome = %q, want inspected_room", got.Outcome)
	}
	if hasFactContaining(got, "Doctor Near Door") {
		t.Fatalf("facts = %+v, should not reveal body without light", got.VisibleFacts)
	}
	if !hasFactContaining(got, "I cannot make out any distinct objects.") {
		t.Fatalf("facts = %+v, want darkness message", got.VisibleFacts)
	}
}

func TestInspectHiddenObjectInPitchBlackExplainsDarkness(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionInspect, Target: "body"})

	if got.Outcome != "object_not_visible" {
		t.Fatalf("Outcome = %q, want object_not_visible", got.Outcome)
	}
	if !hasFactContaining(got, "too dark") {
		t.Fatalf("facts = %+v, want darkness explanation", got.VisibleFacts)
	}
}

func TestTalkInventorySpecificItemPresent(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action:  intent.ActionTalk,
		Item:    "flashlight",
		RawText: "do you have flashlight",
	})

	if got.Outcome != "inventory_has_item" {
		t.Fatalf("Outcome = %q, want inventory_has_item", got.Outcome)
	}
	if !hasFactContaining(got, "Yes. I have Flashlight.") {
		t.Fatalf("facts = %+v, want flashlight confirmation", got.VisibleFacts)
	}
}

func TestTalkInventorySpecificItemMissing(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action:  intent.ActionTalk,
		Item:    "flashlight",
		RawText: "do you have flashlight",
	})

	if got.Outcome != "inventory_missing_item" {
		t.Fatalf("Outcome = %q, want inventory_missing_item", got.Outcome)
	}
	if !hasFactContaining(got, "No. I do not have flashlight.") {
		t.Fatalf("facts = %+v, want missing flashlight answer", got.VisibleFacts)
	}
}

func TestTalkInventoryList(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	state.AddInventory(scenario.ItemBrassKey)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action:  intent.ActionTalk,
		RawText: "what are you carrying",
	})

	if got.Outcome != "inventory_listed" {
		t.Fatalf("Outcome = %q, want inventory_listed", got.Outcome)
	}
	if !hasFactContaining(got, "Brass Key") {
		t.Fatalf("facts = %+v, want Brass Key", got.VisibleFacts)
	}
	if !hasFactContaining(got, "Flashlight") {
		t.Fatalf("facts = %+v, want Flashlight", got.VisibleFacts)
	}
}

func TestTalkInventoryKeywordListsInventory(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action: intent.ActionTalk,
		Item:   "inventory",
	})

	if got.Outcome != "inventory_listed" {
		t.Fatalf("Outcome = %q, want inventory_listed", got.Outcome)
	}
	if !hasFactContaining(got, "Flashlight") {
		t.Fatalf("facts = %+v, want Flashlight", got.VisibleFacts)
	}
}

func TestTalkItemLocationUnknownUntilDiscovered(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action:  intent.ActionTalk,
		Item:    "flashlight",
		RawText: "where is the flashlight",
	})

	if got.Outcome != "item_location_unknown" {
		t.Fatalf("Outcome = %q, want item_location_unknown", got.Outcome)
	}
	if !hasFactContaining(got, "I have not found Flashlight yet.") {
		t.Fatalf("facts = %+v, want not found answer", got.VisibleFacts)
	}
}

func TestTalkItemLocationAfterDiscovery(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	resolver.Resolve(intent.Intent{Action: intent.ActionSearch, Target: "desk"})
	got := resolver.Resolve(intent.Intent{
		Action:  intent.ActionTalk,
		Item:    "flashlight",
		RawText: "where is the flashlight",
	})

	if got.Outcome != "item_location_known" {
		t.Fatalf("Outcome = %q, want item_location_known", got.Outcome)
	}
	if !hasFactContaining(got, "I found Flashlight in Reception Desk.") {
		t.Fatalf("facts = %+v, want item location", got.VisibleFacts)
	}
}

func TestTalkItemLocationInInventory(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{
		Action:  intent.ActionTalk,
		Item:    "flashlight",
		RawText: "where is the flashlight",
	})

	if got.Outcome != "item_location_inventory" {
		t.Fatalf("Outcome = %q, want item_location_inventory", got.Outcome)
	}
	if !hasFactContaining(got, "I have Flashlight.") {
		t.Fatalf("facts = %+v, want inventory location", got.VisibleFacts)
	}
}

func TestWaitAdvancesTime(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionWait})

	if got.Outcome != "waited" {
		t.Fatalf("Outcome = %q, want waited", got.Outcome)
	}
	if state.NowSeconds != 10 {
		t.Fatalf("NowSeconds = %d, want 10", state.NowSeconds)
	}
}

func TestScheduledEventFiresDuringAction(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.ScheduleEvent(10, game.WorldEvent{
		Type:        game.EventSound,
		Description: "Something scrapes inside the ventilation shaft.",
		Danger:      game.DangerModerate,
	})
	resolver := NewResolver(state)

	got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})

	if len(got.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(got.Events))
	}
	if got.Events[0].Description != "Something scrapes inside the ventilation shaft." {
		t.Fatalf("event description = %q", got.Events[0].Description)
	}
	if state.NowSeconds != 20 {
		t.Fatalf("NowSeconds = %d, want 20", state.NowSeconds)
	}
	if state.Kaya.Stress == 0 {
		t.Fatal("Kaya stress did not change after danger event")
	}
}

func hasFactContaining(result game.ActionResult, text string) bool {
	for _, fact := range result.VisibleFacts {
		if strings.Contains(fact.Text, text) {
			return true
		}
	}
	return false
}
