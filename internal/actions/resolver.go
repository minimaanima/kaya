package actions

import (
	"fmt"
	"sort"
	"strings"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/world"
)

type Resolver struct {
	state *world.State
}

func NewResolver(state *world.State) Resolver {
	return Resolver{state: state}
}

func (r Resolver) Resolve(in intent.Intent) game.ActionResult {
	if in.NeedsClarification {
		return clarification(in.ClarificationQuestion)
	}
	if r.state == nil {
		return failed("missing_world", "The connection is unstable. I cannot read the room state.")
	}

	if result, ok := r.autonomyResult(in); ok {
		return result
	}

	var result game.ActionResult
	switch in.Action {
	case intent.ActionInspect:
		result = r.inspect(in)
	case intent.ActionMove:
		result = r.move(in)
	case intent.ActionSearch:
		result = r.search(in)
	case intent.ActionTakeItem:
		result = r.takeItem(in)
	case intent.ActionUseItem:
		result = r.useItem(in)
	case intent.ActionTurnOn:
		result = r.turnOn(in)
	case intent.ActionTurnOff:
		result = r.turnOff(in)
	case intent.ActionExplore:
		result = r.explore()
	case intent.ActionListen:
		result = makeResult("listened", 5, "I listen carefully. The damaged building creaks around me.")
	case intent.ActionWait:
		result = makeResult("waited", 10, "I wait where I am.")
	case intent.ActionTalk:
		return r.talk(in)
	case intent.ActionUnknown:
		return clarification("What do you want me to do?")
	default:
		result = failed("unsupported_action", fmt.Sprintf("I am not sure how to do that yet: %s.", in.Action))
	}

	return r.finish(result)
}

func (r Resolver) inspect(in intent.Intent) game.ActionResult {
	if strings.TrimSpace(in.Target) != "" {
		if r.targetIsCurrentRoom(in.Target) {
			in.Target = ""
			return r.inspect(in)
		}

		resolution, err := r.state.ResolveObjectGroup(in.Target, false)
		if err != nil {
			return failed("inspect_failed", err.Error())
		}
		if resolution.Ambiguous() {
			return objectAmbiguity(resolution)
		}
		if resolution.Missing() {
			if r.objectExistsButIsNotVisible(in.Target) {
				return failed("object_not_visible", "It is too dark for me to see that.")
			}
			return failed("object_not_found", "I cannot see that here.")
		}

		object := resolution.Matches[0]
		result := makeResult("inspected_object", 5, object.Description)
		return r.withObjectObservation(result, object, world.ObservationInspect)
	}

	room, err := r.state.CurrentRoom()
	if err != nil {
		return failed("inspect_failed", err.Error())
	}
	objects, err := r.state.VisibleObjects()
	if err != nil {
		return failed("inspect_failed", err.Error())
	}
	exits, err := r.state.AvailableExits()
	if err != nil {
		return failed("inspect_failed", err.Error())
	}

	facts := []game.Fact{typedFact(game.FactID(string(room.ID)+":description"), game.FactRoomDescription, string(room.ID), room.Description, room.Description)}
	if len(objects) == 0 {
		facts = append(facts, typedFact(game.FactID(string(room.ID)+":visible_objects"), game.FactVisibleObjects, string(room.ID), "none", "I cannot make out any distinct objects."))
	} else {
		facts = append(facts, typedFact(game.FactID(string(room.ID)+":visible_objects"), game.FactVisibleObjects, string(room.ID), joinObjectNames(objects), "I can see: "+joinObjectNames(objects)+"."))
	}
	if len(exits) > 0 {
		facts = append(facts, typedFact(game.FactID(string(room.ID)+":known_exits"), game.FactKnownExits, string(room.ID), joinExitDirections(exits), "I can go: "+joinExitDirections(exits)+"."))
	}
	r.state.RememberObjects(objectIDs(objects))

	return game.ActionResult{
		Status:          game.ActionSucceeded,
		DurationSeconds: 5,
		Outcome:         "inspected_room",
		VisibleFacts:    facts,
		Danger:          game.DangerNone,
	}
}

