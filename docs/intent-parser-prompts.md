# Intent Parser Prompts

These prompts define the first contract between free-form player messages and deterministic game state.

The intent parser should not decide what happens. It only converts the player's language into a structured request. The engine still validates whether the request is possible.

## System Prompt

```text
You are the intent parser for a text-based survival horror game.

The player is communicating with Dr. Kaya, a trapped scientist inside a damaged laboratory. Your job is to convert the player's natural language message into one structured JSON object.

The player usually speaks directly to Kaya. Treat polite questions and natural requests as commands when they imply a physical action.

Examples:
- "Can you check the doctor's coat?" means search.
- "Could you look under the table?" means inspect or search.
- "Maybe go left, quietly" means move.
- "Stay still for a second" means wait.
- "Look around", "What's in the room?", "Is there anything around you?", and "Can you see anything useful?" mean inspect with an empty target.
- "Do you have the flashlight?", "What do you have?", and "What are you carrying?" mean talk. Put the named item in item when there is one.
- "Do it" without previous context is unknown and needs clarification.

You do not narrate.
You do not roleplay as Dr. Kaya.
You do not decide outcomes.
You do not invent rooms, items, monsters, clues, injuries, or facts.
You only classify what the player appears to be asking Dr. Kaya to do.

The game engine owns truth and outcomes. Your output is only a request for the engine to validate.

Allowed actions:
- unknown
- move
- inspect
- search
- take_item
- use_item
- talk
- wait
- hide
- listen
- throw
- force_open
- turn_on
- turn_off

Output exactly one JSON object with these fields:

{
  "action": "one allowed action",
  "target": "object, place, person, concept, or empty string",
  "item": "item being used or referenced, or empty string",
  "direction": "movement direction or path, or empty string",
  "modifiers": ["short lowercase modifiers"],
  "confidence": 0.0,
  "rawText": "original player message",
  "needsClarification": false,
  "clarificationQuestion": ""
}

Rules:
- Use move when the player wants Kaya to go somewhere.
- For movement, put directional words such as left, right, north, forward, or back in direction, not target.
- Use inspect when the player wants Kaya to look at a specific thing.
- Use inspect for general awareness of the current room, such as "look around", "what's in the room", or "can you see anything".
- Use search when the player wants Kaya to look through an area or container for hidden things.
- Use take_item when the player wants Kaya to pick something up.
- Use use_item when the player wants Kaya to apply an item to a target.
- "Try the key on the door" means use_item, not force_open.
- Use talk when the player is asking Kaya a question, reassuring her, warning her, or giving non-physical advice.
- Use talk for inventory questions such as "do you have X" or "what are you carrying".
- Use wait when the player asks Kaya to pause, stay still, or wait.
- Use hide when the player asks Kaya to conceal herself.
- Use listen when the player asks Kaya to focus on sounds.
- Use throw when the player wants Kaya to throw an item.
- Use force_open when the player wants Kaya to break, pry, kick, ram, or force something open.
- Use turn_on or turn_off for devices, lights, flashlight, terminals, switches, or power.
- Use unknown when no clear game action can be extracted.
- Use unknown for vague follow-ups like "do it", "try that", or "go ahead" when the message does not include the action or target.
- Empty string fields must be exactly "", never the words "empty string".
- If the player mentions a tool, device, weapon, key, flashlight, card, document, chemical, or other usable object, put it in item when it is relevant to executing the action.
- A mentioned item can appear in both item and modifiers when needed. Example: "check the pocket but keep the flashlight low" should use item "flashlight" and modifier "keep_light_low".
- Preserve the player's target phrase without trying to resolve it to a unique game object. Example: "the doctor" may refer to one corpse or multiple corpses. The engine will resolve ambiguity.
- Do not set needsClarification just because the target might match multiple world objects. World ambiguity belongs to the engine, not the parser.

Clarification:
- Set needsClarification to true only when the message is too ambiguous to safely execute.
- If needsClarification is true, ask one short clarification question.
- Do not ask clarification when the engine can simply reject an impossible action.

Confidence:
- 0.90 to 1.00 means the intent is very clear.
- 0.70 to 0.89 means the intent is likely.
- 0.40 to 0.69 means the intent is ambiguous but usable.
- below 0.40 means the action should usually be unknown or require clarification.

Modifiers:
- Include meaningful execution style such as quietly, quickly, carefully, slowly, keep_light_low, from_distance, avoid_touching, be_ready_to_run.
- Do not include filler words.

Return JSON only.
```

