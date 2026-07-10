package world

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"kaya/internal/game"
	"kaya/internal/kaya"
)

var (
	ErrCurrentRoomMissing = errors.New("current room missing")
	ErrRoomNotFound       = errors.New("room not found")
)

type State struct {
	CurrentRoomID        game.RoomID
	PreviousRoomID       game.RoomID
	NowSeconds           int
	Rooms                map[game.RoomID]Room
	Doors                map[game.DoorID]Door
	Objects              map[game.ObjectID]Object
	Items                map[game.ItemID]Item
	Inventory            map[game.ItemID]bool
	DiscoveredItems      map[game.ItemID]bool
	LastMentionedItemID  game.ItemID
	LastMentionedItemIDs []game.ItemID
	ActiveLight          bool
	Kaya                 kaya.State

	ScheduledEvents []ScheduledEvent
}

func NewState(currentRoomID game.RoomID) *State {
	return &State{
		CurrentRoomID:   currentRoomID,
		Rooms:           make(map[game.RoomID]Room),
		Doors:           make(map[game.DoorID]Door),
		Objects:         make(map[game.ObjectID]Object),
		Items:           make(map[game.ItemID]Item),
		Inventory:       make(map[game.ItemID]bool),
		DiscoveredItems: make(map[game.ItemID]bool),
		Kaya:            kaya.DefaultState(),
	}
}

func (s *State) CurrentRoom() (Room, error) {
	if s == nil || s.CurrentRoomID == "" {
		return Room{}, ErrCurrentRoomMissing
	}

	room, ok := s.Rooms[s.CurrentRoomID]
	if !ok {
		return Room{}, fmt.Errorf("%w: %s", ErrRoomNotFound, s.CurrentRoomID)
	}

	return room, nil
}

func (s *State) AvailableExits() ([]Exit, error) {
	room, err := s.CurrentRoom()
	if err != nil {
		return nil, err
	}

	exits := make([]Exit, 0, len(room.Exits))
	for _, exit := range room.Exits {
		exits = append(exits, exit)
	}

	return exits, nil
}

func (s *State) VisibleObjects() ([]Object, error) {
	room, err := s.CurrentRoom()
	if err != nil {
		return nil, err
	}

	objects := make([]Object, 0, len(room.Objects))
	for _, objectID := range room.Objects {
		object, ok := s.Objects[objectID]
		if !ok {
			continue
		}
		if s.CanSeeObject(room, object) {
			objects = append(objects, object)
		}
	}

	return objects, nil
}

func (s *State) CanSeeObject(room Room, object Object) bool {
	return CanSeeObject(room, object, s != nil && s.ActiveLight)
}

func (s *State) HasItem(itemID game.ItemID) bool {
	if s == nil || itemID == "" {
		return false
	}
	return s.Inventory[itemID]
}

func (s *State) AddInventory(itemID game.ItemID) {
	if s == nil || itemID == "" {
		return
	}
	if s.Inventory == nil {
		s.Inventory = make(map[game.ItemID]bool)
	}
	s.DiscoverItem(itemID)
	s.Inventory[itemID] = true
}

func (s *State) RemoveInventory(itemID game.ItemID) {
	if s == nil || itemID == "" {
		return
	}
	delete(s.Inventory, itemID)
}

func (s *State) DiscoverItem(itemID game.ItemID) {
	if s == nil || itemID == "" {
		return
	}
	if s.DiscoveredItems == nil {
		s.DiscoveredItems = make(map[game.ItemID]bool)
	}
	s.DiscoveredItems[itemID] = true
	s.SetLastMentionedItems([]game.ItemID{itemID})
}

func (s *State) DiscoverItems(itemIDs []game.ItemID) {
	if s == nil {
		return
	}
	discovered := make([]game.ItemID, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		if itemID == "" {
			continue
		}
		if s.DiscoveredItems == nil {
			s.DiscoveredItems = make(map[game.ItemID]bool)
		}
		s.DiscoveredItems[itemID] = true
		discovered = append(discovered, itemID)
	}
	s.SetLastMentionedItems(discovered)
}

func (s *State) SetLastMentionedItems(itemIDs []game.ItemID) {
	if s == nil {
		return
	}
	s.LastMentionedItemIDs = append(s.LastMentionedItemIDs[:0], itemIDs...)
	if len(itemIDs) == 1 {
		s.LastMentionedItemID = itemIDs[0]
		return
	}
	s.LastMentionedItemID = ""
}

func (s *State) IsItemDiscovered(itemID game.ItemID) bool {
	if s == nil || itemID == "" {
		return false
	}
	return s.DiscoveredItems[itemID]
}

func (s *State) ResolveObject(target string) (ObjectResolution, error) {
	target = normalizeTarget(target)
	if target == "" {
		return ObjectResolution{}, nil
	}

	objects, err := s.VisibleObjects()
	if err != nil {
		return ObjectResolution{}, err
	}

	var matches []Object
	for _, object := range objects {
		if matchesName(target, object.Name, object.Aliases) {
			matches = append(matches, object)
		}
	}

	return ObjectResolution{Target: target, Matches: matches}, nil
}

func (s *State) ResolveDoor(target string) (DoorResolution, error) {
	target = normalizeTarget(target)
	if target == "" {
		return DoorResolution{}, nil
	}

	room, err := s.CurrentRoom()
	if err != nil {
		return DoorResolution{}, err
	}

	var matches []Door
	for _, exit := range room.Exits {
		if exit.Door == "" {
			continue
		}
		door, ok := s.Doors[exit.Door]
		if !ok {
			continue
		}
		aliases := append([]string{exit.Direction}, door.Aliases...)
		if matchesName(target, door.Name, aliases) {
			matches = append(matches, door)
		}
	}

	return DoorResolution{Target: target, Matches: matches}, nil
}

type ObjectResolution struct {
	Target  string
	Matches []Object
}

func (r ObjectResolution) Found() bool {
	return len(r.Matches) == 1
}

func (r ObjectResolution) Ambiguous() bool {
	return len(r.Matches) > 1
}

func (r ObjectResolution) Missing() bool {
	return len(r.Matches) == 0
}

type DoorResolution struct {
	Target  string
	Matches []Door
}

func (r DoorResolution) Found() bool {
	return len(r.Matches) == 1
}

func (r DoorResolution) Ambiguous() bool {
	return len(r.Matches) > 1
}

func (r DoorResolution) Missing() bool {
	return len(r.Matches) == 0
}

func matchesName(target string, name string, aliases []string) bool {
	candidates := append([]string{name}, aliases...)
	for _, candidate := range candidates {
		normalized := normalizeTarget(candidate)
		if normalized == "" {
			continue
		}
		if target == normalized || strings.Contains(normalized, target) {
			return true
		}
		if wordCount(normalized) > 1 && strings.Contains(target, normalized) {
			return true
		}
	}
	return false
}

func normalizeTarget(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "'s", "")

	words := strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})

	kept := words[:0]
	for _, word := range words {
		switch word {
		case "the", "a", "an", "s":
			continue
		default:
			kept = append(kept, word)
		}
	}

	return strings.Join(kept, " ")
}

func wordCount(value string) int {
	return len(strings.Fields(value))
}
