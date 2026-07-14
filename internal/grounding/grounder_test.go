package grounding

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/world"
)

const (
	testRoom game.RoomID = "quartz_atrium"
	roomTwo  game.RoomID = "linen_gallery"

	objectRelay      game.ObjectID = "relay"
	objectRelayShell game.ObjectID = "relay_shell"
	objectVeiled     game.ObjectID = "veiled"
	objectCradle     game.ObjectID = "cradle"

	itemPrism   game.ItemID = "prism"
	itemGlyph   game.ItemID = "glyph"
	itemHidden  game.ItemID = "hidden"
	itemCarried game.ItemID = "carried"

	doorNorth game.DoorID = "north_hatch"
	doorSouth game.DoorID = "south_hatch"
)

func TestGroundRanksExactNamesAndAliases(t *testing.T) {
	state := syntheticWorld()
	grounder := New(state)

	t.Run("exact object name outranks token match", func(t *testing.T) {
		got := grounder.Ground(intent.SearchAction{Target: reference("Copper Relay", intent.TargetOne)}, nil)
		assertReadyIDs(t, got, RoleObject, string(objectRelay))
	})

	t.Run("exact item alias", func(t *testing.T) {
		got := grounder.Ground(intent.TakeAction{Target: reference("star shard", intent.TargetOne)}, nil)
		assertReadyIDs(t, got, RoleItem, string(itemPrism))
	})

	t.Run("exact exit direction", func(t *testing.T) {
		got := grounder.Ground(intent.MoveAction{Direction: "northward"}, nil)
		assertReadyIDs(t, got, RoleExit, "northward")
	})
}

func TestGroundPreservesEqualScoreAmbiguity(t *testing.T) {
	state := syntheticWorld()
	state.KnownExitDirections[testRoom]["southward"] = true
	got := New(state).Ground(intent.UseAction{
		Item:   reference("phase spindle", intent.TargetOne),
		Target: reference("iris hatch", intent.TargetOne),
	}, nil)

	if got.Clarification == nil {
		t.Fatalf("Ground() clarification = nil, want door ambiguity: %#v", got)
	}
	if got.Clarification.Role != RoleDoor {
		t.Fatalf("clarification role = %q, want %q", got.Clarification.Role, RoleDoor)
	}
	assertCandidateIDs(t, got.Clarification.Candidates, string(doorNorth), string(doorSouth))
	assertReferenceIDs(t, got, RoleItem, string(itemCarried))
}

func TestGroundResolvesRecentPronounsAndPluralReferences(t *testing.T) {
	state := syntheticWorld()
	state.RecentReferents = []game.ReferentGroup{
		{ObjectIDs: []game.ObjectID{objectRelay}},
		{ObjectIDs: []game.ObjectID{objectRelay, objectRelayShell}},
	}

	singular := New(state).Ground(intent.SearchAction{Target: reference("it", intent.TargetOne)}, nil)
	assertReadyIDs(t, singular, RoleObject, string(objectRelay))

	plural := New(state).Ground(intent.SearchAction{Target: reference("them", intent.TargetAll)}, nil)
	assertReadyIDs(t, plural, RoleObject, string(objectRelay), string(objectRelayShell))
}

func TestGroundDemonstrativeDoorUsesUniqueEligibleCandidate(t *testing.T) {
	state := syntheticWorld()
	got := New(state).Ground(intent.UseAction{
		Item:   reference("phase spindle", intent.TargetOne),
		Target: reference("that door", intent.TargetOne),
	}, nil)

	assertReadyIDs(t, got, RoleDoor, string(doorNorth))
}

func TestGroundDemonstrativeDoorPreservesEligibleAmbiguity(t *testing.T) {
	state := syntheticWorld()
	state.KnownExitDirections[testRoom]["southward"] = true
	got := New(state).Ground(intent.UseAction{
		Item:   reference("phase spindle", intent.TargetOne),
		Target: reference("that door", intent.TargetOne),
	}, nil)

	if got.Clarification == nil || got.Clarification.Role != RoleDoor {
		t.Fatalf("Ground() clarification = %#v, want door ambiguity", got.Clarification)
	}
	assertCandidateIDs(t, got.Clarification.Candidates, string(doorNorth), string(doorSouth))
}

