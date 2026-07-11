package rungen

import (
	"errors"
	"fmt"
	"testing"

	"kaya/internal/game"
	"kaya/internal/world"
)

func TestValidateDefinitionAcceptsValidDefinition(t *testing.T) {
	if err := ValidateDefinition(validTestDefinition()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateDefinitionRejectsDuplicateCandidate(t *testing.T) {
	definition := validTestDefinition()
	definition.ItemRules[0].Candidates = []PlacementCandidate{{ObjectID: "desk"}, {ObjectID: "desk"}}

	if err := ValidateDefinition(definition); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateDefinitionRejectsNonSearchableCandidate(t *testing.T) {
	definition := validTestDefinition()
	definition.Build = func() *world.State {
		state := validTestState()
		object := state.Objects["desk"]
		object.Searchable = false
		state.Objects["desk"] = object
		return state
	}

	if err := ValidateDefinition(definition); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateDefinitionRejectsMoreThan64RequiredItems(t *testing.T) {
	definition := validTestDefinition()
	definition.ItemRules = make([]ItemRule, 65)
	for i := range definition.ItemRules {
		definition.ItemRules[i] = ItemRule{
			ItemID:     game.ItemID(fmt.Sprintf("item_%02d", i)),
			Candidates: []PlacementCandidate{{ObjectID: "desk"}},
		}
	}

	if err := ValidateDefinition(definition); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}

func validTestDefinition() Definition {
	return Definition{
		ScenarioID:      "test",
		ScenarioVersion: 1,
		Build:           validTestState,
		StartRoom:       "start",
		WinRoom:         "win",
		LightItem:       "flashlight",
		ItemRules: []ItemRule{{
			ItemID: "flashlight",
			Candidates: []PlacementCandidate{
				{ObjectID: "desk"},
				{ObjectID: "floor"},
			},
		}},
	}
}

func validTestState() *world.State {
	state := world.NewState("start")
	state.Rooms["start"] = world.Room{
		ID:      "start",
		Objects: []game.ObjectID{"desk", "floor"},
		Exits:   []world.Exit{{Direction: "north", To: "win"}},
	}
	state.Rooms["win"] = world.Room{ID: "win"}
	state.Objects["desk"] = world.Object{ID: "desk", Name: "Desk", Searchable: true}
	state.Objects["floor"] = world.Object{ID: "floor", Name: "Floor", Searchable: true}
	state.Items["flashlight"] = world.Item{ID: "flashlight", Name: "Flashlight", Portable: true}
	return state
}
