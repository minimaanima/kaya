# Dr. Kaya Text Survival Horror - Foundation

## Core Concept

The game is a text-based survival horror experience where the player communicates with Dr. Kaya through chat. Dr. Kaya is trapped inside a damaged laboratory after an earthquake and needs help escaping.

The player is not directly controlling a character with fixed commands. Instead, they write natural messages such as:

- "Tell Kaya to check the desk with the flashlight."
- "Ask her to move quietly down the corridor."
- "Use the brick to break the glass."
- "Tell her not to open that door yet."

The game interprets the player's intent, validates it against the current world state, advances time, updates Dr. Kaya's emotional condition, and then returns a response in her voice.

The horror should come from uncertainty, limited information, time pressure, darkness, stress, and the feeling that Dr. Kaya is a real person who may hesitate, panic, refuse, or misunderstand.

## Primary Design Rule

The game engine owns the truth.

The LLM owns interpretation and voice.

This means:

- The engine decides what rooms exist.
- The engine decides where items are.
- The engine decides whether a door is locked.
- The engine decides whether a monster is nearby.
- The engine decides whether Dr. Kaya survives.
- The engine decides whether an action is possible.
- The engine decides what is revealed by light, searching, or movement.

The LLM can help parse messy player language and rephrase Dr. Kaya's responses, but it should not invent facts or decide game outcomes.

## High-Level Flow

```text
Player message
-> Intent parser
-> Structured intent
-> Context validation
-> Action resolver
-> Time advancement
-> World state update
-> Dr. Kaya state update
-> Response generation
-> Player receives Kaya's message
```

## Intent Parsing

The player can write anything, so the intent parser is one of the most important systems.

The parser converts natural language into a structured intent that the game engine can understand.

Example player message:

```text
Tell Kaya to check the dead doctor's coat pockets, but keep the flashlight low.
```

Possible parsed intent:

```json
{
  "action": "inspect",
  "target": "dead doctor pockets",
  "item": "flashlight",
  "modifiers": ["careful", "keep light low"],
  "confidence": 0.88
}
```

The parser should produce structured data, not final story text.

Possible intent fields:

```go
type Intent struct {
    Action     string
    Target     string
    Item       string
    Direction  string
    Modifiers  []string
    Confidence float64
}
```

Common actions:

- `move`
- `inspect`
- `search`
- `take_item`
- `use_item`
- `talk`
- `wait`
- `hide`
- `listen`
- `throw`
- `force_open`
- `turn_on`
- `turn_off`

The parser can be powered by a local LLM, rules, or a hybrid approach. The engine should treat the parsed intent as a request, not as guaranteed truth.

## Context Validation

After parsing, the engine checks whether the requested action makes sense.

Examples:

- A key cannot be used if there is no locked door nearby.
- A flashlight cannot reveal objects if Dr. Kaya does not have it.
- A table cannot be searched if the current room has no table.
- A key on a table cannot be found in a fully dark room unless the flashlight or another light source is active.
- A brick cannot be thrown through a window if there is no window.
- A door cannot be unlocked if the key does not match it.

Invalid actions should still produce an in-world response from Dr. Kaya.

Example:

```text
"There's no door here. Are you sure you're seeing the same room I am?"
```

## Puzzle Philosophy

Puzzles are a core part of the game. They should not only be about using the correct item on the correct object. Some puzzles should require the player's own reasoning, research, memory, and interpretation.

The player may need to:

- Read documents that Dr. Kaya finds.
- Ask Kaya to inspect screens, labels, notes, or terminals.
- Use information Kaya gives them to search external pages or provided in-game websites.
- Combine clues from multiple rooms.
- Understand a procedure before telling Kaya what to do.
- Decode passwords, access levels, formulas, or lab protocols.
- Decide whether a risky solution is worth attempting.

This creates a feeling that the player is helping from outside the laboratory, not simply choosing actions from an adventure game menu.

## Player-Side Research Puzzles

Some puzzles can require the player to investigate information outside the immediate chat.

Example setup:

```text
Kaya finds a locked administrator terminal.
The terminal asks for a staff ID, a password, and a chemical handling override code.
Nearby, Kaya finds a damaged memo mentioning an internal research page.
```

The player may need to:

1. Ask Kaya for exact details from the memo.
2. Visit a specific page, document, wiki, or simulated intranet resource.
3. Find the administrator name or password hint.
4. Cross-reference it with another clue Kaya found.
5. Tell Kaya what credentials or override code to try.

The game engine should still validate the result. The LLM should not simply accept any plausible answer.

Possible research puzzle types:

- Finding an administrator password from documents.
- Looking up a lab protocol.
- Interpreting a chemical warning sheet.
- Understanding which two chemicals react violently.
- Finding an employee ID from personnel records.
- Decoding a containment label.
- Using a map or evacuation plan to choose a route.
- Identifying a specimen weakness from experiment logs.