func TestGroundDemonstrativeDoorHonorsExplicitBindingBeforeFallback(t *testing.T) {
	state := syntheticWorld()
	state.KnownExitDirections[testRoom]["southward"] = true
	got := New(state).Ground(intent.UseAction{
		Item:   reference("phase spindle", intent.TargetOne),
		Target: reference("that door", intent.TargetOne),
	}, &Binding{Role: RoleDoor, CandidateIDs: []string{string(doorSouth)}})

	assertReadyIDs(t, got, RoleDoor, string(doorSouth))
}

func TestGroundDemonstrativeExitUsesUniqueEligibleCandidate(t *testing.T) {
	state := syntheticWorld()
	got := New(state).Ground(intent.MoveAction{Direction: "that exit"}, nil)

	assertReadyIDs(t, got, RoleExit, "northward")
}

func TestGroundDemonstrativeExitPreservesEligibleAmbiguity(t *testing.T) {
	state := syntheticWorld()
	state.KnownExitDirections[testRoom]["southward"] = true
	got := New(state).Ground(intent.MoveAction{Direction: "that exit"}, nil)

	if got.Clarification == nil || got.Clarification.Role != RoleExit {
		t.Fatalf("Ground() clarification = %#v, want exit ambiguity", got.Clarification)
	}
	assertCandidateIDs(t, got.Clarification.Candidates, "northward", "southward")
}

func TestGroundNonDemonstrativeExactMatchStillOutranksRecentReferent(t *testing.T) {
	state := syntheticWorld()
	state.RecentReferents = []game.ReferentGroup{{ObjectIDs: []game.ObjectID{objectRelayShell}}}

	for _, mention := range []string{"Copper Relay", "signal crown"} {
		got := New(state).Ground(intent.SearchAction{Target: reference(mention, intent.TargetOne)}, nil)
		assertReadyIDs(t, got, RoleObject, string(objectRelay))
	}
}

func TestGroundListenTargetsDoorAndExploreHasNoEntityTarget(t *testing.T) {
	state := syntheticWorld()

	listen := New(state).Ground(intent.ListenAction{Target: reference("polar iris", intent.TargetOne)}, nil)
	assertReadyIDs(t, listen, RoleDoor, string(doorNorth))

	untargetedListen := New(state).Ground(intent.ListenAction{}, nil)
	if !untargetedListen.Ready() || len(untargetedListen.References) != 0 {
		t.Fatalf("untargeted listen = %#v, want ready without references", untargetedListen)
	}

	explore := New(state).Ground(intent.ExploreAction{Target: reference("wall tracery", intent.TargetOne)}, nil)
	if !explore.Ready() || len(explore.References) != 0 {
		t.Fatalf("explore = %#v, want ready without entity references", explore)
	}
}

func TestGroundUsesCandidateBoundIDs(t *testing.T) {
	state := syntheticWorld()
	action := intent.SearchAction{Target: reference("relay", intent.TargetOne)}
	binding := &Binding{Role: RoleObject, CandidateIDs: []string{string(objectRelayShell)}}

	got := New(state).Ground(action, binding)
	assertReadyIDs(t, got, RoleObject, string(objectRelayShell))

	missing := New(state).Ground(action, &Binding{Role: RoleObject, CandidateIDs: []string{"not_permitted"}})
	if missing.Missing == nil || missing.Missing.Role != RoleObject {
		t.Fatalf("Ground() missing = %#v, want bound object failure", missing.Missing)
	}
}

func TestGroundTargetAllBindingRejectsPartiallyStaleObjectSelection(t *testing.T) {
	state := syntheticWorld()
	room := state.Rooms[testRoom]
	room.Objects = []game.ObjectID{objectRelay, objectVeiled, objectCradle}
	state.Rooms[testRoom] = room
	boundIDs := []string{string(objectRelay), string(objectRelayShell)}

	got := New(state).Ground(
		intent.SearchAction{Target: reference("relay", intent.TargetAll)},
		&Binding{Role: RoleObject, CandidateIDs: boundIDs},
	)

	assertStaleBinding(t, got, RoleObject, intent.TargetAll, boundIDs, string(objectRelayShell))
	if len(got.References) != 0 {
		t.Fatalf("Ground() references = %#v, want no narrowed object selection", got.References)
	}
}

