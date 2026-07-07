package world

import "kaya/internal/game"

type Exit struct {
	Direction string
	To        game.RoomID
	Door      game.DoorID
}

type Room struct {
	ID          game.RoomID
	Name        string
	Description string
	Visibility  Visibility
	Exits       []Exit
	Objects     []game.ObjectID
	Hazards     []game.HazardID
	Flags       map[string]bool
}

func (r Room) NeedsLight() bool {
	return r.Visibility.RequiresLight()
}