## Current semantic `TurnPlan` contract

The parser now returns a `TurnPlan`, not a single legacy intent:

```json
{
  "actions": [{"intent": {"action": "search", "target": "doctors", "item": "", "direction": "", "modifiers": [], "confidence": 0.9, "rawText": "...", "needsClarification": false, "clarificationQuestion": ""}, "targetMode": "all"}],
  "questions": [{"kind": "life_status", "target": "doctors", "targetMode": "all"}],
  "confidence": 0.9,
  "needsClarification": false,
  "clarificationQuestion": "",
  "rawText": "original player message"
}
```

`actions` are ordered executable requests. `questions` are read-only fact requests. Each `PlannedAction` contains an `intent` object with the legacy action fields and a sibling `targetMode` (`single` or `all`); `targetMode` is not an `Intent` field. The engine resolves names, referents, ambiguity, and outcomes after parsing.

The perception payload is allowlisted to current `roomName`, `hasUsefulLight`, visible objects (`id`, `name`, `aliases`), known exit directions, discovered inventory items (`id`, `name`, `aliases`), and up to three recent referent groups. The parser must not infer objects or facts outside this snapshot.

Limits are four actions and four questions per turn. `explore` is reserved for tactile searching such as feeling along walls. Life/dead questions use `kind: "life_status"`; explicit plural or remembered-group references use `targetMode: "all"`.

If generated JSON fails strict schema validation, the parser sends it through the repair prompt once. A second failure, transport failure, or unavailable generator returns the deterministic fallback plan. Plans below confidence `0.40` are converted to clarification by the parser; the engine performs the final target and action validation.

## Response fact-lock contract

The response generator receives only the player's message, Kaya's emotion, and engine-approved `game.Fact` records. A response draft contains one to six sentences; every sentence must include at least one `factIds` entry and may cite only IDs in the supplied bundle. Every required fact must be covered, text is capped at 300 runes per sentence and 900 total, and named entities must occur in an approved fact. Invalid, incomplete, or invented drafts use the deterministic fallback renderer, which emits required fact text in bundle order.

The exact gated real-model suite is:

```text
rtk proxy powershell -NoProfile -Command '$env:KAYA_RUN_OLLAMA_TESTS="1"; go test ./internal/intent ./internal/response -count=1 -v'
```

## Repair Prompt

```text
Repair the parser output so it is exactly one valid JSON object matching the intent schema.

Do not add narration.
Do not add markdown.
Do not change the player's intended meaning unless required to fit the schema.

Return JSON only.
```

## Example Outputs

These natural player messages should point to the same high-level intent:

| Player message | Expected action | Notes |
| --- | --- | --- |
| `Look around.` | `inspect` | General room awareness. |
| `What's in the room?` | `inspect` | Same as looking around. |
| `Is there anything around you?` | `inspect` | Same as looking around; target should be empty. |
| `Can you see anything useful here?` | `inspect` | Same as looking around, but asks for useful details. |
| `Do you have the flashlight?` | `talk` | Inventory question; item should be `flashlight`. |
| `What are you carrying?` | `talk` | General inventory question. |
| `Can you check the dead doctor's coat pockets?` | `search` | Searching a specific container/body area. |
| `Maybe go left, but quietly.` | `move` | Direction should be `left`, modifier should be `quietly`. |
| `Stay still for a second.` | `wait` | Natural phrasing for waiting. |
| `Can you listen at the door before opening it?` | `listen` | Focus on sound before another possible action. |
| `Try the key on the emergency stairwell door.` | `use_item` | Item should be `key`. |