func TestGroundTargetAllBindingRejectsPartiallyStaleDoorSelection(t *testing.T) {
	state := syntheticWorld()
	boundIDs := []string{string(doorNorth), string(doorSouth)}

	got := New(state).Ground(intent.UseAction{
		Item:   reference("phase spindle", intent.TargetOne),
		Target: reference("iris hatch", intent.TargetAll),
	}, &Binding{Role: RoleDoor, CandidateIDs: boundIDs})

	assertStaleBinding(t, got, RoleDoor, intent.TargetAll, boundIDs, string(doorSouth))
	assertReferenceIDs(t, got, RoleItem, string(itemCarried))
}

func TestGroundRejectsBindingForUnexpectedActionRole(t *testing.T) {
	state := syntheticWorld()
	boundIDs := []string{string(doorNorth)}
	got := New(state).Ground(
		intent.SearchAction{Target: reference("Copper Relay", intent.TargetOne)},
		&Binding{Role: RoleDoor, CandidateIDs: boundIDs},
	)

	if got.Missing == nil || got.Missing.Reason != MissingReasonBindingRole {
		t.Fatalf("Ground() missing = %#v, want binding-role failure", got.Missing)
	}
	if !reflect.DeepEqual(got.Missing.BoundCandidateIDs, boundIDs) {
		t.Fatalf("bound IDs = %v, want %v", got.Missing.BoundCandidateIDs, boundIDs)
	}
	if !reflect.DeepEqual(got.Missing.ExpectedRoles, []Role{RoleObject}) {
		t.Fatalf("expected roles = %v, want [%s]", got.Missing.ExpectedRoles, RoleObject)
	}
	if len(got.References) != 0 {
		t.Fatalf("Ground() references = %#v, want no ordinary mention fallback", got.References)
	}
}

func TestGroundOnlyUsesEligibleEntities(t *testing.T) {
	state := syntheticWorld()
	grounder := New(state)

	tests := []struct {
		name   string
		action intent.SemanticAction
		role   Role
		wantID string
	}{
		{
			name:   "visible object",
			action: intent.SearchAction{Target: reference("signal crown", intent.TargetOne)},
			role:   RoleObject,
			wantID: string(objectRelay),
		},
		{
			name:   "discovered item in visible object",
			action: intent.TakeAction{Target: reference("star shard", intent.TargetOne)},
			role:   RoleItem,
			wantID: string(itemPrism),
		},
		{
			name:   "inventory item",
			action: intent.ToggleAction{Item: reference("phase spindle", intent.TargetOne), State: "on"},
			role:   RoleItem,
			wantID: string(itemCarried),
		},
		{
			name:   "door on known exit",
			action: intent.UseAction{Item: reference("phase spindle", intent.TargetOne), Target: reference("polar iris", intent.TargetOne)},
			role:   RoleDoor,
			wantID: string(doorNorth),
		},
		{
			name:   "known exit",
			action: intent.MoveAction{Direction: "northward"},
			role:   RoleExit,
			wantID: "northward",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := grounder.Ground(test.action, nil)
			assertReadyIDs(t, got, test.role, test.wantID)
		})
	}

	ineligible := []struct {
		name   string
		action intent.SemanticAction
		role   Role
	}{
		{"object hidden without light", intent.SearchAction{Target: reference("velvet cipher", intent.TargetOne)}, RoleObject},
		{"undiscovered item", intent.TakeAction{Target: reference("silent glyph", intent.TargetOne)}, RoleItem},
		{"discovered item is not inventory", intent.ToggleAction{Item: reference("star shard", intent.TargetOne), State: "on"}, RoleItem},
		{"unknown door", intent.UseAction{Item: reference("phase spindle", intent.TargetOne), Target: reference("southern iris", intent.TargetOne)}, RoleDoor},
		{"unknown exit", intent.MoveAction{Direction: "southward"}, RoleExit},
	}

	for _, test := range ineligible {
		t.Run(test.name, func(t *testing.T) {
			got := grounder.Ground(test.action, nil)
			if got.Missing == nil || got.Missing.Role != test.role {
				t.Fatalf("Ground() missing = %#v, want role %q; result=%#v", got.Missing, test.role, got)
			}
		})
	}
}

