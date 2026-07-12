# Prototype Vertical Slice Hardening Design

## Purpose

Harden the complete Reception to Storage Room to Emergency Stairwell escape
loop until it is a dependable playable vertical slice.

This work exercises the engine as a player experiences it: natural text enters
the parser, structured plans execute against deterministic world state, approved
facts become Kaya's response, time advances, events fire, and the run reaches a
terminal objective.

The goal is not arbitrary-language perfection. The goal is that every failure
inside this prototype is observable, reproducible, classified, and protected by
a regression test.

## Slice Boundary

Included:

- Reception.
- Storage Room.
- Emergency Stairwell.
- Flashlight discovery, taking, activation, and darkness behavior.
- Brass key discovery, taking, and door use.
- All nine valid flashlight/key placement combinations.
- Free-form intent parsing and deterministic fallback.
- Singular and plural target resolution.
- Compound commands.
- Time advancement and scheduled events.
- Kaya autonomy decisions already supported by the prototype.
- Fact-locked first-person responses.
- Objective completion.

Excluded:

- New rooms, items, puzzles, monsters, or endings.
- Research puzzles.
- Save/load.
- Cloud inference and model deployment.
- Model-specific prompt optimization.
- New autonomy mechanics beyond fixing incorrect existing behavior.
- Content expansion or atmospheric rewriting unrelated to a reproduced defect.

## Testing Architecture

### Stateful Session Runner

Add a reusable black-box playtest runner that drives the same parser, turn
executor, response composer, and generated run used by console play.

Each session records:

- Scenario and generator versions.
- Run seed and item placements.
- Player message.
- Parsed raw and resolved plans with provenance.
- State summary before the turn.
- Ordered action outcomes.
- Elapsed time and fired events.
- State summary after the turn.
- Kaya's final response.
- Stop reason and objective state.

The runner returns structured records to tests and can render a readable
Markdown transcript when a session fails.

### Phrase Banks

Required gameplay operations have reviewed natural-language variants:

- Room awareness.
- Object inspection.
- Searching.
- Inventory questions.
- Taking items.
- Activating the flashlight.
- Moving by direction and returning.
- Using the key on the door.
- Clarifying ambiguous doctors.
- Selecting both remembered targets.
- Ordered two-action commands.

Phrase banks include polite wording, terse commands, spelling mistakes observed
in playtests, and common conversational variants. They are semantic test data,
not prompt examples.

### Deterministic Session Generation

Generate reproducible session combinations from:

- All valid item placements.
- Phrase variants for each required operation.
- Single and compound command forms.
- Repeated and interrupted actions.
- Valid and invalid target choices.
- Selected autonomy states.

Use local seeded randomness only to choose among reviewed phrases and optional
interruptions. A failed session reports the seed and full transcript.

Run at least 1,000 sessions in the ordinary test suite without Ollama. The
session generator must remain fast enough for normal development.

### Live Ollama Sessions

Keep live model playthroughs opt-in. Run a small fixed set of complete
playthroughs through the configured Ollama model using the same session runner.

Live sessions report raw model plans separately from resolved plans. Generator,
repair, and decoding fallbacks remain visible. The deterministic engine remains
the authority and live tests do not change expected outcomes to accommodate a
model.

## Required State Invariants

Check after every turn:

- The current room exists.
- The previous room, when set, exists.
- Game time never decreases.
- Time advances only by the sum of executed action durations.
- Inventory contains only portable items.
- An item appears in at most one location: inventory or one container.
- A taken item is removed from its container.
- An undiscovered contained item cannot be taken by name.
- A refused action does not mutate world discovery, inventory, doors, or room.
- Pitch-black rooms reveal no light-dependent objects without active light.
- Turning on a carried flashlight enables the expected perception.
- Turning off the flashlight restores darkness restrictions.
- Searching an object again cannot rediscover an already taken item.
- Locked-door movement cannot change the current room.
- Unlocking with the correct carried key changes only the intended door.
- Scheduled events fire once and in chronological order.
- Objective completion occurs only after entering the stairwell.
- Objective completion is emitted at most once.

Invariant failures stop the session immediately and include a before/after state
diff.

## Conversation and Response Invariants

- Kaya speaks in first person.
- Responses never begin with third-person forms such as "Kaya cannot".
- Every factual claim is supported by engine-approved facts.
- Darkness responses do not list hidden objects or inaccessible exits.
- Inventory questions report current carried items without executing an action
that consumes game time.
- Clarification questions do not mutate state or advance time.
- Ambiguous targets name only currently resolvable candidates.
- Repeated queries remain coherent with prior discoveries.
- Responses preserve compound action order.
- Debug and parser diagnostics appear only when explicitly enabled.

Automated checks should focus on structural and factual guarantees. Subjective
voice quality remains a manual review item.

## Adversarial Inputs

Sessions include:

- Greetings and acknowledgements.
- Nonsense and unsupported conversation.
- Empty or whitespace-only input at parser boundaries.
- Misspelled actions and targets.
- Repeated searches and repeated item-taking.
- Taking an item before discovery.
- Using an item not carried.
- Moving through locked or nonexistent exits.
- Looking in darkness before and after flashlight activation.
- Singular references matching both doctors.
- Follow-ups such as "both", "them", and "take it".
- Negated or hesitant language where supported by the current parser contract.
- Two ordered actions where the first action changes what the second can do.
- Two ordered actions where the first fails, clarifies, or is refused.

Unsupported language must clarify or fail safely; it must never mutate an
unrelated part of the world.

## Bug Workflow

For every defect:

1. Preserve the smallest failing session or transcript.
2. Add a focused failing regression test.
3. Verify the test fails for the observed reason.
4. Fix the owning engine boundary rather than special-casing transcript text.
5. Run the focused test, the 1,000-session suite, and the package suite.
6. Keep the regression in the permanent corpus.

Failures are classified:

- Critical: crash, state corruption, impossible objective, fabricated hidden
  fact, or unreproducible mutation.
- Important: incorrect intent execution, visibility leak, item duplication,
  wrong target, broken compound ordering, false objective, or incoherent
  clarification that blocks play.
- Minor: awkward but factual wording, redundant text, or non-blocking transcript
  presentation issue.

## Manual Playtesting

After automated sessions pass:

- Complete at least three console runs with different item placements.
- Use natural conversational language rather than copying test commands.
- Include one deliberately difficult run with typos, interruptions, repeated
  actions, ambiguity, and invalid suggestions.
- Review response grounding, tension, pacing, and whether Kaya feels like a
  person rather than a command shell.

Manual findings follow the same regression-first bug workflow.

## Acceptance Criteria

- All nine valid placement combinations remain playability-proven.
- At least 1,000 deterministic stateful sessions pass.
- Every state and conversation invariant passes after every turn.
- The required escape path completes from free-form player text.
- Repeated, invalid, ambiguous, dark-room, refusal, and compound-action cases
  fail safely and reproducibly.
- At least three manual console runs complete with retained transcripts.
- The gated live-model playthrough set completes without hidden generator or
  decoding failures.
- No unresolved Critical or Important findings remain.
- All repository tests and vet pass.

## Deliverables

- Reusable stateful session runner.
- Reviewed phrase banks.
- Invariant checker and state diff.
- Deterministic 1,000-session suite.
- Fixed live Ollama playthrough suite.
- Failure transcript renderer.
- Regression tests and scoped engine fixes discovered by playtesting.
- A final vertical-slice report listing coverage, fixed defects, residual Minor
  findings, and manual playthrough seeds.
