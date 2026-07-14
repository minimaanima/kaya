package grounding

import (
	"reflect"
	"sort"
	"strings"
	"unicode"

	"kaya/internal/intent"
	"kaya/internal/world"
)

const (
	scoreToken     = 100
	scoreAlias     = 300
	scoreExactName = 400
)

type Grounder struct {
	state *world.State
}

func New(state *world.State) Grounder {
	return Grounder{state: state}
}

func (g Grounder) Ground(action intent.SemanticAction, binding *Binding) Result {
	result := Result{Action: action}
	if g.state == nil {
		result.Err = ErrMissingWorld
		return result
	}
	actionValue := reflect.ValueOf(action)
	if !actionValue.IsValid() || actionValue.Kind() == reflect.Pointer && actionValue.IsNil() {
		result.Err = ErrUnsupportedAction
		return result
	}
	expectedRoles, supported := semanticActionRoles(action)
	if !supported {
		result.Err = ErrUnsupportedAction
		return result
	}
	if binding != nil && !containsRole(expectedRoles, binding.Role) {
		result.Missing = &MissingReference{
			Role:              binding.Role,
			Reason:            MissingReasonBindingRole,
			BoundCandidateIDs: append([]string(nil), binding.CandidateIDs...),
			ExpectedRoles:     append([]Role(nil), expectedRoles...),
		}
		return result
	}

	view, err := g.state.GroundingView()
	if err != nil {
		result.Err = err
		return result
	}

	ground := func(role Role, reference intent.Reference, candidates []Candidate) bool {
		resolved, clarification, missing := resolveReference(reference, role, candidates, view, binding)
		if missing != nil {
			result.Missing = missing
			return false
		}
		if clarification != nil {
			result.Clarification = clarification
			return false
		}
		result.References = append(result.References, resolved)
		return true
	}

	switch typed := action.(type) {
	case intent.MoveAction:
		ground(RoleExit, intent.Reference{Mention: typed.Direction, Quantity: intent.TargetOne}, exitCandidates(view))
	case *intent.MoveAction:
		ground(RoleExit, intent.Reference{Mention: typed.Direction, Quantity: intent.TargetOne}, exitCandidates(view))
	case intent.InspectAction:
		groundOptionalReference(&result, RoleObject, typed.Target, objectCandidates(view), view, binding)
	case *intent.InspectAction:
		groundOptionalReference(&result, RoleObject, typed.Target, objectCandidates(view), view, binding)
	case intent.SearchAction:
		ground(RoleObject, typed.Target, objectCandidates(view))
	case *intent.SearchAction:
		ground(RoleObject, typed.Target, objectCandidates(view))
	case intent.TakeAction:
		ground(RoleItem, typed.Target, discoveredItemCandidates(view))
	case *intent.TakeAction:
		ground(RoleItem, typed.Target, discoveredItemCandidates(view))
	case intent.UseAction:
		if ground(RoleItem, typed.Item, inventoryItemCandidates(view)) {
			ground(RoleDoor, typed.Target, doorCandidates(view))
		}
	case *intent.UseAction:
		if ground(RoleItem, typed.Item, inventoryItemCandidates(view)) {
			ground(RoleDoor, typed.Target, doorCandidates(view))
		}
	case intent.ToggleAction:
		ground(RoleItem, typed.Item, inventoryItemCandidates(view))
	case *intent.ToggleAction:
		ground(RoleItem, typed.Item, inventoryItemCandidates(view))
	case intent.TalkAction:
		ground(RoleObject, typed.Target, objectCandidates(view))
	case *intent.TalkAction:
		ground(RoleObject, typed.Target, objectCandidates(view))
	case intent.ListenAction:
		groundOptionalReference(&result, RoleDoor, typed.Target, doorCandidates(view), view, binding)
	case *intent.ListenAction:
		groundOptionalReference(&result, RoleDoor, typed.Target, doorCandidates(view), view, binding)
	case intent.ExploreAction, *intent.ExploreAction, intent.WaitAction, *intent.WaitAction:
	default:
		result.Err = ErrUnsupportedAction
	}

	return result
}

func groundOptionalReference(result *Result, role Role, reference intent.Reference, candidates []Candidate, view world.GroundingView, binding *Binding) {
	if strings.TrimSpace(reference.Mention) == "" && (binding == nil || binding.Role != role) {
		return
	}
	resolved, clarification, missing := resolveReference(reference, role, candidates, view, binding)
	result.Clarification = clarification
	result.Missing = missing
	if clarification == nil && missing == nil {
		result.References = append(result.References, resolved)
	}
}