func TestGroundDoesNotMutateWorld(t *testing.T) {
	state := syntheticWorld()
	before := cloneSyntheticState(state)

	_ = New(state).Ground(intent.UseAction{
		Item:   reference("phase spindle", intent.TargetOne),
		Target: reference("iris hatch", intent.TargetOne),
	}, nil)

	if !reflect.DeepEqual(state, before) {
		t.Fatalf("Ground() mutated world\n got: %#v\nwant: %#v", state, before)
	}
}

func TestGroundIsInvariantUnderCandidateOrder(t *testing.T) {
	first := syntheticWorld()
	second := syntheticWorld()
	first.KnownExitDirections[testRoom]["southward"] = true
	second.KnownExitDirections[testRoom]["southward"] = true
	second.Rooms[testRoom] = reverseRoomOrder(second.Rooms[testRoom])
	second.Objects = reverseObjectMap(second.Objects)
	second.Doors = reverseDoorMap(second.Doors)
	second.Items = reverseItemMap(second.Items)

	action := intent.UseAction{
		Item:   reference("phase spindle", intent.TargetOne),
		Target: reference("iris hatch", intent.TargetOne),
	}
	left := New(first).Ground(action, nil)
	right := New(second).Ground(action, nil)

	if !reflect.DeepEqual(left, right) {
		t.Fatalf("Ground() changed with candidate order\nleft:  %#v\nright: %#v", left, right)
	}
}

func TestGroundAllReturnsEveryTopScoringTie(t *testing.T) {
	state := syntheticWorld()
	got := New(state).Ground(intent.SearchAction{Target: reference("relay", intent.TargetAll)}, nil)
	assertReadyIDs(t, got, RoleObject, string(objectRelay), string(objectRelayShell))
}

func TestGroundRejectsNilTypedAction(t *testing.T) {
	var action *intent.SearchAction
	got := New(syntheticWorld()).Ground(action, nil)
	if !errors.Is(got.Err, ErrUnsupportedAction) {
		t.Fatalf("Ground() error = %v, want %v", got.Err, ErrUnsupportedAction)
	}
}

func FuzzGroundOrderAndAliasesInvariant(f *testing.F) {
	f.Add("lattice", byte(0))
	f.Add("woven signal", byte(7))

	f.Fuzz(func(t *testing.T, alias string, order byte) {
		alias = normalizedFuzzAlias(alias)
		if alias == "" {
			t.Skip()
		}

		first := syntheticWorld()
		second := syntheticWorld()
		left := first.Objects[objectRelay]
		right := second.Objects[objectRelay]
		left.Aliases = append(left.Aliases, alias)
		right.Aliases = append([]string{alias}, right.Aliases...)
		first.Objects[objectRelay] = left
		second.Objects[objectRelay] = right
		if order&1 != 0 {
			room := second.Rooms[testRoom]
			room.Objects[0], room.Objects[1] = room.Objects[1], room.Objects[0]
			second.Rooms[testRoom] = room
		}
		if order&2 != 0 {
			second.Objects = reverseObjectMap(second.Objects)
		}

		action := intent.SearchAction{Target: reference(alias, intent.TargetOne)}
		gotFirst := New(first).Ground(action, nil)
		gotSecond := New(second).Ground(action, nil)
		if !reflect.DeepEqual(gotFirst, gotSecond) {
			t.Fatalf("alias %q order %d changed result\nfirst:  %#v\nsecond: %#v", alias, order, gotFirst, gotSecond)
		}
	})
}

