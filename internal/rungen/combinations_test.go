package rungen

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"kaya/internal/game"
)

func TestPlacementCombinationsAreStableAndComplete(t *testing.T) {
	rules := []ItemRule{
		{ItemID: "flashlight", Candidates: []PlacementCandidate{{ObjectID: "floor"}, {ObjectID: "desk"}, {ObjectID: "chair"}}},
		{ItemID: "key", Candidates: []PlacementCandidate{{ObjectID: "doctor_door"}, {ObjectID: "cabinet"}, {ObjectID: "doctor_cabinet"}}},
	}

	got, err := placementCombinations(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 9 {
		t.Fatalf("combinations = %d, want 9", len(got))
	}
	wantFirst := []Placement{{ItemID: "flashlight", ObjectID: "chair"}, {ItemID: "key", ObjectID: "cabinet"}}
	if !reflect.DeepEqual(got[0], wantFirst) {
		t.Fatalf("first = %+v, want %+v", got[0], wantFirst)
	}
}

func TestShufflePlacementsRepeatsForSeed(t *testing.T) {
	rules := []ItemRule{
		{ItemID: "flashlight", Candidates: []PlacementCandidate{{ObjectID: "desk"}, {ObjectID: "floor"}, {ObjectID: "chair"}}},
		{ItemID: "key", Candidates: []PlacementCandidate{{ObjectID: "cabinet"}, {ObjectID: "doctor"}, {ObjectID: "locker"}}},
	}
	combinations, err := placementCombinations(rules)
	if err != nil {
		t.Fatal(err)
	}
	clone := func(source [][]Placement) [][]Placement {
		result := make([][]Placement, len(source))
		for i := range source {
			result[i] = append([]Placement(nil), source[i]...)
		}
		return result
	}
	a := clone(combinations)
	b := clone(combinations)

	shufflePlacements(a, 12345)
	shufflePlacements(b, 12345)

	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different orders")
	}
}

func TestPlacementCombinationsRejectsProductAboveLimit(t *testing.T) {
	rules := make([]ItemRule, 13)
	for i := range rules {
		rules[i] = ItemRule{
			ItemID:     game.ItemID(fmt.Sprintf("item_%02d", i)),
			Candidates: []PlacementCandidate{{ObjectID: "a"}, {ObjectID: "b"}},
		}
	}

	if _, err := placementCombinations(rules); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}