func resolveReference(reference intent.Reference, role Role, candidates []Candidate, view world.GroundingView, binding *Binding) (GroundedReference, *Clarification, *MissingReference) {
	mention := strings.TrimSpace(reference.Mention)
	quantity := reference.Quantity
	if quantity == "" || quantity == intent.TargetSingle {
		quantity = intent.TargetOne
	}

	if binding != nil && binding.Role == role {
		bound, stale := revalidateCandidateIDs(candidates, binding.CandidateIDs)
		if len(binding.CandidateIDs) == 0 || len(stale) > 0 {
			return GroundedReference{}, nil, &MissingReference{
				Role:              role,
				Mention:           mention,
				Quantity:          quantity,
				Reason:            MissingReasonStaleBinding,
				BoundCandidateIDs: append([]string(nil), binding.CandidateIDs...),
				StaleCandidateIDs: stale,
			}
		}
		return GroundedReference{Role: role, Mention: mention, Quantity: quantity, Candidates: bound}, nil, nil
	}

	pronoun := referencePronoun(mention)
	if pronoun != pronounNone {
		recent := recentCandidates(role, pronoun, candidates, view)
		if len(recent) == 0 && len(candidates) == 0 {
			return GroundedReference{}, nil, unresolvedReference(role, mention, quantity)
		}
		if len(recent) == 0 {
			recent = cloneCandidates(candidates)
			if len(recent) > 1 {
				return GroundedReference{}, &Clarification{Role: role, Mention: mention, Candidates: recent}, nil
			}
		}
		if quantity != intent.TargetAll && len(recent) > 1 {
			return GroundedReference{}, &Clarification{Role: role, Mention: mention, Candidates: recent}, nil
		}
		return GroundedReference{Role: role, Mention: mention, Quantity: quantity, Candidates: recent}, nil, nil
	}

	matches := rankedMatches(mention, candidates)
	if len(matches) == 0 {
		return GroundedReference{}, nil, unresolvedReference(role, mention, quantity)
	}
	if quantity != intent.TargetAll && len(matches) > 1 {
		return GroundedReference{}, &Clarification{Role: role, Mention: mention, Candidates: matches}, nil
	}
	return GroundedReference{Role: role, Mention: mention, Quantity: quantity, Candidates: matches}, nil, nil
}

func rankedMatches(mention string, candidates []Candidate) []Candidate {
	target := normalizeReference(mention)
	if target == "" {
		return nil
	}

	topScore := 0
	matches := make([]Candidate, 0)
	for _, candidate := range candidates {
		score := candidateScore(target, candidate)
		if score == 0 || score < topScore {
			continue
		}
		if score > topScore {
			topScore = score
			matches = matches[:0]
		}
		matches = append(matches, cloneCandidate(candidate))
	}
	sortCandidates(matches)
	return matches
}

func candidateScore(target string, candidate Candidate) int {
	if normalizeReference(candidate.Name) == target {
		return scoreExactName
	}
	best := 0
	for _, alias := range candidate.Aliases {
		if normalizeReference(alias) == target {
			return scoreAlias
		}
		if tokenMatch(target, normalizeReference(alias)) {
			best = scoreToken
		}
	}
	if tokenMatch(target, normalizeReference(candidate.Name)) {
		best = scoreToken
	}
	return best
}

func tokenMatch(target, candidate string) bool {
	targetTokens := normalizedTokens(target)
	candidateTokens := normalizedTokens(candidate)
	if len(targetTokens) == 0 || len(candidateTokens) == 0 {
		return false
	}
	available := make(map[string]bool, len(candidateTokens))
	for _, token := range candidateTokens {
		available[singularToken(token)] = true
	}
	for _, token := range targetTokens {
		if !available[singularToken(token)] {
			return false
		}
	}
	return true
}

