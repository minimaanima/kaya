# Phase 5 Playability Proof Plan

> For Phase 5, a generated run is not accepted because it "looks reasonable." It is accepted only if the engine can produce a deterministic witness path from the starting state to a win state.

## Goal

Build randomized run generation for Kaya where every accepted generated world is proven playable.

The first Phase 5 target is intentionally small:

- Randomize required item placement.
- Keep the room layout mostly fixed.
- Prove the flashlight can be found.
- Prove the key can be found after the flashlight.
- Prove the stairwell door can be unlocked.
- Prove the ending room can be reached.
- Store the seed and proof path for debugging.

## Source Conclusions

### Procedural Content Generation Survey

Source:

- [Procedural Content Generation in Games: A Survey with Insights on Emerging LLM Integration](https://arxiv.org/abs/2410.15644)

Relevant conclusion:

PCG systems are usually grouped into families such as search-based, machine-learning-based, noise-based, and hybrid methods. For Kaya, the important lesson is that LLMs are not the right owner of world truth. LLMs may help produce flavor, but the engine must evaluate validity.

Kaya decision:

```text
LLM may describe.
Engine must generate, validate, and prove.
```

### Search-Based PCG

Sources:

- [The Quest for Content: A Survey of Search-Based Procedural Content Generation for Video Games](https://arxiv.org/abs/2311.04710)
- [Procedural Content Generation through Quality Diversity](https://arxiv.org/abs/1907.04053)

Relevant conclusion:

Search-based PCG treats content generation as an optimization or search problem. A generated artifact is scored by a fitness/evaluation function.

Kaya decision:

We do not need evolutionary search yet. But we do need the core idea:

```text
candidate world -> evaluator -> accept/reject
```

For Phase 5, the evaluator is not a fuzzy score. It is a proof:

```text
valid = exists winning path
```

### Two-Step Dungeon Generation

Source:

- [Two-step Constructive Approaches for Dungeon Generation](https://arxiv.org/abs/1906.04660)

Relevant conclusion:

The paper separates dungeon generation into two phases:

```text
1. Layout creation
2. Furnishing with objects, enemies, treasures, start/goal
```

Kaya decision:

Use the same split:

```text
Layout: prototype lab rooms and exits
Furnishing: randomized item/object placement
```

Do not generate complex maps yet. Generate placements first.

### Locked-Door Mission Feasibility

Source:

- [Illuminating the Space of Dungeon Maps, Locked-door Missions and Enemy Placement Through MAP-Elites](https://arxiv.org/abs/2202.09301)

Relevant conclusion:

The paper encodes dungeons with a tree structure to preserve feasible missions, including locked-door missions. This matters because lock/key placement is not just a map problem; it is a dependency problem.

Kaya decision:

Represent puzzle progression as a mission graph:

```text
start -> flashlight -> light capability -> brass key -> stairwell door -> exit
```

The validator proves the mission graph is satisfiable in the generated world.

### Graph Grammars And Zelda-Style Dungeons

Source:

- [Generative Adversarial Network Rooms in Generative Graph Grammar Dungeons for The Legend of Zelda](https://arxiv.org/abs/2001.05065)

Relevant conclusion:

Dungeon generation can be represented as room graphs with structured connectivity and game progression. Graph grammar work is useful because it makes dependency and connectivity explicit.

Kaya decision:

Use graph reasoning directly:

```text
rooms = graph vertices
exits = graph edges
locks/items/capabilities = edge and action preconditions
```

### Game Pattern: Rogue / NetHack

Sources:

- [Rogue](https://en.wikipedia.org/wiki/Rogue_%28video_game%29)
- [NetHack](https://en.wikipedia.org/wiki/NetHack)

Relevant public description:

Traditional roguelikes use procedurally generated rooms, corridors, items, and stairs. NetHack levels contain generated rooms joined by corridors, and levels persist once generated.

Kaya decision:

The useful pattern is not "copy a dungeon grid." The useful pattern is:

```text
generate spatial structure
connect it
place objects
ensure progression route exists
persist the generated result
```

### Game Pattern: Spelunky

Source:

- [Spelunky](https://en.wikipedia.org/wiki/Spelunky)

Relevant public description:

Spelunky combines roguelike random generation with a real-time platformer. Public descriptions emphasize randomized levels, repeated play, and interactive/destructible terrain that gives players alternate ways through generated spaces.

Kaya decision:

Kaya should support multiple valid solutions later, but Phase 5 only needs one guaranteed route. The generator should eventually support optional alternate routes, but the validator must first prove the critical route.

### Game Pattern: Rooms And Mazes

Source:

- [Rooms and Mazes: A Procedural Dungeon Generator](https://journal.stuffwithstuff.com/2014/12/21/rooms-and-mazes/)

Relevant conclusion:

The article's useful idea is region connectivity. Generate parts of a dungeon, connect disconnected regions, then optionally remove dead ends.

Kaya decision:

For map generation later, use connected-region validation. For Phase 5 item placement, use capability reachability validation.

### Go Determinism

Sources:

- [Go math/rand](https://pkg.go.dev/math/rand)
- [Go spec: map iteration order](https://go.dev/ref/spec#For_statements)

Relevant conclusion:

`math/rand.New(rand.NewSource(seed))` gives local seeded randomness. But Go map iteration order is not specified.

Kaya decision:

Never build random candidate order from unsorted maps.

Wrong:

```go
for id := range state.Objects {
    candidates = append(candidates, id)
}
rng.Shuffle(len(candidates), ...)
```

Right:

```go
candidates := []game.ObjectID{
    scenario.ObjectReceptionDesk,
    scenario.ObjectReceptionFloor,
    scenario.ObjectCollapsedChair,
}
rng.Shuffle(len(candidates), func(i, j int) {
    candidates[i], candidates[j] = candidates[j], candidates[i]
})
```

Same seed must mean same world.

## Formal Model

### World Graph

Represent the world as a finite directed graph:

```text
G = (R, E)
```

Where:

```text
R = rooms
E = exits between rooms
```

For Kaya prototype:

```text
R = {reception, storage, stairwell}
E = {
    reception -> storage,
    storage -> reception,
    storage -> stairwell
}
```

Some edges have preconditions.

Example:

```text
storage -> stairwell requires brass_key unlocks stairwell_door
```

### Capabilities

A capability is a fact the engine can use for progression.

```text
C = finite set of capabilities
```

For first Phase 5:

```text
C = {
    has_flashlight,
    light_on,
    has_brass_key,
    stairwell_unlocked
}
```

Future capabilities:

```text
knows_admin_password
has_chemical_a
has_chemical_b
knows_formula
has_hazmat_mask
```

### Game State Node

The validator can model a simplified symbolic state:

```text
s = (room, capabilities)
```

For a stronger proof, include doors and inventory explicitly:

```text
s = (room, capabilities, door_states, inventory, active_light)
```

The runtime engine has more state than this, including time and Kaya emotion. Phase 5 should use two proof layers:

```text
1. Symbolic reachability proof
2. Engine replay proof
```

The symbolic proof finds a winning path. The engine replay proof runs that path through the actual deterministic resolver.

### Actions

Each action is a transition:

```text
a: s -> s'
```

Each action has preconditions and effects.

Examples:

```text
take_flashlight:
    precondition: room contains reachable flashlight
    effect: add has_flashlight

turn_on_flashlight:
    precondition: has_flashlight
    effect: add light_on

search_body_for_key:
    precondition: in storage and light_on
    effect: add has_brass_key

unlock_stairwell:
    precondition: has_brass_key
    effect: add stairwell_unlocked

move_to_stairwell:
    precondition: in storage and stairwell_unlocked
    effect: room = stairwell
```

### Win Condition

Define the win set:

```text
W = { s | s.room == stairwell }
```

A generated run is playable iff:

```text
exists path p = [s0, s1, ..., sn]
such that:
    s0 = initial state
    sn in W
    every transition si -> si+1 is legal
```

In plain words:

```text
There is at least one legal sequence of engine actions from start to ending.
```

## Proof Strategy

### Theorem

If the Phase 5 validator returns:

```go
ValidationResult{Valid: true, Witness: steps}
```

and the witness replay succeeds through the deterministic resolver, then the generated run has at least one valid ending path.

### Proof Sketch

1. The generator produces a finite world state.
2. The validator builds a finite search space from rooms, exits, doors, items, and capabilities.
3. The validator performs BFS/fixed-point reachability from the initial state.
4. If a win state is reached, BFS stores the predecessor path.
5. That predecessor chain becomes a witness path.
6. The witness path is replayed through the real resolver on a fresh copy of the generated world.
7. If replay reaches the win room, then a concrete legal game path exists.

Therefore:

```text
validator valid + resolver replay success => playable generated run
```

### False Positives And False Negatives

False positive:

```text
Validator accepts a world that the engine cannot actually finish.
```

This is dangerous and must be prevented by resolver replay.

False negative:

```text
Validator rejects a world that a clever player could finish.
```

This is acceptable in early Phase 5. It only reduces variety. It does not ship broken games.

Design rule:

```text
Prefer false negatives over false positives.
```

## Math Bounds

Let:

```text
|R| = number of rooms
|K| = number of boolean capabilities
|D| = number of relevant doors
|I| = number of required items
L = active light flag count = 2
```

Upper bound for symbolic state count:

```text
|S| <= |R| * 2^|K| * 2^|D| * 2^|I| * L
```

For current prototype:

```text
|R| = 3
|K| = 4
|D| = 1
|I| = 2
L = 2
```

So:

```text
|S| <= 3 * 2^4 * 2^1 * 2^2 * 2
|S| <= 3 * 16 * 2 * 4 * 2
|S| <= 768
```

That is tiny. BFS is safe.

If action count per state is bounded by `A`, BFS cost is:

```text
O(|S| * A)
```

For Phase 5, this is effectively instant.

### Candidate Combination Count

If there are:

```text
F = flashlight candidate count
K = key candidate count
```

Then required placement combinations are:

```text
F * K
```

If:

```text
F = 3
K = 3
```

Then:

```text
3 * 3 = 9 possible required-item worlds
```

This means we can exhaustively validate every required-item combination, not just test random seeds.

Seed sweeps are useful, but they are not mathematical proof over the whole placement space unless the candidate space is fully covered.

Recommended:

```text
Unit test all required placement combinations.
Seed-sweep generated outputs as a second safety check.
```

## Generator Algorithm

Use seeded constructive generation plus validation.

```text
input: RunConfig{Seed}
output: GeneratedRun or error

1. Build scenario template.
2. Clear randomized required-item placements.
3. Build sorted candidate lists.
4. Use seed to choose placements.
5. Apply placements to world.
6. Validate world.
7. Replay witness path through resolver.
8. Accept only if both validator and replay succeed.
```

Pseudocode:

```go
func Generate(config RunConfig) (GeneratedRun, error) {
    rng := rand.New(rand.NewSource(config.Seed))

    state := scenario.NewPrototypeTemplate()
    ClearRequiredPlacements(state)

    placements := []Placement{
        choosePlacement(rng, scenario.ItemFlashlight, FlashlightCandidates()),
        choosePlacement(rng, scenario.ItemBrassKey, KeyCandidates()),
    }

    ApplyPlacements(state, placements)

    validation := Validate(state)
    if !validation.Valid {
        return GeneratedRun{}, validation.AsError()
    }

    replay := ReplayWitness(state, validation.Witness)
    if !replay.Valid {
        return GeneratedRun{}, replay.AsError()
    }

    return GeneratedRun{
        Seed:       config.Seed,
        State:      state,
        Placements: placements,
        Validation: validation,
    }, nil
}
```

## Validator Algorithm

Use BFS over symbolic states.

```text
queue = [initial_state]
visited = {initial_state}
parent = map[state]previous_step

while queue not empty:
    current = pop_front(queue)

    if current is win:
        return witness path

    for action in legalActions(current, world):
        next = apply(action, current)
        if next not visited:
            visited.add(next)
            parent[next] = (current, action)
            push_back(next)

return invalid
```

Why BFS:

- It is complete over a finite graph.
- It gives a shortest witness path by number of symbolic actions.
- It is easy to debug.
- It does not need heuristics.

## Witness Replay

The symbolic validator is not enough by itself because the real engine has extra rules:

- Kaya autonomy.
- Time.
- Visibility.
- Item discovery.
- Door states.
- Inventory.
- Resolver target matching.

So every validation witness must be replayed through the real resolver.

Example witness:

```text
inspect room
search reception desk
take flashlight
turn on flashlight
move east
search doctor near cabinet
take brass key
use brass key on stairwell door
move north
```

Replay uses structured intents, not LLM parsing:

```go
[]intent.Intent{
    {Action: intent.ActionInspect},
    {Action: intent.ActionSearch, Target: "reception desk"},
    {Action: intent.ActionTakeItem, Target: "flashlight"},
    {Action: intent.ActionTurnOn, Target: "flashlight"},
    {Action: intent.ActionMove, Direction: "east"},
    {Action: intent.ActionSearch, Target: "doctor near cabinet"},
    {Action: intent.ActionTakeItem, Target: "brass key"},
    {Action: intent.ActionUseItem, Item: "brass key", Target: "stairwell door"},
    {Action: intent.ActionMove, Direction: "north"},
}
```

Replay succeeds iff:

```text
state.CurrentRoomID == scenario.RoomStairwell
```

## Kaya Autonomy In The Proof

Phase 4 added stress, trust, fear, refusal, and confirmation.

That means playability cannot ignore Kaya.

A generated run is not truly playable if the only path requires Kaya to do something she deterministically refuses.

For first Phase 5:

```text
Validator checks default Kaya state.
Replay checks actual resolver autonomy.
```

If replay returns:

```text
kaya_refused
kaya_needs_confirmation
```

then the proof path is not directly executable.

Policy:

```text
kaya_refused => generated run invalid
kaya_needs_confirmation => generated run valid only if confirmation flow exists
```

Since we do not yet have a confirmation memory system, first Phase 5 should treat confirmation as invalid for the required proof path.

Later, when confirmation is implemented:

```text
kaya_needs_confirmation -> player confirms -> action continues
```

Then confirmation can become part of the witness path.

## Required Data Model

Package:

```text
internal/rungen
```

Files:

```text
config.go
candidate.go
placement.go
generator.go
validator.go
witness.go
replay.go
debug.go
generator_test.go
validator_test.go
replay_test.go
```

Types:

```go
type RunConfig struct {
    Seed int64
}

type GeneratedRun struct {
    Seed       int64
    State      *world.State
    Placements []Placement
    Validation ValidationResult
}

type Placement struct {
    ItemID   game.ItemID
    ObjectID game.ObjectID
}

type Candidate struct {
    ItemID       game.ItemID
    ObjectID     game.ObjectID
    Requires     []Capability
    Weight       int
}

type Capability string

type ValidationResult struct {
    Valid   bool
    Reason  string
    Witness []WitnessStep
}

type WitnessStep struct {
    Action intent.Action
    Target string
    Item   string
    Room   game.RoomID
    Grants []Capability
}
```

Capabilities:

```go
const (
    CapabilityFlashlight        Capability = "has_flashlight"
    CapabilityLight             Capability = "light_on"
    CapabilityBrassKey          Capability = "has_brass_key"
    CapabilityStairwellUnlocked Capability = "stairwell_unlocked"
)
```

## First Candidate Set

Add candidate objects:

```text
Reception Desk
Reception Floor
Collapsed Chair
Doctor Near Cabinet
Doctor Near Door
Storage Cabinet
```

Flashlight:

```text
Reception Desk
Reception Floor
Collapsed Chair
```

Brass key:

```text
Doctor Near Cabinet
Doctor Near Door
Storage Cabinet
```

Invalid:

```text
Flashlight in storage darkness
Flashlight behind stairwell door
Brass key in stairwell
Brass key in non-searchable object
Required item in object that is not targetable
```

## Implementation Plan

### Task 1: Scenario Template Split

Files:

- Modify: `internal/scenario/prototype.go`
- Add tests: `internal/scenario/prototype_test.go`

Goal:

Create a prototype template where required item placement can be cleared and reapplied.

Steps:

- [ ] Add extra searchable placement objects.
- [ ] Add `NewPrototypeTemplate()` or `NewPrototypeWorldWithPlacements(placements []rungen.Placement)`.
- [ ] Keep `NewPrototypeWorld()` as the default deterministic world.
- [ ] Test that default world still has flashlight and key in current places.
- [ ] Test that template can clear required item placements.

### Task 2: Rungen Types

Files:

- Create: `internal/rungen/config.go`
- Create: `internal/rungen/candidate.go`
- Create: `internal/rungen/placement.go`

Goal:

Add typed generation inputs and outputs without generator behavior yet.

Steps:

- [ ] Add `RunConfig`.
- [ ] Add `Placement`.
- [ ] Add `Candidate`.
- [ ] Add `Capability`.
- [ ] Add sorted candidate lists for flashlight and key.
- [ ] Test candidate order is stable.

### Task 3: Deterministic Placement

Files:

- Create: `internal/rungen/generator.go`
- Add tests: `internal/rungen/generator_test.go`

Goal:

Same seed gives same placements; different seeds can vary placements.

Steps:

- [ ] Write failing test for same seed reproducibility.
- [ ] Write failing test for candidate order not depending on maps.
- [ ] Implement local `rand.New(rand.NewSource(seed))`.
- [ ] Implement candidate choice.
- [ ] Implement placement application.

### Task 4: Symbolic Validator

Files:

- Create: `internal/rungen/validator.go`
- Add tests: `internal/rungen/validator_test.go`

Goal:

Prove structural playability by BFS/capability reachability.

Steps:

- [ ] Write failing test: valid default world returns witness path.
- [ ] Write failing test: flashlight behind darkness is invalid.
- [ ] Write failing test: key behind stairwell is invalid.
- [ ] Implement symbolic state.
- [ ] Implement legal actions.
- [ ] Implement BFS.
- [ ] Return witness path.

### Task 5: Resolver Replay

Files:

- Create: `internal/rungen/replay.go`
- Add tests: `internal/rungen/replay_test.go`

Goal:

Prove witness path works in the real engine.

Steps:

- [ ] Write failing test: default witness reaches stairwell.
- [ ] Write failing test: witness fails if Kaya refuses.
- [ ] Convert witness steps to structured intents.
- [ ] Run `actions.NewResolver(state).Resolve(intent)` for each step.
- [ ] Require final room to be `scenario.RoomStairwell`.

### Task 6: Accepted Generated Run

Files:

- Modify: `internal/rungen/generator.go`
- Add tests: `internal/rungen/generator_test.go`

Goal:

`Generate(seed)` returns only validated and replayed runs.

Steps:

- [ ] Write failing test: generated run contains valid validation result.
- [ ] Write failing test: generated run includes placements.
- [ ] Write seed sweep test for seeds `1..1000`.
- [ ] Implement generate -> validate -> replay -> accept.
- [ ] Return detailed error if validation fails.

### Task 7: CLI And Playtests

Files:

- Modify: `cmd/kaya/main.go`
- Add tests: `cmd/kaya/main_test.go`

Goal:

Expose seeds for manual play and automated playtest.

Steps:

- [ ] Add `kaya play --seed <seed>`.
- [ ] Print/log seed and placements.
- [ ] Add playtest scripts for fixed seeds.
- [ ] Add playtest expectation that each fixed seed can reach the stairwell.

## Test Strategy

### Exhaustive Required-Placement Tests

For first Phase 5:

```text
flashlight candidates = 3
key candidates = 3
combinations = 9
```

Test all 9.

This proves the required placement space, not just sample seeds.

### Seed Sweep

Run:

```text
for seed in 1..1000:
    run = Generate(seed)
    assert run.Validation.Valid
    assert ReplayWitness(run.State, run.Validation.Witness).Valid
```

This catches RNG and wiring errors.

### Negative Tests

Force invalid worlds:

```text
flashlight in dark storage
flashlight behind stairwell door
key in stairwell
key in missing object
key in non-searchable object
door requires nonexistent key
Kaya refuses required path
```

Each must fail with a specific reason.

## Final Conclusion

Phase 5 should not be "random item placement."

It should be:

```text
seeded constructive generation
+ mission graph/capability solver
+ BFS witness proof
+ deterministic resolver replay
```

This gives the engine a real proof:

```text
There exists at least one valid ending path in this generated run.
```

That proof is the difference between:

```text
random story toy
```

and:

```text
survival horror game engine
```

The safe first implementation is small, mathematically bounded, and directly useful for later research puzzles, password puzzles, chemical puzzles, hazards, and larger lab layouts.
