# Engine Milestones

This document defines the build phases for the Dr. Kaya engine. Each milestone should become its own execution plan before implementation starts.

The main rule is to keep the deterministic game engine ahead of the LLM layer. The engine must own world truth, validation, timing, state changes, and outcomes.

## Phase 0 - Project Skeleton

Goal:

Establish the Go project structure and core domain types.

Status:

Complete.

Scope:

- Go module.
- Package layout.
- Core IDs.
- Intent structures.
- World structures.
- Item interface and initial items.
- Kaya state structures.
- Prompt documentation.

Exit criteria:

- `go test ./...` compiles.
- Packages have clear ownership.
- No generic `types`, `common`, or `util` package is required.

## Phase 1 - Deterministic World State

Goal:

Create a playable in-memory world model without LLM integration.

Status:

Complete.

Scope:

- World state container.
- Room registry.
- Door registry.
- Object registry.
- Item registry.
- Inventory.
- Current room tracking.
- Basic visibility checks.
- Basic world query functions.
- Object and door aliases for natural target matching.
- Ambiguous target detection.

Required behavior:

- Get current room.
- List visible objects.
- List available exits.
- Check whether a room requires light.
- Check whether an item is in inventory.
- Check whether an object can be searched.
- Resolve a target phrase like "doctor", "body", or "stairwell door" to matching world objects.
- Report ambiguity when a target phrase matches multiple valid objects.

Exit criteria:

- A small hardcoded world can be loaded.
- Tests prove dark-room visibility works.
- Tests prove inventory and object lookup work.
- Tests prove ambiguous targets are detected without guessing.

## Phase 2 - Action Resolver

Goal:

Validate and execute structured intents against world state.

Status:

Complete for the first prototype slice.

Scope:

- `move`
- `inspect`
- `search`
- `take_item`
- `use_item`
- `turn_on`
- `turn_off`
- `listen`
- `wait`

Required behavior:

- Invalid actions return useful engine results.
- Valid actions mutate state when appropriate.
- Item use goes through item behavior.
- The resolver does not call the LLM.

Exit criteria:

- Player can solve a simple key-and-door puzzle through structured intents.
- Tests cover valid and invalid actions.
- Action results contain facts suitable for Kaya response generation.
- `kaya play` can run a manual console prototype using Ollama intent parsing and deterministic action resolution.

## Phase 3 - Time System

Goal:

Make actions consume time and allow world events to happen during that time.

Status:

Complete for deterministic scheduled events.

Scope:

- Game clock.
- Action durations.
- Time advancement.
- Scheduled events.
- Random event hooks.
- Delayed response metadata.

Required behavior:

- Moving takes time.
- Searching takes time.
- Waiting advances time.
- Events can trigger during elapsed time.
- Action results report duration.

Exit criteria:

- Tests prove action durations advance the clock.
- Tests prove scheduled events fire at the right time.
- Engine can explain what happened during an action.
- Console play shows action time cost and fired world events.

## Phase 4 - Kaya Autonomy

Goal:

Make Dr. Kaya's state affect whether and how actions are executed.

Status:

Complete for the first deterministic autonomy slice.

Scope:

- Stress.
- Trust.
- Fear.
- Pain.
- Exhaustion.
- Refusal rules.
- Confirmation requests for dangerous actions.
- State changes from action results.

Required behavior:

- Kaya can refuse or hesitate.
- Dangerous actions can require confirmation.
- Stress and trust affect willingness.
- Engine distinguishes impossible actions from refused actions.

Exit criteria:

- Tests prove high stress can block risky actions.
- Tests prove high trust can allow some risky actions.
- Tests prove emotional state updates after danger, injury, or success.

## Phase 5 - Randomized Run Generation

Goal:

Add roguelike variation while keeping puzzles valid.

Status:

Complete for the first seeded, playability-proven prototype slice. Unit tests, race tests, vet, exhaustive placement proof, the 1,000-seed sweep, Qwen intent integration, and a full fixed-seed CLI playthrough pass.

Scope:

- Seeded run generation.
- Item placement rules.
- Hazard placement rules.
- Locked-door rules.
- Required-path validation.
- Reproducible debug seeds.

Required behavior:

- Same seed produces same world.
- Different seeds can move key items.
- Required items remain reachable.
- Early-game softlocks are prevented.

Exit criteria:

- Tests prove seed reproducibility.
- Tests prove required key placement is always valid.
- Minimal prototype can randomize the key location.