func (r Resolver) move(in intent.Intent) game.ActionResult {
	direction := strings.TrimSpace(in.Direction)
	if direction == "" {
		direction = strings.TrimSpace(in.Target)
	}
	if direction == "" {
		return clarification("Which way should I go?")
	}

	room, err := r.state.CurrentRoom()
	if err != nil {
		return failed("move_failed", err.Error())
	}

	knownExits, err := r.state.AvailableExits()
	if err != nil {
		return failed("move_failed", err.Error())
	}

	if isBackDirection(direction) && r.state.PreviousRoomID != "" {
		for _, exit := range knownExits {
			if exit.To == r.state.PreviousRoomID {
				return r.moveThroughExit(direction, exit)
			}
		}
	}

	for _, exit := range knownExits {
		if !world.MatchesTarget(direction, exit.Direction, nil) {
			continue
		}
		return r.moveThroughExit(direction, exit)
	}
	for _, exit := range room.Exits {
		if world.MatchesTarget(direction, exit.Direction, nil) {
			return failed("exit_unknown", "I cannot safely find that route in the dark.")
		}
	}

	return failed("exit_not_found", "I cannot find that way out.")
}

func (r Resolver) moveThroughExit(direction string, exit world.Exit) game.ActionResult {
	if exit.Door != "" {
		door, ok := r.state.Doors[exit.Door]
		if !ok {
			return failed("move_failed", "The door data is missing.")
		}
		if !door.IsPassable() {
			return failed("door_blocked", fmt.Sprintf("The %s is %s.", door.Name, door.State))
		}
	}

	from := r.state.CurrentRoomID
	r.state.CurrentRoomID = exit.To
	r.state.PreviousRoomID = from
	if err := r.state.ObserveRoom(exit.To, from); err != nil {
		return failed("move_failed", err.Error())
	}
	destination, err := r.state.CurrentRoom()
	if err != nil {
		return failed("move_failed", err.Error())
	}
	result := makeResult("moved", 20, "I move "+direction+" into "+destination.Name+".", destination.Description)
	result.Danger = roomEntryDanger(destination, r.state.ActiveLight)
	return result
}

func (r Resolver) search(in intent.Intent) game.ActionResult {
	if strings.TrimSpace(in.Target) == "" {
		return clarification("What should I search?")
	}

	resolution, err := r.state.ResolveObjectGroup(in.Target, false)
	if err != nil {
		return failed("search_failed", err.Error())
	}
	if resolution.Ambiguous() {
		return objectAmbiguity(resolution)
	}
	if resolution.Missing() {
		if r.objectExistsButIsNotVisible(in.Target) {
			return failed("object_not_visible", "It is too dark for me to search that.")
		}
		return failed("object_not_found", "I cannot see that here.")
	}

	object := resolution.Matches[0]
	if !object.Searchable {
		return r.withObjectObservation(failed("not_searchable", "I do not see a useful way to search that."), object, world.ObservationSearch)
	}
	if len(object.ContainedItems) == 0 {
		return r.withObjectObservation(makeResult("searched_empty", 30, "I search the "+object.Name+" but find nothing useful."), object, world.ObservationSearch)
	}

	facts := []string{"I search the " + object.Name + "."}
	foundItemIDs := make([]game.ItemID, 0, len(object.ContainedItems))
	for _, itemID := range object.ContainedItems {
		item, ok := r.state.Items[itemID]
		if !ok {
			continue
		}
		foundItemIDs = append(foundItemIDs, itemID)
		facts = append(facts, "I find "+item.Name+".")
	}
	r.state.DiscoverItems(foundItemIDs)
	r.state.RememberItems(foundItemIDs)

	return r.withObjectObservation(makeResult("searched_found_items", 35, facts...), object, world.ObservationSearch)
}

