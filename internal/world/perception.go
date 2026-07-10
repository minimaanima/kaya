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
		RecentReferents: s.perceivedReferents(objects),
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
		return ObjectResolution{All: all}, nil
	}

	switch target {
	case "it", "that":
		return s.resolveRememberedObjects(target, false, all)
	case "they", "them", "those", "both":
		return s.resolveRememberedObjects(target, true, all)
	}

	resolution, err := s.ResolveObject(target)
	if err != nil || len(resolution.Matches) > 0 {
		resolution.All = all
		return resolution, err
	}
	if singular := singularLastWord(target); singular != target {
		resolution, err = s.ResolveObject(singular)
		resolution.All = all
		return resolution, err
	}
	resolution.All = all
	return resolution, nil
}

func (s *State) resolveRememberedObjects(target string, plural bool, all bool) (ObjectResolution, error) {
	visible, err := s.VisibleObjects()
	if err != nil {
		return ObjectResolution{}, err
	}
	for i := len(s.RecentReferents) - 1; i >= 0; i-- {
		ids := s.RecentReferents[i].ObjectIDs
		if (plural && len(ids) < 2) || (!plural && len(ids) != 1) {
			continue
		}
		remembered := objectIDSet(ids)
		matches := make([]Object, 0, len(remembered))
		for _, object := range visible {
			if remembered[object.ID] {
				matches = append(matches, object)
			}
		}
		if (plural && len(matches) >= 2) || (!plural && len(matches) == 1) {
			return ObjectResolution{Target: target, Matches: matches, All: all}, nil
		}
	}
	return ObjectResolution{Target: target, All: all}, nil
}

func (s *State) rememberReferent(group game.ReferentGroup) {
	if len(group.ObjectIDs) == 0 && len(group.ItemIDs) == 0 {
		return
	}
	s.RecentReferents = appendCoalescedReferent(s.RecentReferents, group)
}

func (s *State) perceivedReferents(visible []Object) []game.ReferentGroup {
	result := make([]game.ReferentGroup, 0, maxRecentReferentGroups)
	for _, group := range s.RecentReferents {
		filtered := game.ReferentGroup{
			ObjectIDs: perceivedObjectIDs(group.ObjectIDs, visible),
			ItemIDs:   s.perceivedItemIDs(group.ItemIDs),
		}
		if len(filtered.ObjectIDs) == 0 && len(filtered.ItemIDs) == 0 {
			continue
		}
		result = appendCoalescedReferent(result, filtered)
	}
	return result
}

func perceivedObjectIDs(ids []game.ObjectID, visible []Object) []game.ObjectID {
	requested := objectIDSet(ids)
	perceived := make([]game.ObjectID, 0, len(requested))
	for _, object := range visible {
		if requested[object.ID] {
			perceived = append(perceived, object.ID)
		}
	}
	return perceived
}

func (s *State) perceivedItemIDs(ids []game.ItemID) []game.ItemID {
	seen := make(map[game.ItemID]bool, len(ids))
	perceived := make([]game.ItemID, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] || (!s.DiscoveredItems[id] && !s.Inventory[id]) {
			continue
		}
		if _, exists := s.Items[id]; !exists {
			continue
		}
		seen[id] = true
		perceived = append(perceived, id)
	}
	return perceived
}

func appendCoalescedReferent(groups []game.ReferentGroup, group game.ReferentGroup) []game.ReferentGroup {
	coalesced := make([]game.ReferentGroup, 0, len(groups)+1)
	for _, existing := range groups {
		if sameReferentGroup(existing, group) {
			continue
		}
		coalesced = append(coalesced, existing)
	}
	coalesced = append(coalesced, game.ReferentGroup{
		ObjectIDs: append([]game.ObjectID(nil), group.ObjectIDs...),
		ItemIDs:   append([]game.ItemID(nil), group.ItemIDs...),
	})
	if len(coalesced) > maxRecentReferentGroups {
		coalesced = append([]game.ReferentGroup(nil), coalesced[len(coalesced)-maxRecentReferentGroups:]...)
	}
	return coalesced
}

func sameReferentGroup(left, right game.ReferentGroup) bool {
	return sameObjectIDs(left.ObjectIDs, right.ObjectIDs) && sameItemIDs(left.ItemIDs, right.ItemIDs)
}

func sameObjectIDs(left, right []game.ObjectID) bool {
	leftSet := objectIDSet(left)
	rightSet := objectIDSet(right)
	if len(leftSet) != len(rightSet) {
		return false
	}
	for id := range leftSet {
		if !rightSet[id] {
			return false
		}
	}
	return true
}

func sameItemIDs(left, right []game.ItemID) bool {
	leftSet := make(map[game.ItemID]bool, len(left))
	rightSet := make(map[game.ItemID]bool, len(right))
	for _, id := range left {
		if id != "" {
			leftSet[id] = true
		}
	}
	for _, id := range right {
		if id != "" {
			rightSet[id] = true
		}
	}
	if len(leftSet) != len(rightSet) {
		return false
	}
	for id := range leftSet {
		if !rightSet[id] {
			return false
		}
	}
	return true
}

func objectIDSet(ids []game.ObjectID) map[game.ObjectID]bool {
	set := make(map[game.ObjectID]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			set[id] = true
		}
	}
	return set
}

func singularLastWord(target string) string {
	words := strings.Fields(target)
	if len(words) == 0 || !strings.HasSuffix(words[len(words)-1], "s") || len(words[len(words)-1]) == 1 {
		return target
	}
	words[len(words)-1] = strings.TrimSuffix(words[len(words)-1], "s")
	return strings.Join(words, " ")
}