## Knowledge-Based Puzzle Example

Puzzle:

```text
A reinforced lab door is jammed shut.
There are chemicals nearby.
Kaya cannot force the door open.
The player must figure out whether a controlled reaction can break the lock or hinge.
```

Possible flow:

1. Kaya describes the available chemicals.
2. The player asks her to read labels, warnings, storage codes, or lab notes.
3. The player researches or interprets which substances can create pressure, heat, gas, corrosion, or an explosive reaction.
4. The player tells Kaya what to mix, where to place it, and how far to stand back.
5. The engine checks if the combination is valid, risky, useless, or deadly.

Important:

- The puzzle should have enough information to solve fairly.
- Wrong combinations should have consequences.
- Kaya may resist if the action sounds dangerous.
- High trust may make her more willing to try.
- High stress may make her handle the procedure poorly.
- Time should advance while she prepares the solution.

Example engine outcomes:

```text
Correct mixture + safe placement:
The lock breaks, Kaya survives, stress increases.

Correct mixture + unsafe placement:
The door opens, but Kaya is injured.

Wrong mixture:
Nothing useful happens, time is lost, fumes increase danger.

Very dangerous mixture:
Explosion, injury, fire, or death.
```

These puzzles should feel serious. If the player is making Kaya combine chemicals, operate machinery, or enter credentials, the game should make them think carefully before acting.

## World Model

The world is made of rooms, exits, objects, hazards, and hidden information.

Possible room fields:

```go
type Room struct {
    ID          string
    Name        string
    Description string
    Dark        bool
    Exits       map[string]string
    Objects     []ObjectRef
    Hazards     []HazardRef
    Flags       map[string]bool
}
```

Room examples:

- Reception
- Main Corridor
- Emergency Stairwell
- Laboratory A
- Laboratory B
- Morgue
- Security Office
- Server Room
- Containment Wing
- Maintenance Shaft

Room state can change over time:

- Lights fail.
- Doors jam.
- Fires spread.
- Water rises.
- A creature moves through vents.
- A corpse is discovered.
- A path collapses.

## Inventory and Items

Items should be functional objects with behavior, not just names.

An item can define when and how it can be used.

Possible item interface:

```go
type Item interface {
    ID() string
    Name() string
    Description() string
    CanUse(ctx ActionContext) bool
    Use(ctx ActionContext) ActionResult
}
```

Example item types:

- `Key`
- `Flashlight`
- `Brick`
- `AccessCard`
- `Bandage`
- `Battery`
- `Crowbar`
- `LabNotes`
- `Radio`
- `Scalpel`

### Key

A key can unlock a matching locked door.

It should fail if:

- There is no nearby locked door.
- The door uses a different key.
- Dr. Kaya does not have the key.

### Flashlight

The flashlight reveals objects in dark rooms.

It should have limits:

- Battery level.
- Visibility range.
- It may attract attention.
- It may flicker under stress or damage.

### Brick

A brick can be thrown.

Possible uses:

- Break glass.
- Distract a creature.
- Trigger a noise in another area.
- Test if a floor is unstable.

Throwing a brick should often create noise and advance time.

## Darkness and Visibility

Darkness is a core puzzle and horror mechanic.

Rooms can be:

- Fully lit
- Dim
- Dark
- Pitch black

Objects can require visibility before they are discoverable.

Example:

```text
The key may be on the table, but Dr. Kaya cannot see it unless she uses the flashlight.
```

Some things may only be visible with specific conditions:

- Flashlight on
- Emergency lights active
- Door opened
- Object moved
- Stress low enough for careful observation
- Enough time spent searching

## Time System

The game should have a sense of time. Dr. Kaya should not always answer immediately.

Actions take time:

- Looking around: 5-15 seconds
- Walking down a corridor: 20-60 seconds
- Searching a desk: 30-90 seconds
- Unlocking a door: 5-20 seconds
- Forcing a door: 30-120 seconds
- Hiding: variable
- Waiting: chosen by player or system

Possible structure:

```go
type ActionResult struct {
    StartedAt   int
    Duration    int
    Outcome     string
    StressDelta int
    Events      []WorldEvent
}
```

During time advancement, the world can change:

- A sound happens nearby.
- A creature moves.
- The lights flicker.
- Dr. Kaya hears breathing.
- A door opens somewhere else.
- A fire spreads.
- A random event triggers.

The player should feel like Dr. Kaya is physically doing things, not instantly executing commands.

## Dr. Kaya State

Dr. Kaya has her own emotional and physical state.

Possible fields:

```go
type KayaState struct {
    Stress      int
    Trust       int
    Fear        int
    Pain        int
    Exhaustion  int
    Injured     bool
    HasDoubt    bool
}
```

Her state affects:

- How she speaks.
- Whether she follows advice.
- Whether she hesitates.
- Whether she refuses.
- Whether she notices details.
- Whether she panics under danger.
- Whether she trusts the player.

