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
				Number:    1,
				Player:    "look around\n```\nnot a fence",
				Before:    before,
				Processed: true,
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
						Text:                    "I heard ``` a knock.",
						UsedFallback:            true,
						FallbackReason:          "generator unavailable",
						UsedFactIDs:             []game.FactID{"z", "a"},
						RepairAttempted:         true,
						InitialValidationReason: "unsupported_claim",
						RepairValidationReason:  "unknown_entity",
						RepairGenerationError:   "generate repaired response: offline",
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
		"Processed: `true`",
		"Processed: `false`",
		"Raw actions:",
		"Resolved actions:",
		"Parse source:",
		"Generator provenance: `used`",
		"Provenance errors:",
		"Outcomes:",
		"Events:",
		"Response metadata:",
		"Used fact IDs: `z`, `a`",
		"Repair attempted: `true`",
		"Repair succeeded: `false`",
		"Initial validation reason:",
		"Repair validation reason:",
		"Repair generation error:",
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
	if events := strings.Count(got, "Events:\n"); events != 1 {
		t.Fatalf("event sections = %d, want 1 processed turn:\n%s", events, got)
	}
}

func TestRenderMarkdownMarksUnprocessedTurnEvidenceUnavailable(t *testing.T) {
	before := transcriptSnapshot()
	after := transcriptSnapshot()
	after.Time = before.Time + 1
	step := Step{
		Number:           1,
		Player:           "go east",
		Before:           before,
		After:            after,
		ObjectiveEmitted: true,
		Violations:       []Violation{{Code: "event_before_current_time", Detail: "scheduled event is stale"}},
		Error:            "parser unavailable",
	}

	got := RenderMarkdown(Session{Steps: []Step{step}})
	for _, expected := range []string{
		"Processed: `false`",
		"Raw actions:\n- unavailable",
		"Resolved actions:\n- unavailable",
		"Result evidence:\n- unavailable",
		"Response evidence:\n- unavailable",
		"Before:",
		"After:",
		"State diff:",
		"Violations:",
		"parser unavailable",
		"Objective emitted: `false`",
		"Objective emissions: `0`",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q:\n%s", expected, got)
		}
	}
	for _, unexpected := range []string{
		"Parse source:",
		"Processed turn duration:",
		"Result metadata:",
		"Response text:",
		"Response metadata:",
		"emotion=\"\"",
		"Objective emitted: `true`",
	} {
		if strings.Contains(got, unexpected) {
			t.Fatalf("unexpected unprocessed turn evidence %q:\n%s", unexpected, got)
		}
	}
}

func TestRenderMarkdownIsStableAcrossMapIteration(t *testing.T) {
	first := Session{Seed: 9, Steps: []Step{{Before: transcriptSnapshot(), After: transcriptSnapshot()}}}
	second := Session{Seed: 9, Steps: []Step{{Before: transcriptSnapshotReordered(), After: transcriptSnapshotReordered()}}}

	if got, want := RenderMarkdown(first), RenderMarkdown(second); got != want {
		t.Fatalf("transcripts differ for equivalent snapshots:\nfirst:\n%s\nsecond:\n%s", got, want)
	}
}

