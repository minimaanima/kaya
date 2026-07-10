package runscenario

import (
	"testing"

	"kaya/internal/rungen"
)

func TestPrototypeDefinitionHasThreeCandidatesPerItem(t *testing.T) {
	definition := PrototypeDefinition()

	if err := rungen.ValidateDefinition(definition); err != nil {
		t.Fatal(err)
	}
	if len(definition.ItemRules) != 2 {
		t.Fatalf("rules = %d, want 2", len(definition.ItemRules))
	}
	for _, rule := range definition.ItemRules {
		if len(rule.Candidates) != 3 {
			t.Fatalf("item %q candidates = %d, want 3", rule.ItemID, len(rule.Candidates))
		}
	}
}
