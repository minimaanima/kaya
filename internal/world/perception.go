package world

import (
	"sort"
	"strings"

	"kaya/internal/game"
)

const maxRecentReferentGroups = 3

func (s *State) PerceptionSnapshot() (game.PerceptionSnapshot, error) {
	room, err := s.CurrentRoom()
	if err != nil {
		return game.PerceptionSnapshot{}, err
	}

	objects, err := s.VisibleObjects()
	if err != nil {
		return game.PerceptionSnapshot{}, err
	}
	exits, err := s.AvailableExits()
	if err != nil {
		return game.PerceptionSnapshot{}, err
	}

	snapshot := game.PerceptionSnapshot{
		RoomName:        room.Name,
		HasUsefulLight:  s.ActiveLight || !room.NeedsLight(),
		VisibleObjects:  make([]game.PerceivedObject, 0, len(objects)),
		KnownExits:      make([]game.PerceivedExit, 0, len(exits)),
		Inventory:       make([]game.PerceivedItem, 0, len(s.Inventory)),
		RecentReferents: copyRecentReferents(s.RecentReferents),
	}
	for _, object := range objects {
		snapshot.VisibleObjects = append(snapshot.VisibleObjects, game.PerceivedObject{
			ID: object.ID, Name: object.Name, Aliases: append([]string(nil), object.Aliases...),
		})
	}
	for _, exit := range exits {
		snapshot.KnownExits = append(snapshot.KnownExits, game.PerceivedExit{Direction: exit.Direction})
	}

	itemIDs := make([]game.ItemID, 0, len(s.Inventory))
	for itemID, present := range s.Inventory {
		if present {
			itemIDs = append(itemIDs, itemID)
		}
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i] < itemIDs[j] })
	for _, itemID := range itemIDs {
		item, ok := s.Items[itemID]
		if !ok {
			continue
		}
		snapshot.Inventory = append(snapshot.Inventory, game.PerceivedItem{
			ID: item.ID, Name: item.Name, Aliases: append([]string(nil), item.Aliases...),
		})
	}

	return snapshot, nil
}

func (s *State) RememberObjects(ids []game.ObjectID) {
	if s == nil {
		return
	}
	seen := make(map[game.ObjectID]bool, len(ids))
	valid := make([]game.ObjectID, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		valid = append(valid, id)
	}
	group := game.ReferentGroup{ObjectIDs: valid}
	s.rememberReferent(group)
}

func (s *State) RememberItems(ids []game.ItemID) {
	if s == nil {
		return
	}
	seen := make(map[game.ItemID]bool, len(ids))
	valid := make([]game.ItemID, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		valid = append(valid, id)
	}
	s.rememberReferent(game.ReferentGroup{ItemIDs: valid})
}

func (s *State) ResolveObjectGroup(target string, all bool) (ObjectResolution, error) {
	target = normalizeTarget(target)
	if target == "" {
		return ObjectResolution{}, nil
	}

	switch target {
	case "it", "that":
		return s.resolveRememberedObjects(target, false)
	case "they", "them", "those", "both":
		return s.resolveRememberedObjects(target, true)
	}

	resolution, err := s.ResolveObject(target)
	if err != nil || len(resolution.Matches) > 0 {
		return resolution, err
	}
	if singular := singularLastWord(target); singular != target {
		return s.ResolveObject(singular)
	}
	return resolution, nil
}

func (s *State) resolveRememberedObjects(target string, plural bool) (ObjectResolution, error) {
	visible, err := s.VisibleObjects()
	if err != nil {
		return ObjectResolution{}, err
	}
	byID := make(map[game.ObjectID]Object, len(visible))
	for _, object := range visible {
		byID[object.ID] = object
	}
	for i := len(s.RecentReferents) - 1; i >= 0; i-- {
		ids := s.RecentReferents[i].ObjectIDs
		if (plural && len(ids) < 2) || (!plural && len(ids) != 1) {
			continue
		}
		matches := make([]Object, 0, len(ids))
		for _, id := range ids {
			if object, ok := byID[id]; ok {
				matches = append(matches, object)
			}
		}
		if (plural && len(matches) >= 2) || (!plural && len(matches) == 1) {
			return ObjectResolution{Target: target, Matches: matches}, nil
		}
	}
	return ObjectResolution{Target: target}, nil
}

func (s *State) rememberReferent(group game.ReferentGroup) {
	if len(group.ObjectIDs) == 0 && len(group.ItemIDs) == 0 {
		return
	}
	s.RecentReferents = append(s.RecentReferents, group)
	if len(s.RecentReferents) > maxRecentReferentGroups {
		s.RecentReferents = append([]game.ReferentGroup(nil), s.RecentReferents[len(s.RecentReferents)-maxRecentReferentGroups:]...)
	}
}

func copyRecentReferents(groups []game.ReferentGroup) []game.ReferentGroup {
	result := make([]game.ReferentGroup, 0, maxRecentReferentGroups)
	for _, group := range groups {
		if len(group.ObjectIDs) == 0 && len(group.ItemIDs) == 0 {
			continue
		}
		result = append(result, game.ReferentGroup{
			ObjectIDs: append([]game.ObjectID(nil), group.ObjectIDs...),
			ItemIDs:   append([]game.ItemID(nil), group.ItemIDs...),
		})
		if len(result) > maxRecentReferentGroups {
			result = append([]game.ReferentGroup(nil), result[len(result)-maxRecentReferentGroups:]...)
		}
	}
	return result
}

func singularLastWord(target string) string {
	words := strings.Fields(target)
	if len(words) == 0 || !strings.HasSuffix(words[len(words)-1], "s") || len(words[len(words)-1]) == 1 {
		return target
	}
	words[len(words)-1] = strings.TrimSuffix(words[len(words)-1], "s")
	return strings.Join(words, " ")
}