func syntheticWorld() *world.State {
	state := world.NewState(testRoom)
	state.Rooms[testRoom] = world.Room{
		ID:         testRoom,
		Name:       "Quartz Atrium",
		Visibility: world.VisibilityDark,
		Objects:    []game.ObjectID{objectRelay, objectRelayShell, objectVeiled, objectCradle},
		Exits: []world.Exit{
			{Direction: "northward", To: roomTwo, Door: doorNorth},
			{Direction: "southward", To: roomTwo, Door: doorSouth},
		},
	}
	state.Rooms[roomTwo] = world.Room{ID: roomTwo, Name: "Linen Gallery"}
	state.Objects[objectRelay] = world.Object{
		ID: objectRelay, Name: "Copper Relay", Aliases: []string{"relay", "signal crown"}, Searchable: true,
	}
	state.Objects[objectRelayShell] = world.Object{
		ID: objectRelayShell, Name: "Copper Relay Housing", Aliases: []string{"relay", "outer crown"}, Searchable: true,
	}
	state.Objects[objectVeiled] = world.Object{
		ID: objectVeiled, Name: "Velvet Cipher", Aliases: []string{"veiled code"}, RequiresLight: true, Searchable: true,
	}
	state.Objects[objectCradle] = world.Object{
		ID: objectCradle, Name: "Ivory Cradle", Aliases: []string{"cradle"}, Searchable: true,
		ContainedItems: []game.ItemID{itemPrism, itemGlyph, itemHidden},
	}
	state.Items[itemPrism] = world.Item{ID: itemPrism, Name: "Prismatic Fragment", Aliases: []string{"star shard"}, Portable: true}
	state.Items[itemGlyph] = world.Item{ID: itemGlyph, Name: "Noisy Glyph", Aliases: []string{"loud glyph"}, Portable: true}
	state.Items[itemHidden] = world.Item{ID: itemHidden, Name: "Silent Glyph", Aliases: []string{"mute glyph"}, Portable: true}
	state.Items[itemCarried] = world.Item{ID: itemCarried, Name: "Phase Spindle", Aliases: []string{"phase key"}, Portable: true}
	state.Inventory[itemCarried] = true
	state.DiscoveredItems[itemPrism] = true
	state.DiscoveredItems[itemGlyph] = true
	state.DiscoveredItems[itemCarried] = true
	state.KnownExitDirections[testRoom] = map[string]bool{"northward": true, "southward": false}
	state.Doors[doorNorth] = world.Door{
		ID: doorNorth, Name: "Polar Iris", Aliases: []string{"iris hatch", "northern iris"}, From: testRoom, To: roomTwo, State: world.DoorLocked,
	}
	state.Doors[doorSouth] = world.Door{
		ID: doorSouth, Name: "Southern Iris", Aliases: []string{"iris hatch", "southern iris"}, From: testRoom, To: roomTwo, State: world.DoorLocked,
	}
	return state
}

func reference(mention string, quantity intent.TargetMode) intent.Reference {
	return intent.Reference{Mention: mention, Quantity: quantity}
}

func assertReadyIDs(t *testing.T, got Result, role Role, want ...string) {
	t.Helper()
	if !got.Ready() {
		t.Fatalf("Ground() not ready: %#v", got)
	}
	assertReferenceIDs(t, got, role, want...)
}

func assertReferenceIDs(t *testing.T, got Result, role Role, want ...string) {
	t.Helper()
	reference, ok := got.Reference(role)
	if !ok {
		t.Fatalf("Ground() reference %q missing: %#v", role, got)
	}
	assertCandidateIDs(t, reference.Candidates, want...)
}

func assertCandidateIDs(t *testing.T, candidates []Candidate, want ...string) {
	t.Helper()
	got := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		got = append(got, candidate.ID)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate IDs = %v, want %v", got, want)
	}
}

