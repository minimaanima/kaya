package rungen

import (
	"fmt"
	"sort"
)

func placementCombinations(rules []ItemRule) ([][]Placement, error) {
	normalized := append([]ItemRule(nil), rules...)
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].ItemID < normalized[j].ItemID
	})

	total := 1
	for i := range normalized {
		normalized[i].Candidates = append([]PlacementCandidate(nil), normalized[i].Candidates...)
		sort.Slice(normalized[i].Candidates, func(a, b int) bool {
			return normalized[i].Candidates[a].ObjectID < normalized[i].Candidates[b].ObjectID
		})
		candidateCount := len(normalized[i].Candidates)
		if candidateCount == 0 || total > MaxPlacementCombinations/candidateCount {
			return nil, fmt.Errorf("%w: placement combinations exceed %d", ErrInvalidDefinition, MaxPlacementCombinations)
		}
		total *= candidateCount
	}

	result := make([][]Placement, 0, total)
	var visit func(int, []Placement)
	visit = func(ruleIndex int, current []Placement) {
		if ruleIndex == len(normalized) {
			result = append(result, append([]Placement(nil), current...))
			return
		}
		rule := normalized[ruleIndex]
		for _, candidate := range rule.Candidates {
			visit(ruleIndex+1, append(current, Placement{
				ItemID:   rule.ItemID,
				ObjectID: candidate.ObjectID,
			}))
		}
	}
	visit(0, nil)
	return result, nil
}

func shufflePlacements(combinations [][]Placement, seed int64) {
	rng := splitMix64{state: uint64(seed)}
	for i := len(combinations) - 1; i > 0; i-- {
		j := int(rng.bounded(uint64(i + 1)))
		combinations[i], combinations[j] = combinations[j], combinations[i]
	}
}