## Phase 6 - Intent Parser Integration

Phase 6 status: In progress; deterministic coverage is complete, but the Qwen gate and full verification remain pending.

Goal:

Connect free-form player messages to structured intents.

Scope:

- Parser interface.
- Simple rule-based parser for tests and fallback.
- LLM parser adapter.
- JSON validation.
- Repair prompt path.
- Confidence and clarification handling.

Required behavior:

- Parser returns `intent.Intent`.
- Invalid LLM JSON can be repaired or rejected.
- Low-confidence messages can ask for clarification.
- Engine never trusts parser output without validation.

Exit criteria:

- Tests cover parser schema validation.
- Tests cover ambiguous input.
- A free-form message can drive the simple prototype.
- Gated Ollama integration tests cover natural player phrasing such as "look around", "what's in the room", and "can you check the doctor's coat".

## Phase 7 - Kaya Response Generation

Phase 7 status: In progress; the response gate passes, but the milestone remains pending until the full verification set passes.

Goal:

Convert engine facts into Dr. Kaya's chat response.

Scope:

- Response input schema.
- Deterministic fallback renderer.
- LLM voice adapter.
- Emotion-aware style constraints.
- Fact-locking rules.

Required behavior:

- Response generator receives only engine-approved facts.
- LLM must not invent items, rooms, exits, monsters, or outcomes.
- Kaya's voice reflects stress, fear, trust, and injury.
- Fallback renderer works without an LLM.

Exit criteria:

- Tests prove fallback responses include required facts.
- Prompt docs define what the LLM may and may not do.
- Basic messaging loop can show Kaya-style responses.

## Phase 8 - First Playable Scenario

Goal:

Build the first complete playable slice.

Scope:

- Reception.
- Main Corridor.
- Storage Room.
- Emergency Stairwell door.
- Flashlight.
- Key.
- Searchable table.
- Searchable body.
- Random key placement.
- Locked-door escape objective.

Required behavior:

- Player can communicate in free-form text.
- Kaya can move, inspect, search, take items, use flashlight, and unlock the door.
- Darkness affects what can be found.
- Time advances.
- Kaya's stress changes.
- The run can be won.

Exit criteria:

- One full playthrough works from start to escape.
- At least one randomized variation works.
- Tests cover the main puzzle path.

## Phase 9 - Research Puzzle Framework

Goal:

Support puzzles that require player-side reasoning or external/in-game document research.

Scope:

- Puzzle state model.
- Document/clue model.
- Terminal/account credential checks.
- Procedure validation.
- Multi-step puzzle validation.
- Consequence handling for wrong answers.

Required behavior:

- Kaya can report exact document contents.
- Player can use discovered information later.
- Engine validates passwords, formulas, and procedures.
- Wrong answers can waste time, increase risk, injure Kaya, or change world state.

Exit criteria:

- One administrator-password puzzle works.
- One chemical/procedure puzzle works in deterministic form.
- Tests cover correct, incorrect, and dangerous solutions.

## Phase 10 - Persistence and Debugging

Goal:

Make runs inspectable, reproducible, and saveable.

Scope:

- Save/load.
- Run seed.
- Event log.
- Action log.
- Parser log.
- Debug command output.
- State snapshot tests.

Required behavior:

- A run can be saved and resumed.
- A bug can be reproduced from seed and action log.
- Engine decisions can be inspected without reading prose.

Exit criteria:

- Save/load round trip test passes.
- Seed plus action log can replay a run.
- Debug output shows room, inventory, time, Kaya state, and active flags.

## Phase 11 - Content Expansion

Goal:

Expand from prototype into a larger horror scenario.

Scope:

- More rooms.
- More items.
- More hazards.
- More clue chains.
- More randomized events.
- Monster or creature systems.
- Secret experiment reveal structure.

Required behavior:

- New content is data-driven where practical.
- New puzzles use existing engine systems.
- Story reveals are controlled by engine state.

Exit criteria:

- Content can be added without rewriting core resolver logic.
- At least one larger route has multiple solutions.
- Failure states are clear and testable.

## Planning Rule

Before starting any phase, create a short execution plan for that phase:

- Objective.
- Files/packages to touch.
- Data structures needed.
- Tests needed.
- Manual verification path.
- Known risks or design choices.

Implementation should stay inside the current phase unless a dependency is blocking progress.
