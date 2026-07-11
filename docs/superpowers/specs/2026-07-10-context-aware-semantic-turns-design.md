# Context-Aware Semantic Turns Design

**Status:** Approved on 2026-07-10.

## Goal

Make free-form play behave like a grounded conversation without giving the LLM authority over the game world.

The target flow is:

```text
player message
  -> perceived-world snapshot
  -> LLM turn plan
  -> deterministic target resolution and action execution
  -> engine-approved fact bundle
  -> LLM Kaya response
  -> validated response or deterministic fallback
```

This is the first combined Phase 6 and Phase 7 slice. It also corrects a Phase 1 perception leak: pitch-black rooms currently expose every exit.

## Player Experience

The slice must support these examples:

- In darkness, Kaya reports only exits she already knows. Entering a dark room remembers the route back, but does not reveal other exits.
- Turning on the flashlight reveals the room's exits and visible objects.
- `feel along the walls for another exit` spends 30 seconds and discovers one previously unknown exit.
- After Kaya reports two doctors, `search them` means both doctors.
- `inspect the doctor` is ambiguous when both doctors are valid targets, so Kaya asks which doctor.
- `search the doctors -- are they dead?` searches each doctor in order, charges time and autonomy separately, and answers the life-status question using engine facts.
- If the first target succeeds and the second is refused or fails, the first result remains applied.
- Parser or response-model failure produces a useful deterministic response, never `I hear you.` as a generic escape.

## Selected Approach

Use a hybrid semantic turn. The LLM interprets language and writes Kaya's phrasing. The deterministic engine resolves IDs, checks perception, mutates state, advances time, applies autonomy, and approves every factual claim.

Alternatives rejected:

- Adding more isolated intent-prompt examples would not solve pronouns, plural targets, compound requests, or hidden-world leakage.
- Giving the LLM the full world and letting it choose outcomes would expose secrets and make runs impossible to reproduce.
- A purely rule-based conversational parser would be deterministic but brittle across natural player wording.

## Package Boundaries

Keep each responsibility independent:

- `internal/intent` owns `TurnPlan`, parser schemas, validation, and the deterministic parser fallback.
- `internal/world` owns perception truth, known exits, visible target resolution, and recent perceived referents.
- `internal/actions` keeps the existing authoritative resolver for one physical action.
- `internal/turn` resolves target groups, executes individual actions, answers fact questions, and builds approved fact bundles.
- `internal/response` owns deterministic rendering, the Ollama voice adapter, and output validation.
- `cmd/kaya` orchestrates one turn but contains no world, parsing, or response rules.

The existing single-`Intent` resolver remains the authoritative unit for one physical action. A turn executor sits above it; it does not duplicate movement, time, item, event, or autonomy logic.

## Perceived-World Snapshot

The parser must never receive the complete `world.State`. A read-only snapshot exposes only what Kaya can currently know:

```go
type PerceptionSnapshot struct {
	RoomName        string
	HasUsefulLight  bool
	VisibleObjects  []PerceivedObject
	KnownExits      []PerceivedExit
	Inventory       []PerceivedItem
	RecentReferents []ReferentGroup
}
```

Display names and safe aliases may be included. Hidden objects, undiscovered items, unknown exits, locked-door internals, future events, and random placements must be excluded.

Snapshot slices use stable authored order. No player-facing or prompt-facing order may depend on Go map iteration.

## Known Exits And Darkness

`world.State` records known exits per room and direction. Knowledge belongs to the current run and survives leaving and returning to a room.

Rules:

1. Entering a room marks the reverse route as known, because Kaya just used it.
2. When the room has enough light, its authored exits become known.
3. In darkness, inspect and movement choices include only known exits.
4. Guessing an unknown direction in darkness does not move Kaya. The result explains that she cannot safely find that route.
5. Turning on a usable flashlight immediately makes the current room perceivable; the next observation reports all exits and visible objects.
6. Every explicit tactile exploration attempt costs 30 seconds and triggers normal clock events and autonomy processing. A successful attempt reveals the first unknown exit in authored room order.
7. If no exit remains unknown, the attempt still consumes its time and reports that without inventing a route.

Add `intent.ActionExplore` for explicit phrases such as `feel along the walls`, `find another exit`, and `search for a way out`. Ordinary object `search` remains unchanged.