func TestRenderMarkdownPreservesCompleteTurnEvidenceAndSemanticOrder(t *testing.T) {
	step := Step{
		Number:    1,
		Processed: true,
		Turn: session.ProcessedTurn{
			DurationSeconds: 17,
			Plan: intent.TurnPlan{
				Actions: []intent.PlannedAction{{
					Intent: intent.Intent{
						Action:                intent.ActionMove,
						Target:                "target ``` text",
						Item:                  "key",
						Direction:             "north",
						Modifiers:             []string{"then", "quietly"},
						Confidence:            0.75,
						RawText:               "intent raw ``` text",
						NeedsClarification:    true,
						ClarificationQuestion: "intent question ``` text",
					},
					TargetMode: intent.TargetAll,
				}},
				Questions:             []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "doctors", TargetMode: intent.TargetAll}},
				Confidence:            0.9,
				NeedsClarification:    true,
				ClarificationQuestion: "turn question ``` text",
				RawText:               "turn raw ``` text",
			},
			Provenance: intent.ParseProvenance{
				Source: intent.ParseSourceModel,
				RawPlan: intent.TurnPlan{
					Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionTalk, RawText: "raw action text"}, TargetMode: intent.TargetSingle}},
					RawText: "raw plan text",
				},
				HasRawPlan: true,
			},
			Result: turn.Result{
				StopReason:            "stop ``` text",
				ClarificationQuestion: "result question ``` text",
				Emotion:               kaya.EmotionScared,
				QuestionFacts: []game.Fact{
					{ID: "question-z", Text: "question z"},
					{ID: "question-a", Text: "question a"},
				},
				Outcomes: []turn.ActionOutcome{{
					Intent:         intent.Intent{Action: intent.ActionMove, RawText: "outcome intent raw", NeedsClarification: true, ClarificationQuestion: "outcome intent question"},
					TargetObjectID: "desk",
					Result: game.ActionResult{
						Status:                game.ActionClarification,
						TargetObjectIDs:       []game.ObjectID{"desk", "door"},
						StartedAtSeconds:      11,
						DurationSeconds:       6,
						Outcome:               "outcome ``` text",
						VisibleFacts:          []game.Fact{{ID: "visible-z", Text: "visible z"}, {ID: "visible-a", Text: "visible a"}},
						Events:                []game.WorldEvent{{Description: "event z"}, {Description: "event a"}},
						StressDelta:           1,
						TrustDelta:            -2,
						FearDelta:             3,
						PainDelta:             -4,
						ExhaustionDelta:       5,
						Danger:                game.DangerHigh,
						NeedsClarification:    true,
						ClarificationQuestion: "action result question ``` text",
					},
				}},
			},
			Response: response.Response{UsedFactIDs: []game.FactID{"fact-z", "fact-a"}},
		},
		Violations: []Violation{{Code: "code ``` text", Detail: "detail"}},
	}

	got := RenderMarkdown(Session{Steps: []Step{step}})
	for _, expected := range []string{
		"Processed: `true`",
		"Processed turn duration: `17`",
		"turn raw ``` text",
		"intent raw ``` text",
		"intent question ``` text",
		"stop ``` text",
		"result question ``` text",
		"emotion=\"scared\"",
		"target_object_ids=[desk,door]",
		"stress_delta=1 trust_delta=-2 fear_delta=3 pain_delta=-4 exhaustion_delta=5",
		"action result question ``` text",
		"Used fact IDs: `fact-z`, `fact-a`",
		"Violation code:",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing %q:\n%s", expected, got)
		}
	}
	for _, ordered := range [][2]string{
		{"visible z", "visible a"},
		{"event z", "event a"},
		{"question z", "question a"},
	} {
		if strings.Index(got, ordered[0]) > strings.Index(got, ordered[1]) {
			t.Fatalf("semantic sequence reordered %q before %q:\n%s", ordered[0], ordered[1], got)
		}
	}
	if strings.Contains(got, "- code=`code ``` text`") {
		t.Fatalf("violation code was rendered inline:\n%s", got)
	}
}

func TestRenderMarkdownDistinguishesPresentEmptyStateEntries(t *testing.T) {
	missing := Snapshot{}
	present := Snapshot{
		ItemAliases:         map[game.ItemID][]string{"flashlight": {}},
		ObjectItems:         map[game.ObjectID][]game.ItemID{"desk": {}},
		ObjectRevealedItems: map[game.ObjectID][]game.ItemID{"desk": {}},
		KnownExitDirections: map[game.RoomID]map[string]bool{"reception": {}},
		ObservedObjectFacts: map[game.ObjectID]map[game.FactKind]game.Fact{"desk": {}},
	}

	got := RenderMarkdown(Session{Steps: []Step{
		{Number: 1, Before: missing, After: present},
		{Number: 2, Before: present, After: missing},
	}})
	for _, expected := range []string{
		"item_aliases.flashlight: added=\"\"",
		"object_items.desk: added=\"\"",
		"object_revealed_items.desk: added=\"\"",
		"known_exit_directions.reception: added=\"present\"",
		"observed_object_facts.desk: added=\"present\"",
		"item_aliases.flashlight: removed=\"\"",
		"object_items.desk: removed=\"\"",
		"object_revealed_items.desk: removed=\"\"",
		"known_exit_directions.reception: removed=\"present\"",
		"observed_object_facts.desk: removed=\"present\"",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("missing state-diff entry %q:\n%s", expected, got)
		}
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
