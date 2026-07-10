package scenario

import (
	"kaya/internal/game"
	"kaya/internal/world"
)

const (
	RoomReception game.RoomID = "reception"
	RoomStorage   game.RoomID = "storage"
	RoomStairwell game.RoomID = "stairwell"

	DoorStairwell game.DoorID = "door_stairwell"

	ObjectReceptionDesk  game.ObjectID = "reception_desk"
	ObjectReceptionFloor game.ObjectID = "reception_floor"
	ObjectCollapsedChair game.ObjectID = "collapsed_chair"
	ObjectBodyCabinet    game.ObjectID = "body_cabinet"
	ObjectBodyDoor       game.ObjectID = "body_door"
	ObjectStorageCabinet game.ObjectID = "storage_cabinet"

	ItemBrassKey   game.ItemID = "brass_key"
	ItemFlashlight game.ItemID = "flashlight"

	PrototypeScenarioID      = "prototype_escape"
	PrototypeScenarioVersion = 1
)

func NewPrototypeTemplate() *world.State {
	state := world.NewState(RoomReception)

	state.Rooms[RoomReception] = world.Room{
		ID:          RoomReception,
		Name:        "Reception",
		Description: "A damaged reception area. The ceiling has split above the front desk.",
		Visibility:  world.VisibilityLit,
		Exits: []world.Exit{{
			Direction: "east",
			To:        RoomStorage,
		}},
		Objects: []game.ObjectID{ObjectReceptionDesk, ObjectReceptionFloor, ObjectCollapsedChair},
	}
	state.Rooms[RoomStorage] = world.Room{
		ID:          RoomStorage,
		Name:        "Storage Room",
		Description: "A pitch-black storage room with overturned cabinets and a chemical smell.",
		Visibility:  world.VisibilityPitchBlack,
		Exits: []world.Exit{{
			Direction: "west",
			To:        RoomReception,
		}, {
			Direction: "north",
			To:        RoomStairwell,
			Door:      DoorStairwell,
		}},
		Objects: []game.ObjectID{ObjectBodyCabinet, ObjectBodyDoor, ObjectStorageCabinet},
	}
	state.Rooms[RoomStairwell] = world.Room{
		ID:          RoomStairwell,
		Name:        "Emergency Stairwell",
		Description: "A concrete stairwell beyond a locked fire door.",
		Visibility:  world.VisibilityDim,
	}

	state.Doors[DoorStairwell] = world.Door{
		ID:          DoorStairwell,
		Name:        "Emergency Stairwell Door",
		Aliases:     []string{"stairwell door", "fire door", "emergency door"},
		From:        RoomStorage,
		To:          RoomStairwell,
		State:       world.DoorLocked,
		RequiredKey: ItemBrassKey,
	}

	state.Objects[ObjectReceptionDesk] = world.Object{
		ID:          ObjectReceptionDesk,
		Name:        "Reception Desk",
		Aliases:     []string{"desk", "table", "front desk", "drawer", "drawers"},
		Description: "A cracked laminate desk with drawers hanging open.",
		Kind:        world.ObjectSurface,
		Searchable:  true,
	}
	state.Objects[ObjectReceptionFloor] = world.Object{
		ID:          ObjectReceptionFloor,
		Name:        "Reception Floor",
		Aliases:     []string{"floor", "reception floor", "broken tiles"},
		Description: "Broken tiles and fallen ceiling panels cover the floor.",
		Kind:        world.ObjectSurface,
		Searchable:  true,
	}
	state.Objects[ObjectCollapsedChair] = world.Object{
		ID:          ObjectCollapsedChair,
		Name:        "Collapsed Chair",
		Aliases:     []string{"chair", "collapsed chair", "office chair"},
		Description: "A collapsed office chair lies beneath a torn coat.",
		Kind:        world.ObjectSurface,
		Searchable:  true,
	}
	state.Objects[ObjectBodyCabinet] = world.Object{
		ID:            ObjectBodyCabinet,
		Name:          "Doctor Near Cabinet",
		Aliases:       []string{"doctor", "body", "corpse", "doctor near cabinet", "coat pockets"},
		Description:   "A dead doctor slumped beside a metal cabinet.",
		Kind:          world.ObjectBody,
		RequiresLight: true,
		Searchable:    true,
	}
	state.Objects[ObjectBodyDoor] = world.Object{
		ID:            ObjectBodyDoor,
		Name:          "Doctor Near Door",
		Aliases:       []string{"doctor", "body", "corpse", "doctor near door"},
		Description:   "A dead doctor collapsed near the stairwell door.",
		Kind:          world.ObjectBody,
		RequiresLight: false,
		Searchable:    true,
	}
	state.Objects[ObjectStorageCabinet] = world.Object{
		ID:            ObjectStorageCabinet,
		Name:          "Storage Cabinet",
		Aliases:       []string{"storage cabinet", "metal cabinet"},
		Description:   "A dented storage cabinet stands against the dark wall.",
		Kind:          world.ObjectContainer,
		RequiresLight: true,
		Searchable:    true,
	}

	state.Items[ItemBrassKey] = world.Item{
		ID:          ItemBrassKey,
		Name:        "Brass Key",
		Aliases:     []string{"key", "small key", "brass key"},
		Description: "A small brass key.",
		Portable:    true,
	}
	state.Items[ItemFlashlight] = world.Item{
		ID:          ItemFlashlight,
		Name:        "Flashlight",
		Aliases:     []string{"torch", "light"},
		Description: "A heavy flashlight with a weak beam.",
		Portable:    true,
	}

	state.ScheduleEvent(45, game.WorldEvent{
		Type:        game.EventSound,
		Description: "Somewhere deeper in the building, metal scrapes against concrete.",
		Danger:      game.DangerLow,
	})

	_ = state.ObserveRoom(RoomReception, "")
	return state
}

func NewPrototypeWorld() *world.State {
	state := NewPrototypeTemplate()

	desk := state.Objects[ObjectReceptionDesk]
	desk.ContainedItems = []game.ItemID{ItemFlashlight}
	state.Objects[desk.ID] = desk

	body := state.Objects[ObjectBodyCabinet]
	body.ContainedItems = []game.ItemID{ItemBrassKey}
	state.Objects[body.ID] = body

	return state
}