func assertStaleBinding(t *testing.T, got Result, role Role, quantity intent.TargetMode, boundIDs []string, staleIDs ...string) {
	t.Helper()
	if got.Missing == nil || got.Missing.Reason != MissingReasonStaleBinding {
		t.Fatalf("Ground() missing = %#v, want stale binding", got.Missing)
	}
	if got.Missing.Role != role || got.Missing.Quantity != quantity {
		t.Fatalf("missing role/quantity = %q/%q, want %q/%q", got.Missing.Role, got.Missing.Quantity, role, quantity)
	}
	if !reflect.DeepEqual(got.Missing.BoundCandidateIDs, boundIDs) {
		t.Fatalf("bound IDs = %v, want %v", got.Missing.BoundCandidateIDs, boundIDs)
	}
	if !reflect.DeepEqual(got.Missing.StaleCandidateIDs, staleIDs) {
		t.Fatalf("stale IDs = %v, want %v", got.Missing.StaleCandidateIDs, staleIDs)
	}
}

func cloneSyntheticState(state *world.State) *world.State {
	clone := *state
	clone.Rooms = make(map[game.RoomID]world.Room, len(state.Rooms))
	for id, room := range state.Rooms {
		room.Objects = append([]game.ObjectID(nil), room.Objects...)
		room.Exits = append([]world.Exit(nil), room.Exits...)
		clone.Rooms[id] = room
	}
	clone.Objects = reverseObjectMap(reverseObjectMap(state.Objects))
	clone.Items = reverseItemMap(reverseItemMap(state.Items))
	clone.Doors = reverseDoorMap(reverseDoorMap(state.Doors))
	clone.Inventory = cloneBoolMap(state.Inventory)
	clone.DiscoveredItems = cloneBoolMap(state.DiscoveredItems)
	clone.KnownExitDirections = make(map[game.RoomID]map[string]bool, len(state.KnownExitDirections))
	for roomID, directions := range state.KnownExitDirections {
		copyDirections := make(map[string]bool, len(directions))
		for direction, known := range directions {
			copyDirections[direction] = known
		}
		clone.KnownExitDirections[roomID] = copyDirections
	}
	clone.RecentReferents = append([]game.ReferentGroup(nil), state.RecentReferents...)
	return &clone
}

func reverseRoomOrder(room world.Room) world.Room {
	room.Objects = append([]game.ObjectID(nil), room.Objects...)
	room.Exits = append([]world.Exit(nil), room.Exits...)
	for left, right := 0, len(room.Objects)-1; left < right; left, right = left+1, right-1 {
		room.Objects[left], room.Objects[right] = room.Objects[right], room.Objects[left]
	}
	for left, right := 0, len(room.Exits)-1; left < right; left, right = left+1, right-1 {
		room.Exits[left], room.Exits[right] = room.Exits[right], room.Exits[left]
	}
	return room
}

func reverseObjectMap(source map[game.ObjectID]world.Object) map[game.ObjectID]world.Object {
	ids := make([]game.ObjectID, 0, len(source))
	for id := range source {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	result := make(map[game.ObjectID]world.Object, len(source))
	for _, id := range ids {
		value := source[id]
		value.Aliases = append([]string(nil), value.Aliases...)
		value.ContainedItems = append([]game.ItemID(nil), value.ContainedItems...)
		result[id] = value
	}
	return result
}

func reverseItemMap(source map[game.ItemID]world.Item) map[game.ItemID]world.Item {
	ids := make([]game.ItemID, 0, len(source))
	for id := range source {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	result := make(map[game.ItemID]world.Item, len(source))
	for _, id := range ids {
		value := source[id]
		value.Aliases = append([]string(nil), value.Aliases...)
		result[id] = value
	}
	return result
}

func reverseDoorMap(source map[game.DoorID]world.Door) map[game.DoorID]world.Door {
	ids := make([]game.DoorID, 0, len(source))
	for id := range source {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	result := make(map[game.DoorID]world.Door, len(source))
	for _, id := range ids {
		value := source[id]
		value.Aliases = append([]string(nil), value.Aliases...)
		result[id] = value
	}
	return result
}

func cloneBoolMap[ID comparable](source map[ID]bool) map[ID]bool {
	result := make(map[ID]bool, len(source))
	for id, value := range source {
		result[id] = value
	}
	return result
}

func normalizedFuzzAlias(value string) string {
	runes := make([]rune, 0, len(value))
	for _, current := range value {
		if current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' || current == ' ' {
			runes = append(runes, current)
		}
		if len(runes) == 32 {
			break
		}
	}
	return string(runes)
}
