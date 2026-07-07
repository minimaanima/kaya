package world

import "kaya/internal/game"

type DoorState string

const (
	DoorOpen      DoorState = "open"
	DoorClosed    DoorState = "closed"
	DoorLocked    DoorState = "locked"
	DoorJammed    DoorState = "jammed"
	DoorDestroyed DoorState = "destroyed"
)

type Door struct {
	ID          game.DoorID
	Name        string
	From        game.RoomID
	To          game.RoomID
	State       DoorState
	RequiredKey game.ItemID
	NoiseOnUse  int
}

func (d Door) IsPassable() bool {
	return d.State == DoorOpen || d.State == DoorDestroyed
}

func (d Door) CanUnlockWith(itemID game.ItemID) bool {
	return d.State == DoorLocked && d.RequiredKey != "" && d.RequiredKey == itemID
}