func (r Resolver) takeItem(in intent.Intent) game.ActionResult {
	itemTarget := strings.TrimSpace(in.Item)
	if itemTarget == "" {
		itemTarget = strings.TrimSpace(in.Target)
	}
	if isPronounTarget(itemTarget) {
		lastMentioned := r.state.LastMentionedItemIDs
		if len(lastMentioned) == 0 && r.state.LastMentionedItemID != "" {
			lastMentioned = []game.ItemID{r.state.LastMentionedItemID}
		}
		if len(lastMentioned) == 0 {
			return clarification("What should I pick up?")
		}
		if len(lastMentioned) > 1 {
			return clarification("Which item should I pick up: " + r.itemNames(lastMentioned) + "?")
		}
		item, ok := r.state.Items[lastMentioned[0]]
		if !ok {
			return failed("item_not_found", "I cannot find that item here.")
		}
		itemTarget = item.Name
	}
	if itemTarget == "" {
		return clarification("What should I pick up?")
	}

	room, err := r.state.CurrentRoom()
	if err != nil {
		return failed("take_failed", err.Error())
	}

	for _, objectID := range room.Objects {
		object, ok := r.state.Objects[objectID]
		if !ok || !r.state.CanSeeObject(room, object) {
			continue
		}
		for _, itemID := range object.ContainedItems {
			item, ok := r.state.Items[itemID]
			if !ok {
				continue
			}
			if !world.MatchesTarget(itemTarget, item.Name, item.Aliases) {
				continue
			}
			if r.state.HasItem(itemID) {
				return makeResult("item_already_taken", 1, "I already have "+item.Name+".")
			}
			if !r.state.IsItemDiscovered(itemID) {
				return failed("item_not_found", "I cannot find that item here.")
			}
			if !item.Portable {
				return failed("item_not_portable", "I cannot put that in my bag.")
			}

			r.state.AddInventory(itemID)
			object.ContainedItems = removeItemID(object.ContainedItems, itemID)
			r.state.Objects[objectID] = object
			r.state.RememberItems([]game.ItemID{itemID})
			return makeResult("item_taken", 5, "I pick up "+item.Name+".")
		}
	}

	return failed("item_not_found", "I cannot find that item here.")
}

func (r Resolver) useItem(in intent.Intent) game.ActionResult {
	if in.Item == "" && world.MatchesTarget(in.Target, "key", []string{"brass key", "small key"}) {
		in.Item = in.Target
		in.Target = ""
	}
	if in.Item == "" && isPronounTarget(in.Target) {
		if inferred, ok := r.inferUsableInventoryItem(); ok {
			in.Item = inferred
			in.Target = ""
		}
	}

	if world.MatchesTarget(in.Item, "flashlight", []string{"torch", "light"}) ||
		world.MatchesTarget(in.Target, "flashlight", []string{"torch", "light", "your flashlight"}) {
		if _, ok := r.findInventoryItem("flashlight"); !ok {
			return failed("missing_item", "I do not have the flashlight.")
		}
		r.state.ActiveLight = true
		if err := r.state.ObserveRoom(r.state.CurrentRoomID, ""); err != nil {
			return failed("use_failed", err.Error())
		}
		return makeResult("flashlight_on", 3, "I turn on the flashlight.")
	}
	if !world.MatchesTarget(in.Item, "key", []string{"brass key", "small key"}) {
		return failed("unsupported_item", "I do not know how to use that yet.")
	}

	keyID, ok := r.findInventoryItem(in.Item)
	if !ok {
		return failed("missing_item", "I do not have that item.")
	}

	doorTarget := strings.TrimSpace(in.Target)
	if doorTarget == "" {
		door, ok := r.inferKeyDoor(keyID)
		if !ok {
			return clarification("What should I use the key on?")
		}
		return r.unlockDoor(door, keyID)
	}

	resolution, err := r.state.ResolveDoor(doorTarget)
	if err != nil {
		return failed("use_failed", err.Error())
	}
	if resolution.Ambiguous() {
		return doorAmbiguity(resolution)
	}
	if resolution.Missing() {
		return failed("door_not_found", "I cannot see that door here.")
	}

	return r.unlockDoor(resolution.Matches[0], keyID)
}

func (r Resolver) unlockDoor(door world.Door, keyID game.ItemID) game.ActionResult {
	if !door.CanUnlockWith(keyID) {
		return failed("key_does_not_fit", "The key does not fit the lock.")
	}

	door.State = world.DoorClosed
	r.state.Doors[door.ID] = door
	return makeResult("door_unlocked", 8, "The key turns in the lock. The "+door.Name+" is unlocked.")
}

