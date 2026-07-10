package world

import (
	"fmt"

	"kaya/internal/game"
)

func (s *State) ObserveRoom(roomID, enteredFrom game.RoomID) error {
	room, ok := s.Rooms[roomID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrRoomNotFound, roomID)
	}
	if s.KnownExitDirections == nil {
		s.KnownExitDirections = make(map[game.RoomID]map[string]bool)
	}
	if s.KnownExitDirections[roomID] == nil {
		s.KnownExitDirections[roomID] = make(map[string]bool)
	}
	for _, exit := range room.Exits {
		if s.ActiveLight || !room.NeedsLight() || exit.To == enteredFrom {
			s.KnownExitDirections[roomID][exit.Direction] = true
		}
	}
	return nil
}

func (s *State) AvailableExits() ([]Exit, error) {
	room, err := s.CurrentRoom()
	if err != nil {
		return nil, err
	}
	known := s.KnownExitDirections[room.ID]
	exits := make([]Exit, 0, len(room.Exits))
	for _, exit := range room.Exits {
		if known[exit.Direction] {
			exits = append(exits, exit)
		}
	}
	return exits, nil
}

func (s *State) DiscoverNextUnknownExit() (Exit, bool, error) {
	room, err := s.CurrentRoom()
	if err != nil {
		return Exit{}, false, err
	}
	for _, exit := range room.Exits {
		if !s.KnownExitDirections[room.ID][exit.Direction] {
			if s.KnownExitDirections[room.ID] == nil {
				s.KnownExitDirections[room.ID] = make(map[string]bool)
			}
			s.KnownExitDirections[room.ID][exit.Direction] = true
			return exit, true, nil
		}
	}
	return Exit{}, false, nil
}
