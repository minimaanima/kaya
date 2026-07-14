# Scalable Intent Pipeline Design

## Decision

Replace phrase-driven correction with a typed semantic pipeline. Ollama interprets unrestricted language, but its JSON is untrusted. Deterministic code validates action contracts, grounds mentions against current perception, and controls clarification and execution.

Ollama is required for unrestricted language. The offline fallback remains deliberately small: basic movement, room awareness, inventory, quit, and a clear unavailable-model response.

## Pipeline

```text
player message + perception
        |
        v
model DTO (mentions and exact source evidence)
        |
        v
semantic compiler
  - action/slot contracts
  - executable action vocabulary
  - evidence coverage and duplicate-action checks
        |
        +-- invalid --> one constrained repair --> compile again
        |
        v
typed semantic actions
        |
        v
incremental world grounder
  - object, item, door, direction, and recent referents
        |
        +-- ambiguous --> stored clarification
        +-- missing ----> grounded failure, never invention
        |
        v
deterministic execution and fact-locked response
```

Grounding is incremental. In a compound turn, an earlier action may reveal an item, turn on a light, or move Kaya before the next action is grounded.

## Model Contract

The model returns mentions, never authoritative world IDs. Each action carries the exact message fragment that supports it.

```json
{
  "actions": [{
    "kind": "use",
    "itemMention": "the key",
    "targetMention": "that door",
    "quantity": "one",
    "evidence": "use the key to unlock that door"
  }],
  "questions": []
}
```

The model-facing DTO is converted into action-specific Go types such as `Move`, `Inspect`, `Search`, `Take`, `Use`, `Toggle`, `Wait`, and `Talk`. Unsupported engine actions are not executable merely because they appear in valid JSON.

## Semantic Validation

Contracts define required and forbidden slots per action. Examples:

| Action | Required | Forbidden |
| --- | --- | --- |
| move | direction | item mention |
| search | target mention | direction, item mention |
| take | target mention | direction |
| use | item and target mentions | direction |
| toggle | item mention and on/off state | direction |
| inspect room | no entity required | item mention |

Evidence must be present in the player message. Multiple actions cannot claim the same evidence unless the contract explicitly permits shared coordination. This rejects duplicated or invented actions without adding room names or sentence templates.

Validation errors are structured (`action`, `field`, `code`, `message`). The repair prompt receives only the original message, perception, rejected DTO, errors, and the same schema. There is at most one repair. A second failure becomes clarification and never executes partially.

Repair finishes before any world mutation. Once compound execution begins, later grounding is based on the updated world and can only execute, clarify, or fail factually; it never replays earlier actions.

## Generic Grounding

The grounder resolves mentions using only currently permitted perception and state:

1. exact ID supplied internally by pending clarification;
2. exact normalized name;
3. exact alias;
4. recent singular or plural referent;
5. unique normalized token match.

No arbitrary first match is allowed. Zero matches produce a missing-reference result. Multiple equally valid matches produce candidates and a clarification. The same mechanism applies to objects, inventory items, doors, and exits; adding a room does not add parser conditions.

## Clarification

Session state stores a pending clarification containing the typed action, unresolved role, candidate IDs, and remaining actions. The next player message is interpreted against those candidates. Names, aliases, ordinals, singular/plural selection, confirmation, and cancellation are generic operations.

Clarification is asked only when required information is absent or multiple candidates remain. A unique visible or remembered match executes without asking.

## Migration

1. Add the model DTO, typed compiler, contracts, and structured validation errors behind the current parser interface.
2. Add generic incremental grounding for every executable action role.
3. Add pending clarification to the session processor.
4. Route semantic compilation failures through the single pre-execution repair boundary.
5. Remove contextual phrase overrides and reduce deterministic fallback to the minimal offline set.

The recent room-search, plural-doctor, and unlock phrases remain regression inputs, not production parsing rules.

## Proof

- Contract table tests and fuzz tests reject contradictory fields and duplicate evidence.
- Synthetic worlds rename every entity and vary aliases, collisions, visibility, inventory, and exits.
- Multi-turn tests cover unique grounding, ambiguity, ordinal/name selection, plural selection, confirmation, and cancellation.
- Held-out paraphrases score raw model DTOs separately from repaired results; deterministic phrase correction cannot inflate accuracy.
- Stateful generated-world playtests prove compound actions, darkness, discovery, time, and completion still work.
- Existing transition and response invariants remain mandatory.

## Non-Goals

- No room-specific parser rules.
- No attempt to build a full offline natural-language parser.
- No capability-system rewrite in this phase; capability-driven items and puzzles can follow once typed grounding is stable.