func (r Resolver) turnOn(in intent.Intent) game.ActionResult {
	if in.Item == "" && isPronounTarget(in.Target) {
		if inferred, ok := r.inferUsableInventoryItem(); ok {
			in.Item = inferred
			in.Target = ""
		}
	}

	if world.MatchesTarget(in.Item, "flashlight", []string{"torch", "light"}) ||
		world.MatchesTarget(in.Target, "flashlight", []string{"torch", "light", "your flashlight"}) {
		if _, ok := r.findInventoryItem("flashlight"); !ok {
			return failed("missing_item", "I do not have the flashlight.")
		}
		r.state.ActiveLight = true
		if err := r.state.ObserveRoom(r.state.CurrentRoomID, ""); err != nil {
			return failed("turn_on_failed", err.Error())
		}
		return makeResult("flashlight_on", 3, "I turn on the flashlight.")
	}
	return failed("cannot_turn_on", "I cannot turn that on.")
}

func (r Resolver) explore() game.ActionResult {
	exit, found, err := r.state.DiscoverNextUnknownExit()
	if err != nil {
		return failed("explore_failed", err.Error())
	}
	if !found {
		return makeResult("no_unknown_exits", 30, "I feel along the walls but cannot find another exit.")
	}
	return makeResult("exit_discovered", 30, "I feel along the wall and find a way "+exit.Direction+".")
}

func (r Resolver) turnOff(in intent.Intent) game.ActionResult {
	if in.Item == "" && isPronounTarget(in.Target) {
		if inferred, ok := r.inferUsableInventoryItem(); ok {
			in.Item = inferred
			in.Target = ""
		}
	}

	if world.MatchesTarget(in.Item, "flashlight", []string{"torch", "light"}) ||
		world.MatchesTarget(in.Target, "flashlight", []string{"torch", "light", "your flashlight"}) {
		if _, ok := r.findInventoryItem("flashlight"); !ok {
			return failed("missing_item", "I do not have the flashlight.")
		}
		r.state.ActiveLight = false
		return makeResult("flashlight_off", 2, "I turn off the flashlight.")
	}
	return failed("cannot_turn_off", "I cannot turn that off.")
}

func (r Resolver) talk(in intent.Intent) game.ActionResult {
	if isItemLocationQuestion(in) {
		return r.answerItemLocation(in)
	}

	if isInventoryQuestion(in) {
		target := strings.TrimSpace(in.Item)
		if target == "" {
			target = strings.TrimSpace(in.Target)
		}
		if isInventoryListTarget(target) {
			target = ""
		}
		if target != "" {
			if _, ok := r.findInventoryItem(target); ok {
				return makeResult("inventory_has_item", 0, "Yes. I have "+r.displayItemName(target)+".")
			}
			return makeResult("inventory_missing_item", 0, "No. I do not have "+target+".")
		}

		names := r.inventoryNames()
		if len(names) == 0 {
			return makeResult("inventory_empty", 0, "I do not have anything useful on me.")
		}
		return makeResult("inventory_listed", 0, "I have: "+strings.Join(names, ", ")+".")
	}

	if isItemPresenceQuestion(in) {
		return r.answerItemLocation(in)
	}

	return makeResult("talked", 0, "I hear you.")
}

func (r Resolver) answerItemLocation(in intent.Intent) game.ActionResult {
	target := strings.TrimSpace(in.Item)
	if target == "" {
		target = strings.TrimSpace(in.Target)
	}
	if target == "" || isInventoryListTarget(target) {
		return makeResult("talked", 0, "I hear you.")
	}

	itemID, item, ok := r.findKnownItem(target)
	if !ok {
		return makeResult("item_location_unknown", 0, "I do not know what "+target+" is.")
	}
	if r.state.HasItem(itemID) {
		return makeResult("item_location_inventory", 0, "I have "+item.Name+".")
	}
	if !r.state.IsItemDiscovered(itemID) {
		return makeResult("item_location_unknown", 0, "I have not found "+item.Name+" yet.")
	}
	if container, ok := r.findItemContainer(itemID); ok {
		return makeResult("item_location_known", 0, "I found "+item.Name+" in "+container.Name+".")
	}
	return makeResult("item_location_known", 0, "I found "+item.Name+" nearby.")
}

