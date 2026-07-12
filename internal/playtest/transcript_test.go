package playtest

import (
	"errors"
	"strings"
	"testing"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/kaya"
	"kaya/internal/response"
	"kaya/internal/rungen"
	"kaya/internal/session"
	"kaya/internal/turn"
	"kaya/internal/world"
)

func TestRenderMarkdownIncludesReproductionEvidence(t *testing.T) {
	before := transcriptSnapshot()
	after := transcriptSnapshot()
	after.CurrentRoom = "storage"
	after.Time = 12
	after.ActiveLight = true
	after.Inventory = []game.ItemID{"flashlight", "brass_key"}

	session := Session{
		ScenarioID:       "kaya-prototype",
		ScenarioVersion:  3,
		GeneratorVersion: 7,
		Seed:             42,
		Placements: []rungen.Placement{
			{ItemID: "brass_key", ObjectID: "cabinet"},
			{ItemID: "flashlight", ObjectID: "desk"},
		},
		Steps: []Step{
			{
				Number: 1,
				Player: "look around\n```\nnot a fence",
				Before: before,
				Turn: session.ProcessedTurn{
					Plan: intent.TurnPlan{Actions: []intent.PlannedAction{{
						Intent:     intent.Intent{Action: intent.ActionInspect, RawText: "look around"},
						TargetMode: intent.TargetSingle,
					}}, Confidence: 0.8},
					Provenance: intent.ParseProvenance{
						Source:        intent.ParseSourceRepair,
						RawPlan:       intent.TurnPlan{Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionTalk}, TargetMode: intent.TargetSingle}}},
						HasRawPlan:    true,
						Canonicalized: true,
						RepairReason:  errors.New("initial decode failed"),
						FallbackError: errors.New("fallback was not needed"),
					},
					Result: turn.Result{Outcomes: []turn.ActionOutcome{{
						Intent: intent.Intent{Action: intent.ActionInspect},
						Result: game.ActionResult{
							Outcome: "inspected_room",
							Status:  game.ActionSucceeded,
							Events:  []game.WorldEvent{{Type: game.EventSound, Description: "knock", Danger: game.DangerLow}},
						},
					}}},
					Response: response.Response{
						Text:           "I heard ``` a knock.",
						UsedFallback:   true,
						FallbackReason: "generator unavailable",
						UsedFactIDs:    []game.FactID{"z", "a"},
					},
				},
				After:            after,
				ObjectiveEmitted: true,
				Violations: []Violation{
					{Code: "zeta", Detail: "later"},
					{Code: "alpha", Detail: "first ``` violation"},
				},
			},
			{
				Number: 2,
				Player: "wait",
				Before: after,
				After:  after,
				Error:  "parser said ``` nope",
			},
		},
		ObjectiveEmissions: 1,
	}

	got := RenderMarkdown(session)
	for _, expected := range []string{
		"# Kaya Stateful Playtest",
		"Scenario: `kaya-prototype`",
		"Scenario version: `3`",
		"Generator version: `7`",
		"Seed: `42`",
		"Flashlight",
		"Player:\n",
		"Raw actions:",
		"Resolved actions:",
		"Parse source:",
		"Generator provenance: `used`",
		"Provenance errors:",
		"Outcomes:",
		"Events:",
		"Response metadata:",
		"Used fact IDs: `a`, `z`",
		"Before:",
		"After:",
		"State diff:",
		"Violations:",
		"Objective emitted: `true`",
		"Objective emissions: `1`",
		"parser said ``` nope",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q:\n%s", expected, got)
		}
	}
	if strings.Contains(got, "Player: `look around\n```") {
		t.Fatalf("player text was rendered inline:\n%s", got)
	}
	if strings.Contains(got, "detail=\"first ``` violation\"") {
		t.Fatalf("violation detail was rendered inline:\n%s", got)
	}
	if events := strings.Count(got, "Events:\n"); events != len(session.Steps) {
		t.Fatalf("event sections = %d, want %d:\n%s", events, len(session.Steps), got)
	}
}

func TestRenderMarkdownIsStableAcrossMapIteration(t *testing.T) {
	first := Session{Seed: 9, Steps: []Step{{Before: transcriptSnapshot(), After: transcriptSnapshot()}}}
	second := Session{Seed: 9, Steps: []Step{{Before: transcriptSnapshotReordered(), After: transcriptSnapshotReordered()}}}

	if got, want := RenderMarkdown(first), RenderMarkdown(second); got != want {
		t.Fatalf("transcripts differ for equivalent snapshots:\nfirst:\n%s\nsecond:\n%s", got, want)
	}
}

func transcriptSnapshot() Snapshot {
	return Snapshot{
		CurrentRoom: "reception",
		Time:        3,
		Inventory:   []game.ItemID{"flashlight"},
		Discovered:  []game.ItemID{"brass_key"},
		ItemNames: map[game.ItemID]string{
			"flashlight": "Flashlight",
			"brass_key":  "Brass Key",
		},
		ItemAliases: map[game.ItemID][]string{
			"flashlight": {"torch", "light"},
			"brass_key":  {"small key", "key"},
		},
		ObjectItems: map[game.ObjectID][]game.ItemID{
			"desk":    {"flashlight"},
			"cabinet": {"brass_key"},
		},
		ObjectRevealedItems: map[game.ObjectID][]game.ItemID{
			"desk": {"flashlight"},
		},
		DoorStates: map[game.DoorID]world.DoorState{
			"stairwell": world.DoorLocked,
		},
		RemainingEventTimes: []int{11, 5},
		RemainingEvents: []world.ScheduledEvent{
			{TriggerAtSeconds: 11, Event: game.WorldEvent{Type: game.EventSound, Description: "later"}},
			{TriggerAtSeconds: 5, Event: game.WorldEvent{Type: game.EventSound, Description: "sooner"}},
		},
		Kaya: kaya.State{Stress: 2, Trust: 3, Fear: 4, Pain: 5, Exhaustion: 6},
	}
}

func transcriptSnapshotReordered() Snapshot {
	snapshot := transcriptSnapshot()
	snapshot.Inventory = []game.ItemID{"flashlight"}
	snapshot.ItemNames = map[game.ItemID]string{
		"brass_key":  "Brass Key",
		"flashlight": "Flashlight",
	}
	snapshot.ItemAliases = map[game.ItemID][]string{
		"brass_key":  {"key", "small key"},
		"flashlight": {"light", "torch"},
	}
	snapshot.ObjectItems = map[game.ObjectID][]game.ItemID{
		"cabinet": {"brass_key"},
		"desk":    {"flashlight"},
	}
	snapshot.RemainingEvents = []world.ScheduledEvent{
		{TriggerAtSeconds: 5, Event: game.WorldEvent{Type: game.EventSound, Description: "sooner"}},
		{TriggerAtSeconds: 11, Event: game.WorldEvent{Type: game.EventSound, Description: "later"}},
	}
	return snapshot
}