The resolver and parser receive world queries such as `KnownExits`, `CanUseExit`, and `DiscoverExit`; callers do not read or mutate the knowledge map directly.

## Recent Referents

Conversation memory is small, engine-owned, and perception-safe. A `ReferentGroup` contains stable object or item IDs that Kaya actually perceived, plus whether the group was singular or plural.

The state retains the three most recent groups. New successful observations and direct target actions push a group; duplicates are coalesced. IDs that are no longer valid or perceivable are removed when building a snapshot.

Resolution rules:

- `it` and `that` use the latest valid singular group.
- `they`, `them`, `those`, `both`, and an explicit plural noun use the latest compatible plural group.
- An explicit target name takes precedence over pronoun memory.
- A singular noun matching multiple visible objects is ambiguous and requires clarification.
- A plural target intentionally selects all matching visible objects.
- No pronoun may resolve to an object the player has not perceived.

Object groups preserve authored room order so repeated seeds and replays execute targets identically.

## Turn Plan

The intent parser returns a complete semantic plan instead of one isolated action:

```go
type TurnPlan struct {
	Actions               []PlannedAction `json:"actions"`
	Questions             []FactQuestion  `json:"questions"`
	Confidence            float64         `json:"confidence"`
	NeedsClarification    bool            `json:"needsClarification"`
	ClarificationQuestion string          `json:"clarificationQuestion"`
	RawText               string          `json:"rawText"`
}

type PlannedAction struct {
	Intent     Intent     `json:"intent"`
	TargetMode TargetMode `json:"targetMode"`
}

type FactQuestion struct {
	Kind       FactKind   `json:"kind"`
	Target     string     `json:"target"`
	TargetMode TargetMode `json:"targetMode"`
}
```

`TargetMode` is `single` or `all`. The LLM provides player wording, not engine IDs. The engine resolves targets against the snapshot and recent referents.

The first slice accepts at most four planned actions and four questions per player message. Larger plans are rejected with clarification instead of consuming unbounded time. Empty plans, invalid actions, invalid target modes, low confidence, and unresolved references also request clarification.

The Ollama adapter uses JSON-schema-constrained output with thinking disabled. One repair request is allowed for malformed output. If repair fails or Ollama is unavailable, the deterministic fallback handles common movement, inspect, search, take, light, wait, and explore phrases. If it cannot safely interpret the message, it asks a concrete clarification question.

## Sequential Execution

The turn executor validates the entire structural plan, then processes physical actions in player order. A plural action is expanded into individual existing `intent.Intent` values in authored target order.

Each individual action independently performs:

- Perception and target validation.
- Autonomy/refusal checks.
- World mutation.
- Time advancement.
- Scheduled-event processing.
- Kaya-state changes.

Execution stops on ambiguity, required confirmation, refusal, impossible action, or missing target. Completed actions are not rolled back. The result contains an ordered outcome for every attempted target and a clear stop reason.

Questions do not mutate state or consume additional time. After physical execution stops, the fact-query layer answers only what is currently permitted by perception and completed observations. For a compound request, it attaches facts to the corresponding target outcomes. Facts that Kaya could not establish are returned as `unknown`, not guessed.

## Engine Facts

`internal/turn` builds a `FactBundle` for response generation. The response layer never receives `world.State`:

```go
type Fact struct {
	ID       FactID
	Kind     FactKind
	Subject  string
	Value    string
	Required bool
}

type FactBundle struct {
	Outcomes []ActionOutcome
	Facts    []Fact
	Emotion  EmotionSnapshot
}
```

Facts cover approved categories such as room description, visible objects, known exits, item discovery, movement, action failure, life status, elapsed time, fired events, and Kaya emotion. Scenario objects store observable facts as an authored ordered slice containing a fact kind, value, and the observation actions that may reveal it. For example, a doctor's life status can require inspect or search. This avoids untyped prose checks and map-order instability.

Fact IDs are unique within a turn and stable for the bundle. Required facts include state changes, failures, clarification, elapsed time, and dangerous events that the response must not omit.

## Kaya Response Generation

The Ollama voice adapter receives only:

- The player's original message.
- Ordered action outcomes.
- Approved facts.
- Kaya's current emotion snapshot.
- Short voice constraints.

It does not receive rooms, objects, exits, items, or events outside the fact bundle.

The adapter returns structured sentences:

```go
type ResponseDraft struct {
	Sentences []DraftSentence `json:"sentences"`
}

type DraftSentence struct {
	FactIDs []FactID `json:"factIds"`
	Text    string   `json:"text"`
}
```

Every factual sentence must cite at least one approved fact ID. Validation rejects:

- Unknown fact IDs.
- Missing required fact IDs.
- Empty or excessively long text.
- New named world entities not present in the bundle.
- Invalid JSON or schema violations.

This boundary makes hallucination detectable for IDs and entities while keeping the engine authoritative. It cannot prove every nuance of free prose, so prompts require concise paraphrase and tests aggressively inject unsupported claims. Any validation failure discards the entire draft and uses the deterministic renderer.

The deterministic renderer preserves action order, includes every required fact once, groups repeated target outcomes cleanly, and uses short Kaya-style lines. It is also used when Ollama times out or is unavailable. The response layer never changes world state.

## Error Handling

- Parser timeout, transport failure, malformed JSON, or failed repair: use deterministic parser fallback.
- Unsafe fallback interpretation: ask a specific clarification question.
- Ambiguous singular target: list the visible matching names and ask which one.
- Missing or hidden target: say Kaya cannot perceive it here.
- Partial multi-target failure: report completed outcomes, then the stopping reason.
- Fact query without sufficient observation: report that Kaya cannot tell yet.
- Response timeout, malformed output, unsupported fact ID, omitted required fact, or unknown entity: render deterministic fallback.
- Internal invariant failure: fail the turn without mutation when detected before execution; preserve already committed individual actions when detected later and report a safe engine error.

Logs record parser plans, resolved IDs, action outcomes, approved fact IDs, response validation failures, model name, and durations. Logs must not silently expose hidden world data in player output.

## Test Strategy

Implementation follows red-green-refactor. Deterministic unit and integration tests are the main gate; Ollama tests remain separately gated.

Required tests:

- Entering a dark room remembers only the reverse exit.
- Inspecting darkness does not leak unknown exits or objects.
- Guessing an unknown dark exit cannot move Kaya.
- Tactile exploration discovers one authored exit, costs 30 seconds, and fires due events.
- Tactile exploration with no unknown exits reports that state.
- Turning on the flashlight reveals current-room exits and visible objects.
- Known exits survive leaving and returning.
- Snapshots exclude hidden objects, unknown exits, undiscovered items, placements, and future events.
- Singular ambiguous targets request clarification.
- Explicit plural and plural pronouns resolve every matching perceived object in authored order.
- Singular and plural pronouns use recent valid groups and never hidden targets.
- Recent referent memory is bounded and drops invalid groups.
- Compound action plus question produces one plan and one coherent fact bundle.
- Two-target search charges time and autonomy twice.
- Failure on the second target preserves the first target's mutation and elapsed time.
- Questions expose typed observable facts only after sufficient observation.
- Parser schema validation rejects invalid and oversized plans.
- Parser repair is attempted once; deterministic fallback and clarification paths work without Ollama.
- Response input contains no hidden world state.
- Response validation rejects unknown IDs, missing required facts, unknown entities, invalid JSON, and overlong output.
- Deterministic response fallback preserves outcome order and all required facts.
- Existing Phase 0-5 tests and generator witness replay remain green after perception changes.
- Gated Qwen tests cover the user's exact phrases, common typos, pronouns, plural doctors, compound questions, and darkness exploration.
- A fixed-seed CLI playthrough reaches the stairwell using natural language.

Final verification:

```text
go test ./...
go test -race ./...
go vet ./...
KAYA_RUN_OLLAMA_TESTS=1 go test ./internal/intent ./internal/response
go run ./cmd/kaya play --seed 12345
```

## Completion Criteria

This slice is complete when:

- Darkness never reveals unknown exits or objects.
- Kaya can retrace her route or deliberately discover a dark exit.
- Natural plural, pronoun, and compound requests execute predictably.
- Each physical target receives its own deterministic time, event, and autonomy processing.
- The response LLM receives and cites only engine-approved turn facts, its mechanically verifiable output constraints are enforced, and every rejection path has a tested deterministic fallback.
- The reported manual examples behave as designed with `qwen3.5:4b`.
- Unit tests, race tests, vet, gated Ollama tests, generator replay, and a fixed-seed playthrough pass.