func (r Resolver) autonomyResult(in intent.Intent) (game.ActionResult, bool) {
	danger := r.intentDanger(in)
	decision := r.state.Kaya.CanAttempt(danger)
	if !decision.Allowed {
		return game.ActionResult{
			Status:       game.ActionRefused,
			Outcome:      "kaya_refused",
			VisibleFacts: []game.Fact{typedFact("kaya:refusal", game.FactEmotion, "kaya", "refused", decision.Reason)},
			Danger:       game.DangerNone,
		}, true
	}
	if decision.NeedsConfirmation {
		return game.ActionResult{
			Status:                game.ActionClarification,
			Outcome:               "kaya_needs_confirmation",
			VisibleFacts:          []game.Fact{typedFact("kaya:confirmation", game.FactEmotion, "kaya", "needs_confirmation", decision.Reason)},
			NeedsClarification:    true,
			ClarificationQuestion: decision.Reason,
			Danger:                game.DangerNone,
		}, true
	}
	return game.ActionResult{}, false
}

func (r Resolver) intentDanger(in intent.Intent) game.DangerLevel {
	switch in.Action {
	case intent.ActionMove:
		return r.moveDanger(in)
	case intent.ActionSearch:
		if isBodySearchTarget(in.Target) {
			return game.DangerModerate
		}
	case intent.ActionTurnOff:
		room, err := r.state.CurrentRoom()
		if err == nil && room.NeedsLight() {
			return game.DangerHigh
		}
	}
	return game.DangerNone
}

func (r Resolver) moveDanger(in intent.Intent) game.DangerLevel {
	direction := strings.TrimSpace(in.Direction)
	if direction == "" {
		direction = strings.TrimSpace(in.Target)
	}
	if direction == "" {
		return game.DangerNone
	}

	exits, err := r.state.AvailableExits()
	if err != nil {
		return game.DangerNone
	}

	if isBackDirection(direction) && r.state.PreviousRoomID != "" {
		for _, exit := range exits {
			if exit.To == r.state.PreviousRoomID {
				return r.exitDanger(exit)
			}
		}
	}

	for _, exit := range exits {
		if world.MatchesTarget(direction, exit.Direction, nil) {
			return r.exitDanger(exit)
		}
	}

	return game.DangerNone
}

func (r Resolver) exitDanger(exit world.Exit) game.DangerLevel {
	if exit.Door != "" {
		door, ok := r.state.Doors[exit.Door]
		if !ok || !door.IsPassable() {
			return game.DangerNone
		}
	}

	destination, ok := r.state.Rooms[exit.To]
	if !ok {
		return game.DangerNone
	}
	return roomEntryDanger(destination, r.state.ActiveLight)
}

func roomEntryDanger(room world.Room, hasLight bool) game.DangerLevel {
	if hasLight {
		return game.DangerNone
	}
	if room.Visibility == world.VisibilityPitchBlack {
		return game.DangerHigh
	}
	if room.NeedsLight() {
		return game.DangerModerate
	}
	return game.DangerNone
}

func isBodySearchTarget(target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	return strings.Contains(target, "doctor") ||
		strings.Contains(target, "body") ||
		strings.Contains(target, "corpse")
}

func (r Resolver) inferKeyDoor(keyID game.ItemID) (world.Door, bool) {
	exits, err := r.state.AvailableExits()
	if err != nil {
		return world.Door{}, false
	}

	var matches []world.Door
	for _, exit := range exits {
		if exit.Door == "" {
			continue
		}
		door, ok := r.state.Doors[exit.Door]
		if !ok || !door.CanUnlockWith(keyID) {
			continue
		}
		matches = append(matches, door)
	}

	if len(matches) != 1 {
		return world.Door{}, false
	}
	return matches[0], true
}

