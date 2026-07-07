package world

import "kaya/internal/game"

type ObjectKind string

const (
	ObjectContainer ObjectKind = "container"
	ObjectSurface   ObjectKind = "surface"
	ObjectBody      ObjectKind = "body"
	ObjectTerminal  ObjectKind = "terminal"
	ObjectNote      ObjectKind = "note"
	ObjectDoor      ObjectKind = "door"
)

type Object struct {
	ID              game.ObjectID
	Name            string
	Description     string
	Kind            ObjectKind
	RequiresLight   bool
	Searchable      bool
	ContainedItems  []game.ItemID
	RevealedItemIDs []game.ItemID
	Flags           map[string]bool
}