Example:

If Kaya hears something in a room and the player tells her to enter anyway, she may ask for confirmation.

If stress is high, she may refuse:

```text
"No. No, I am not going in there. I heard something moving."
```

If trust is high, she may obey but still be afraid:

```text
"I hate this. But... alright. I'm going in."
```

## Refusal and Autonomy

Dr. Kaya should not behave like a command-line puppet.

She can:

- Ask if the player is sure.
- Refuse reckless actions.
- Misinterpret vague advice.
- Freeze under pressure.
- Argue when afraid.
- Act on instinct during immediate danger.

This makes the game feel like the player is guiding a person, not moving a token.

Refusal should be based on state and context:

- High stress
- Low trust
- Recent injury
- Obvious danger
- Vague instruction
- Contradictory instruction
- No available path

## Randomness and Roguelike Variation

Each playthrough should vary.

Examples:

- A key may be on a table in one run.
- The same key may be in a dead doctor's pocket in another run.
- A sound may indicate danger in one run.
- In another run, the same sound may only reveal a shaft where something escaped.
- A door may be jammed in one run and unlocked in another.
- A useful item may spawn in different valid locations.

Randomness should be controlled by a seed so a run can be reproduced during debugging.

Possible randomized systems:

- Item placement
- Hazard placement
- Monster route
- Locked doors
- Environmental damage
- Clue order
- False alarms
- Battery availability

Randomness should not be pure chaos. It should respect game logic.

For example:

- A required key must be reachable.
- A flashlight-dependent item should not be required before the flashlight can be found.
- A lethal monster should not appear before the player understands basic movement.

## Story Foundation

The opening situation:

Dr. Kaya is inside a laboratory after an earthquake. The building is damaged. Communications are unstable. The player is somehow able to communicate with her through text.

At first, the goal seems simple:

```text
Help Dr. Kaya escape.
```

Later, the player discovers that the laboratory was hiding secret experiments.

Possible reveal layers:

1. The earthquake damaged the lab.
2. The lab had restricted areas.
3. Staff were hiding records.
4. Test subjects or organisms were contained there.
5. Something escaped during the earthquake.
6. Dr. Kaya may know more than she first admits.
7. The player may not be only an outside helper.

## Response Generation

The response system should combine hard engine facts with Dr. Kaya's emotional voice.

Engine result:

```json
{
  "visibleFacts": [
    "The room is dark.",
    "The flashlight reveals a metal table.",
    "There is a small brass key near the table leg."
  ],
  "stressDelta": 4,
  "timePassed": 25,
  "dangerLevel": "low"
}
```

Kaya-style response:

```text
"Okay... I turned the flashlight toward the table. There's something under it. A key, I think. Small, brass. God, my hands are shaking."
```

The LLM can rephrase the response, but it must receive the engine facts as constraints.

It should not add:

- New items
- New exits
- New monsters
- New injuries
- New clues
- New outcomes

Unless those facts were already produced by the engine.

## Suggested Go Modules

Possible package layout:

```text
/cmd/kaya
  main.go

/internal/game
  engine.go
  state.go
  clock.go

/internal/world
  room.go
  object.go
  hazard.go
  generator.go

/internal/items
  item.go
  key.go
  flashlight.go
  brick.go

/internal/intent
  intent.go
  parser.go
  schema.go

/internal/actions
  resolver.go
  move.go
  inspect.go
  use_item.go
  search.go

/internal/kaya
  state.go
  behavior.go
  voice.go

/internal/llm
  client.go
  prompts.go
  response.go
```

## Minimal First Prototype

The first Go prototype should be small.

Recommended scope:

- 3 rooms
- 1 locked door
- 1 key
- 1 flashlight
- 1 dark room
- 1 searchable object
- 1 simple random key placement
- Basic stress and trust
- Basic intent parsing
- Basic delayed action durations

Prototype rooms:

```text
Reception -> Main Corridor -> Storage Room
```

Prototype puzzle:

```text
Storage Room is dark.
The key may be on a table or in a corpse's pocket.
The flashlight is needed to reliably find it.
The key unlocks the emergency stairwell door.
```

Prototype win condition:

```text
Dr. Kaya unlocks the emergency stairwell and escapes the first section.
```

## Early Technical Priorities

1. Build the deterministic engine first.
2. Add a simple parser with a strict output schema.
3. Add item behavior through interfaces.
4. Add time costs to actions.
5. Add Dr. Kaya's stress/trust state.
6. Add randomized but valid item placement.
7. Add local LLM rewriting only after the engine is stable.

## Guiding Feeling

The player should feel like they are texting someone who is trapped, scared, intelligent, and not fully under their control.

The game should feel natural to talk to, but strict underneath.

The best version of this is not a normal text adventure with prettier commands. It is a survival horror conversation where language, trust, darkness, and time are all part of the danger.