func (r Resolver) inferUsableInventoryItem() (string, bool) {
	var matches []string
	for itemID := range r.state.Inventory {
		item, ok := r.state.Items[itemID]
		if !ok {
			continue
		}
		if world.MatchesTarget(string(itemID), "flashlight", []string{"torch", "light"}) ||
			world.MatchesTarget(item.Name, "flashlight", []string{"torch", "light"}) {
			matches = append(matches, "flashlight")
		}
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

func (r Resolver) objectExistsButIsNotVisible(target string) bool {
	room, err := r.state.CurrentRoom()
	if err != nil {
		return false
	}

	for _, objectID := range room.Objects {
		object, ok := r.state.Objects[objectID]
		if !ok || r.state.CanSeeObject(room, object) {
			continue
		}
		if world.MatchesTarget(target, object.Name, object.Aliases) {
			return true
		}
	}
	return false
}

func (r Resolver) targetIsCurrentRoom(target string) bool {
	room, err := r.state.CurrentRoom()
	if err != nil {
		return false
	}
	return world.MatchesTarget(target, room.Name, []string{string(room.ID), "room", "current room", "here", "around you"})
}

func (r Resolver) findInventoryItem(target string) (game.ItemID, bool) {
	for itemID := range r.state.Inventory {
		item, ok := r.state.Items[itemID]
		if !ok {
			continue
		}
		if world.MatchesTarget(target, item.Name, item.Aliases) {
			return itemID, true
		}
	}
	return "", false
}

func (r Resolver) findKnownItem(target string) (game.ItemID, world.Item, bool) {
	for itemID, item := range r.state.Items {
		if world.MatchesTarget(target, item.Name, item.Aliases) {
			return itemID, item, true
		}
	}
	return "", world.Item{}, false
}

func (r Resolver) findItemContainer(itemID game.ItemID) (world.Object, bool) {
	for _, object := range r.state.Objects {
		for _, containedID := range object.ContainedItems {
			if containedID == itemID {
				return object, true
			}
		}
	}
	return world.Object{}, false
}

func (r Resolver) itemNames(itemIDs []game.ItemID) string {
	names := make([]string, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		item, ok := r.state.Items[itemID]
		if !ok {
			continue
		}
		names = append(names, item.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (r Resolver) inventoryNames() []string {
	names := make([]string, 0, len(r.state.Inventory))
	for itemID := range r.state.Inventory {
		item, ok := r.state.Items[itemID]
		if !ok {
			continue
		}
		names = append(names, item.Name)
	}
	sort.Strings(names)
	return names
}

func (r Resolver) displayItemName(target string) string {
	for itemID := range r.state.Inventory {
		item, ok := r.state.Items[itemID]
		if !ok {
			continue
		}
		if world.MatchesTarget(target, item.Name, item.Aliases) {
			return item.Name
		}
	}
	return target
}

func isItemLocationQuestion(in intent.Intent) bool {
	raw := strings.ToLower(strings.TrimSpace(in.RawText))
	if strings.TrimSpace(in.Item) == "" && strings.TrimSpace(in.Target) == "" {
		return false
	}
	return strings.Contains(raw, "where is") ||
		strings.Contains(raw, "where's") ||
		strings.Contains(raw, "where did") ||
		strings.Contains(raw, "where was") ||
		strings.Contains(raw, "where can") ||
		strings.Contains(raw, "where would")
}

func isItemPresenceQuestion(in intent.Intent) bool {
	raw := strings.ToLower(strings.TrimSpace(in.RawText))
	if strings.TrimSpace(in.Item) == "" && strings.TrimSpace(in.Target) == "" {
		return false
	}
	return strings.Contains(raw, "is there") ||
		strings.Contains(raw, "are there") ||
		strings.Contains(raw, "do you see") ||
		strings.Contains(raw, "can you see") ||
		strings.Contains(raw, "have you found") ||
		strings.Contains(raw, "did you find")
}

func isInventoryQuestion(in intent.Intent) bool {
	raw := strings.ToLower(strings.TrimSpace(in.RawText))
	target := strings.ToLower(strings.TrimSpace(in.Target))
	item := strings.ToLower(strings.TrimSpace(in.Item))
	if strings.Contains(raw, "in mind") {
		return false
	}
	return target == "inventory" ||
		item == "inventory" ||
		target == "items" ||
		item == "items" ||
		strings.Contains(raw, "do you have") ||
		strings.Contains(raw, "do ypou have") ||
		strings.Contains(raw, "are you carrying") ||
		strings.Contains(raw, "what do you have") ||
		strings.Contains(raw, "what are you carrying") ||
		strings.Contains(raw, "inventory")
}

func isInventoryListTarget(target string) bool {
	return world.MatchesTarget(target, "inventory", []string{"items", "anything", "what do you have", "what are you carrying"})
}

func isBackDirection(direction string) bool {
	return world.MatchesTarget(direction, "back", []string{"backward", "previous room", "where you came from"})
}

func isPronounTarget(target string) bool {
	return world.MatchesTarget(target, "it", []string{"that", "that thing", "the thing"})
}

func (r Resolver) finish(result game.ActionResult) game.ActionResult {
	if r.state == nil || result.NeedsClarification {
		return result
	}

	result.StartedAtSeconds = r.state.NowSeconds
	if result.DurationSeconds > 0 {
		events := r.state.Advance(result.DurationSeconds)
		result.Events = append(result.Events, events...)
	}
	r.state.Kaya = r.state.Kaya.Apply(result)
	return result
}

func makeResult(outcome string, duration int, facts ...string) game.ActionResult {
	visibleFacts := make([]game.Fact, 0, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact) == "" {
			continue
		}
		visibleFacts = append(visibleFacts, typedFact(game.FactID(fmt.Sprintf("%s:%d", outcome, len(visibleFacts))), game.FactAction, "action", outcome, fact))
	}

	return game.ActionResult{
		Status:          game.ActionSucceeded,
		DurationSeconds: duration,
		Outcome:         outcome,
		VisibleFacts:    visibleFacts,
		Danger:          game.DangerNone,
	}
}

func failed(outcome string, fact string) game.ActionResult {
	return game.ActionResult{
		Status:          game.ActionFailed,
		DurationSeconds: 2,
		Outcome:         outcome,
		VisibleFacts:    []game.Fact{typedFact(game.FactID(outcome), game.FactFailure, "action", outcome, fact)},
		Danger:          game.DangerNone,
	}
}

func clarification(question string) game.ActionResult {
	if strings.TrimSpace(question) == "" {
		question = "What should I do?"
	}
	return game.ActionResult{
		Status:                game.ActionClarification,
		Outcome:               "needs_clarification",
		VisibleFacts:          []game.Fact{typedFact("needs_clarification", game.FactClarification, "action", "needs_clarification", question)},
		NeedsClarification:    true,
		ClarificationQuestion: question,
		Danger:                game.DangerNone,
	}
}

func confirmationWithOutcome(outcome string, question string) game.ActionResult {
	result := clarification(question)
	result.Outcome = outcome
	return result
}

func objectAmbiguity(resolution world.ObjectResolution) game.ActionResult {
	names := make([]string, 0, len(resolution.Matches))
	for _, object := range resolution.Matches {
		names = append(names, object.Name)
	}
	question := "Which one do you mean: " + strings.Join(names, ", ") + "?"
	return clarification(question)
}

func doorAmbiguity(resolution world.DoorResolution) game.ActionResult {
	names := make([]string, 0, len(resolution.Matches))
	for _, door := range resolution.Matches {
		names = append(names, door.Name)
	}
	question := "Which door do you mean: " + strings.Join(names, ", ") + "?"
	return clarification(question)
}

func joinObjectNames(objects []world.Object) string {
	names := make([]string, 0, len(objects))
	for _, object := range objects {
		names = append(names, object.Name)
	}
	return strings.Join(names, ", ")
}

func joinExitDirections(exits []world.Exit) string {
	directions := make([]string, 0, len(exits))
	for _, exit := range exits {
		directions = append(directions, exit.Direction)
	}
	return strings.Join(directions, ", ")
}

func (r Resolver) withObjectObservation(result game.ActionResult, object world.Object, method world.ObservationMethod) game.ActionResult {
	result.TargetObjectIDs = []game.ObjectID{object.ID}
	r.state.RememberObjects(result.TargetObjectIDs)
	result.VisibleFacts = append(result.VisibleFacts, r.state.ObserveObject(object.ID, method)...)
	return result
}

func typedFact(id game.FactID, kind game.FactKind, subject string, value string, text string) game.Fact {
	return game.Fact{ID: id, Kind: kind, Subject: subject, Value: value, Text: text, Required: true}
}

func objectIDs(objects []world.Object) []game.ObjectID {
	ids := make([]game.ObjectID, 0, len(objects))
	for _, object := range objects {
		ids = append(ids, object.ID)
	}
	return ids
}

func removeItemID(itemIDs []game.ItemID, removed game.ItemID) []game.ItemID {
	filtered := itemIDs[:0]
	for _, itemID := range itemIDs {
		if itemID != removed {
			filtered = append(filtered, itemID)
		}
	}
	return filtered
}