func normalizeReference(value string) string {
	words := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(current rune) bool {
		return unicode.IsPunct(current) || unicode.IsSpace(current)
	})
	kept := words[:0]
	for _, word := range words {
		switch word {
		case "the", "a", "an":
			continue
		default:
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
}

func normalizedTokens(value string) []string {
	return strings.Fields(normalizeReference(value))
}

func singularToken(value string) string {
	if len(value) > 3 && strings.HasSuffix(value, "s") && !strings.HasSuffix(value, "ss") {
		return strings.TrimSuffix(value, "s")
	}
	return value
}

type pronounKind uint8

const (
	pronounNone pronounKind = iota
	pronounSingular
	pronounPlural
)

func referencePronoun(mention string) pronounKind {
	normalized := normalizeReference(mention)
	words := strings.Fields(normalized)
	if len(words) == 0 {
		return pronounNone
	}
	switch words[0] {
	case "they", "them", "those", "these", "both":
		return pronounPlural
	case "it", "that", "this", "one":
		return pronounSingular
	}
	return pronounNone
}

func recentCandidates(role Role, pronoun pronounKind, candidates []Candidate, view world.GroundingView) []Candidate {
	eligible := make(map[string]Candidate, len(candidates))
	for _, candidate := range candidates {
		eligible[candidate.ID] = candidate
	}
	for index := len(view.RecentReferents) - 1; index >= 0; index-- {
		var ids []string
		switch role {
		case RoleObject:
			for _, id := range view.RecentReferents[index].ObjectIDs {
				ids = append(ids, string(id))
			}
		case RoleItem:
			for _, id := range view.RecentReferents[index].ItemIDs {
				ids = append(ids, string(id))
			}
		default:
			continue
		}
		matched := candidatesByIDMap(eligible, ids)
		if pronoun == pronounSingular && len(matched) == 1 {
			return matched
		}
		if pronoun == pronounPlural && len(matched) > 1 {
			return matched
		}
	}
	return nil
}

func revalidateCandidateIDs(candidates []Candidate, ids []string) ([]Candidate, []string) {
	eligible := make(map[string]Candidate, len(candidates))
	for _, candidate := range candidates {
		eligible[candidate.ID] = candidate
	}
	seen := make(map[string]bool, len(ids))
	selected := make([]Candidate, 0, len(ids))
	stale := make([]string, 0)
	for _, id := range ids {
		candidate, ok := eligible[id]
		if id == "" || !ok || seen[id] {
			stale = append(stale, id)
			continue
		}
		seen[id] = true
		selected = append(selected, cloneCandidate(candidate))
	}
	sortCandidates(selected)
	return selected, stale
}

func candidatesByIDMap(eligible map[string]Candidate, ids []string) []Candidate {
	seen := make(map[string]bool, len(ids))
	selected := make([]Candidate, 0, len(ids))
	for _, id := range ids {
		candidate, ok := eligible[id]
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		selected = append(selected, cloneCandidate(candidate))
	}
	sortCandidates(selected)
	return selected
}

func objectCandidates(view world.GroundingView) []Candidate {
	candidates := make([]Candidate, 0, len(view.Objects))
	for _, object := range view.Objects {
		candidates = append(candidates, Candidate{Kind: CandidateObject, ID: string(object.ID), Name: object.Name, Aliases: append([]string(nil), object.Aliases...)})
	}
	return candidates
}

func discoveredItemCandidates(view world.GroundingView) []Candidate {
	return itemCandidates(view.DiscoveredItems)
}

func inventoryItemCandidates(view world.GroundingView) []Candidate {
	return itemCandidates(view.InventoryItems)
}

func itemCandidates(items []world.Item) []Candidate {
	candidates := make([]Candidate, 0, len(items))
	for _, item := range items {
		candidates = append(candidates, Candidate{Kind: CandidateItem, ID: string(item.ID), Name: item.Name, Aliases: append([]string(nil), item.Aliases...)})
	}
	return candidates
}

func doorCandidates(view world.GroundingView) []Candidate {
	directions := make(map[string][]string, len(view.Doors))
	for _, exit := range view.Exits {
		if exit.Door != "" {
			directions[string(exit.Door)] = append(directions[string(exit.Door)], exit.Direction)
		}
	}
	candidates := make([]Candidate, 0, len(view.Doors))
	for _, door := range view.Doors {
		aliases := append([]string(nil), door.Aliases...)
		aliases = append(aliases, directions[string(door.ID)]...)
		candidates = append(candidates, Candidate{Kind: CandidateDoor, ID: string(door.ID), Name: door.Name, Aliases: aliases})
	}
	return candidates
}

func exitCandidates(view world.GroundingView) []Candidate {
	candidates := make([]Candidate, 0, len(view.Exits))
	for _, exit := range view.Exits {
		candidates = append(candidates, Candidate{Kind: CandidateExit, ID: exit.Direction, Name: exit.Direction})
	}
	return candidates
}

func cloneCandidate(candidate Candidate) Candidate {
	candidate.Aliases = append([]string(nil), candidate.Aliases...)
	sort.Strings(candidate.Aliases)
	return candidate
}

func cloneCandidates(candidates []Candidate) []Candidate {
	cloned := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		cloned = append(cloned, cloneCandidate(candidate))
	}
	sortCandidates(cloned)
	return cloned
}

func sortCandidates(candidates []Candidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Kind != candidates[j].Kind {
			return candidates[i].Kind < candidates[j].Kind
		}
		return candidates[i].ID < candidates[j].ID
	})
}

func unresolvedReference(role Role, mention string, quantity intent.TargetMode) *MissingReference {
	return &MissingReference{
		Role:     role,
		Mention:  mention,
		Quantity: quantity,
		Reason:   MissingReasonUnresolved,
	}
}

func semanticActionRoles(action intent.SemanticAction) ([]Role, bool) {
	switch action.(type) {
	case intent.MoveAction, *intent.MoveAction:
		return []Role{RoleExit}, true
	case intent.InspectAction, *intent.InspectAction,
		intent.SearchAction, *intent.SearchAction,
		intent.TalkAction, *intent.TalkAction:
		return []Role{RoleObject}, true
	case intent.TakeAction, *intent.TakeAction,
		intent.ToggleAction, *intent.ToggleAction:
		return []Role{RoleItem}, true
	case intent.UseAction, *intent.UseAction:
		return []Role{RoleItem, RoleDoor}, true
	case intent.ListenAction, *intent.ListenAction:
		return []Role{RoleDoor}, true
	case intent.ExploreAction, *intent.ExploreAction,
		intent.WaitAction, *intent.WaitAction:
		return nil, true
	default:
		return nil, false
	}
}

func containsRole(roles []Role, wanted Role) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
