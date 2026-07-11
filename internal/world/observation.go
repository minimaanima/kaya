package world

import (
	"kaya/internal/game"
)

type ObservationMethod string

const (
	ObservationInspect ObservationMethod = "inspect"
	ObservationSearch  ObservationMethod = "search"
)

type ObservableFact struct {
	Kind     game.FactKind
	Value    string
	Text     string
	RevealOn []ObservationMethod
}

func (s *State) ObserveObject(objectID game.ObjectID, method ObservationMethod) []game.Fact {
	if s == nil || objectID == "" {
		return nil
	}
	object, ok := s.visibleObject(objectID)
	if !ok {
		return nil
	}
	if s.ObservedObjectFacts == nil {
		s.ObservedObjectFacts = make(map[game.ObjectID]map[game.FactKind]game.Fact)
	}
	if s.ObservedObjectFacts[objectID] == nil {
		s.ObservedObjectFacts[objectID] = make(map[game.FactKind]game.Fact)
	}

	var revealed []game.Fact
	for _, authored := range object.ObservableFacts {
		if !revealsOn(authored.RevealOn, method) {
			continue
		}
		if _, exists := s.ObservedObjectFacts[objectID][authored.Kind]; exists {
			continue
		}
		fact := game.Fact{
			ID:       game.FactID(string(objectID) + ":" + string(authored.Kind)),
			Kind:     authored.Kind,
			Subject:  string(objectID),
			Value:    authored.Value,
			Text:     authored.Text,
			Required: true,
		}
		s.ObservedObjectFacts[objectID][authored.Kind] = fact
		revealed = append(revealed, fact)
	}
	return revealed
}

func (s *State) ObservedFact(objectID game.ObjectID, kind game.FactKind) (game.Fact, bool) {
	if s == nil {
		return game.Fact{}, false
	}
	fact, ok := s.ObservedObjectFacts[objectID][kind]
	return fact, ok
}

func (s *State) visibleObject(objectID game.ObjectID) (Object, bool) {
	room, err := s.CurrentRoom()
	if err != nil {
		return Object{}, false
	}
	for _, candidateID := range room.Objects {
		if candidateID != objectID {
			continue
		}
		object, ok := s.Objects[objectID]
		return object, ok && s.CanSeeObject(room, object)
	}
	return Object{}, false
}

func revealsOn(methods []ObservationMethod, method ObservationMethod) bool {
	for _, candidate := range methods {
		if candidate == method {
			return true
		}
	}
	return false
}