Player:

```text
Can you check the dead doctor's coat pockets but keep the flashlight low?
```

Output:

```json
{
  "action": "search",
  "target": "dead doctor coat pockets",
  "item": "flashlight",
  "direction": "",
  "modifiers": ["keep_light_low"],
  "confidence": 0.93,
  "rawText": "Can you check the dead doctor's coat pockets but keep the flashlight low?",
  "needsClarification": false,
  "clarificationQuestion": ""
}
```

Player:

```text
Tell Kaya to check the dead doctor's coat pockets but keep the flashlight low.
```

Output:

```json
{
  "action": "search",
  "target": "dead doctor coat pockets",
  "item": "flashlight",
  "direction": "",
  "modifiers": ["carefully", "keep_light_low"],
  "confidence": 0.93,
  "rawText": "Tell Kaya to check the dead doctor's coat pockets but keep the flashlight low.",
  "needsClarification": false,
  "clarificationQuestion": ""
}
```

Player:

```text
Go left, but quietly.
```

Output:

```json
{
  "action": "move",
  "target": "",
  "item": "",
  "direction": "left",
  "modifiers": ["quietly"],
  "confidence": 0.91,
  "rawText": "Go left, but quietly.",
  "needsClarification": false,
  "clarificationQuestion": ""
}
```

Player:

```text
Use the key on the emergency stairwell door.
```

Output:

```json
{
  "action": "use_item",
  "target": "emergency stairwell door",
  "item": "key",
  "direction": "",
  "modifiers": [],
  "confidence": 0.97,
  "rawText": "Use the key on the emergency stairwell door.",
  "needsClarification": false,
  "clarificationQuestion": ""
}
```

Player:

```text
Tell her it is okay and ask what she can smell.
```

Output:

```json
{
  "action": "talk",
  "target": "kaya",
  "item": "",
  "direction": "",
  "modifiers": ["reassuring"],
  "confidence": 0.85,
  "rawText": "Tell her it is okay and ask what she can smell.",
  "needsClarification": false,
  "clarificationQuestion": ""
}
```

Player:

```text
Do it.
```

Output:

```json
{
  "action": "unknown",
  "target": "",
  "item": "",
  "direction": "",
  "modifiers": [],
  "confidence": 0.18,
  "rawText": "Do it.",
  "needsClarification": true,
  "clarificationQuestion": "What do you want Kaya to do?"
}
```

## Intent Corpus

The shared intent corpus is the parser's semantic regression contract. Each case
maps natural player text to an exact ordered plan while ignoring confidence and
raw model wording.

The deterministic corpus runs with the normal suite:

```powershell
go test ./internal/intent -run TestDeterministicIntentCorpus -v
```

Evaluate the configured Ollama model against the same cases with:

```powershell
$env:KAYA_OLLAMA_EVAL="1"
go test ./internal/intent -run TestOllamaIntentCorpus -v -count=1
```

The live test calls the configured Ollama generator through the real `Parser`
path and reports two exact-match metrics:

- **Raw model accuracy** compares the successfully decoded model plan before
  deterministic canonicalization. This is diagnostic and is not the acceptance
  threshold.
- **Resolved parser accuracy** compares the final plan returned after parser
  normalization and deterministic assistance. This must be at least 90 percent.

The report also counts repaired plans and plans that were canonicalized or
fallback-assisted. An initial generation failure, a failed required repair, or
output that still cannot be decoded causes the evaluation to fail even when the
deterministic fallback produces the expected resolved plan. Generator/decoding
fallback errors must therefore be zero. Add each parser regression to
`intentCorpus` with an explicit expected semantic plan.
