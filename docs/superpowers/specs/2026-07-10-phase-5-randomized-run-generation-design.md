# Phase 5 Randomized Run Generation Design

**Status:** Approved on 2026-07-10.

## Goal

Build deterministic, seeded item placement for the prototype while proving that every accepted run can reach the Emergency Stairwell through legal engine actions.

Phase 5 follows this rule:

```text
randomness proposes a world
the validator proves a path
the real resolver confirms the proof
```

## Scope

Phase 5 includes:

- Random placement of the flashlight across three safe Reception candidates.
- Random placement of the brass key across three post-flashlight candidates.
- Stable seed reproduction for a fixed scenario and generator version.
- Symbolic breadth-first search that returns a concrete witness path.
- Replay of that witness through `actions.Resolver` on a fresh world.
- CLI seed input and generated-seed output.
- Debug output for placements and validation.

Phase 5 does not include procedural rooms, hazards, monsters, alternate objectives, save/load, or response-generation changes. Its types must allow later phases to add content without rewriting the generator contract.

## Selected Approach

Use constructive placement plus a lock/key capability search and resolver replay.

Alternatives rejected:

- Unvalidated random placement can create softlocks.
- Searching only through full resolver states couples generation to runtime details and makes failures harder to inspect.
- A SAT/constraint-programming dependency is unnecessary for nine placement combinations.

The selected design matches common lock-and-key roguelike generation: build candidate content, derive progression capabilities from items and doors, prove a route, then reject any world whose proof cannot be executed by the authoritative engine.

## Package Boundaries

Create `internal/rungen` as the generic generation package. It may import `actions`, `game`, `intent`, and `world`; it must not import `scenario`.

`internal/scenario` owns dependency-free prototype content. `internal/runscenario` is the assembly adapter that imports both `scenario` and `rungen` and returns a `rungen.Definition`. Keeping this adapter above both packages prevents the `actions -> scenario -> rungen -> actions` test import cycle while leaving content decisions outside the generic algorithm.

Primary interfaces:

```go
type RunConfig struct {
	Seed             int64
	GeneratorVersion int
}

type Definition struct {
	ScenarioID      string
	ScenarioVersion int
	Build           func() *world.State
	StartRoom       game.RoomID
	WinRoom         game.RoomID
	LightItem       game.ItemID
	ItemRules       []ItemRule
}

type ItemRule struct {
	ItemID     game.ItemID
	Candidates []PlacementCandidate
}

type PlacementCandidate struct {
	ObjectID game.ObjectID
}

type Placement struct {
	ItemID   game.ItemID
	ObjectID game.ObjectID
}

type GeneratedRun struct {
	Seed             int64
	GeneratorVersion int
	ScenarioID       string
	ScenarioVersion  int
	State            *world.State
	Placements       []Placement
	Validation       ValidationResult
}
```

`Definition.Build` returns a new scenario template each time. The template contains rooms, doors, objects, and item definitions, but none of the required randomized items are placed.

`scenario.NewPrototypeWorld()` remains backward compatible for item placement by building the template and applying the current fixed flashlight/key locations. It also exposes the three new empty candidate objects, so the prototype object count increases from three to six. `runscenario.PrototypeDefinition()` supplies randomized rules to `rungen.Generate`.

## Determinism And Versioning

Candidate and item rule order must never depend on Go map iteration. Definitions are validated, copied, and sorted by stable IDs before random selection.

The generator builds the Cartesian product of candidate placements, sorts it, and applies one deterministic Fisher-Yates shuffle driven by a private SplitMix64 implementation. Bounded indexes use rejection sampling, not modulo-only reduction, so the shuffle has no modulo bias. This avoids depending on Go runtime RNG changes. The version-1 constants and output sequence are locked by golden-vector tests. The generator tests shuffled combinations in order and returns the first proven playable combination. A combination is never retried.

The Cartesian product is capped at 4,096 combinations. Larger definitions fail with a typed error instead of consuming unbounded memory.

Every run records:

- Seed.
- Generator version.
- Scenario ID.
- Scenario version.
- Final placements.

The initial generator version is `1`. Unsupported versions fail. Reproduction is guaranteed for the same seed, generator version, scenario version, and candidate data.

## Symbolic Playability Proof

The validator performs BFS over compact symbolic state:

- Current room.
- Discovered required items.
- Carried required items.
- Active light state.
- Unlocked relevant doors.

Item and door sets use deterministic indexes and `uint64` bitsets. Definitions with more than 64 required items or 64 relevant doors fail validation.

Legal symbolic transitions are derived from the built `world.State`, not duplicated prose rules:

- Move through a reachable exit.
- Search a visible searchable object and discover its contained required items.
- Take a discovered portable item.
- Turn on the configured light item when carried.
- Use a carried required key on its matching door.

Dark-room object searches require active light when the object or room visibility rules demand it. Locked doors require their declared key. The win condition is reaching `Definition.WinRoom`.

BFS stores predecessor steps and returns the shortest witness by symbolic action count. Search is capped at 10,000 states. Exceeding the cap returns a diagnostic error rather than accepting an unproven run.

```go
type WitnessStep struct {
	Intent          intent.Intent
	ExpectedOutcome string
}

type ValidationResult struct {
	Valid         bool
	Reason        string
	VisitedStates int
	Witness       []WitnessStep
}
```

## Resolver Replay

Symbolic proof is necessary but not authoritative. For every valid symbolic witness:

1. Build a fresh scenario template.
2. Apply the same placements.
3. Execute every witness intent through `actions.Resolver`.
4. Require each expected outcome.
5. Reject refusals, confirmation requests, clarification requests, and other unexpected outcomes.
6. Require the final room to equal `Definition.WinRoom`.

Replay uses the default Kaya state. This catches drift in visibility, discovery, inventory, door, timing, and autonomy rules.

The returned `GeneratedRun.State` is a separate fresh initial world. Validation and replay must not consume or mutate the player's starting state.

## Errors And Diagnostics

Generation fails closed. No unproven world may be returned.

Expose errors compatible with `errors.Is`:

- `ErrInvalidDefinition` for missing IDs, duplicate candidates, missing objects/items, non-searchable candidate objects, impossible bitset sizes, and invalid limits.
- `ErrUnsupportedVersion` for unknown generator versions.
- `ErrNoPlayableRun` when every candidate combination fails proof or replay.
- `ErrValidationLimit` when symbolic search exceeds its state cap.

`ErrNoPlayableRun` includes the number of attempted combinations and concise per-attempt reasons. Debug output contains seed, versions, placements, visited-state count, and witness length. It must not rely on LLM prose.

## CLI And Ollama

`kaya play --seed <int64>` runs the exact requested seed. Without `--seed`, the CLI reads eight bytes from `crypto/rand`, clears the sign bit, retries zero, prints the resulting positive seed before play, and uses it for generation.

Startup output includes:

```text
Run seed: 12345
Generator: 1
Flashlight: Reception Desk
Brass Key: Doctor Near Door
Validation: playable (9 witness steps)
```

The default local intent model changes from `mistral:latest` to the installed `qwen3.5:4b`. `KAYA_OLLAMA_MODEL` and `KAYA_OLLAMA_URL` remain authoritative overrides.

Ollama parses player language only. Generation, validation, replay, and outcomes remain deterministic and work without an LLM.

## Test Strategy

Implementation uses strict red-green-refactor cycles.

Required automated tests:

- Scenario template contains all candidate objects and no randomized required-item placements.
- Fixed `NewPrototypeWorld()` keeps the original flashlight/key locations while exposing all six candidate objects.
- Definition validation rejects malformed content with specific errors.
- Same seed and versions produce identical placements and witness.
- SplitMix64 golden vectors lock seed behavior across Go versions.
- Multiple seeds produce more than one placement combination.
- All nine flashlight/key combinations pass symbolic validation and resolver replay.
- Seeds `1..1000` all generate playable runs.
- Flashlight in dark storage is rejected.
- Flashlight behind the locked stairwell is rejected.
- Key behind the stairwell is rejected.
- Missing, non-searchable, and duplicate candidates are rejected.
- Door requiring an unavailable key is rejected.
- A witness blocked by Kaya autonomy is rejected during replay.
- Generated player state remains at the initial room with empty inventory and initial time.
- CLI accepts signed 64-bit seeds, rejects malformed values, and prints seed/placement diagnostics.
- Existing resolver, parser, clock, autonomy, and scenario tests remain green.

Final verification commands:

```text
go test ./...
go test -race ./...
go vet ./...
```

The gated Ollama integration suite is run separately with `KAYA_RUN_OLLAMA_TESTS=1` against `qwen3.5:4b`. LLM variance cannot gate deterministic generator correctness.

## Completion Criteria

Phase 5 is complete only when:

- Every accepted run has a BFS witness and successful resolver replay.
- All nine placement combinations are proven.
- The 1,000-seed sweep passes.
- Seed/version diagnostics reproduce a run.
- The CLI can complete a generated run with a fixed seed.
- Full tests, race tests, and vet pass.
- `docs/engine-milestones.md` records Phase 5 as complete for the first seeded, proven prototype slice.
