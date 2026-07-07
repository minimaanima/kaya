# Kaya Game Loop Function Graph

This is the main console loop from player text to Kaya's reply.

```mermaid
flowchart TD
    main["cmd/kaya/main.go\nmain()"] --> runPlay["runPlay()"]

    runPlay --> setupLLM["llm.NewOllamaClient()"]
    runPlay --> setupParser["intent.NewParser(client)"]
    runPlay --> setupWorld["scenario.NewPrototypeWorld()"]
    runPlay --> setupResolver["actions.NewResolver(state)"]

    runPlay --> readInput["scanner.Scan()\nread player message"]
    readInput --> parse["intent.Parser.Parse(ctx, message)"]

    parse --> generate["TextGenerator.Generate()\nOllama /api/generate"]
    generate --> parseJSON["ParseJSON(raw)"]
    parseJSON --> normalize["normalizeIntent(parsed)"]
    normalize --> parsedIntent["intent.Intent"]

    parseJSON --> repairNeeded{"JSON failed?"}
    repairNeeded -- yes --> repair["TextGenerator.Generate()\nRepairPrompt"]
    repair --> parseJSON
    repairNeeded -- no --> parsedIntent

    parsedIntent --> resolve["actions.Resolver.Resolve(intent)"]
    resolve --> danger["intentDanger(intent)\nestimate action risk"]
    danger --> autonomy["state.Kaya.CanAttempt(danger)"]
    autonomy --> autonomyGate{"allowed?"}
    autonomyGate -- refused --> refused["ActionResult\noutcome: kaya_refused"]
    autonomyGate -- needs confirmation --> confirm["ActionResult\noutcome: kaya_needs_confirmation"]
    autonomyGate -- yes --> actionDispatch{"intent.Action"}

    actionDispatch --> inspect["inspect()"]
    actionDispatch --> move["move()"]
    actionDispatch --> search["search()"]
    actionDispatch --> takeItem["takeItem()"]
    actionDispatch --> useItem["useItem()"]
    actionDispatch --> turnOn["turnOn()"]
    actionDispatch --> turnOff["turnOff()"]
    actionDispatch --> talk["talk()"]
    actionDispatch --> waitListen["wait/listen inline result"]
    actionDispatch --> clarification["clarification()"]

    inspect --> result["game.ActionResult"]
    move --> result
    search --> result
    takeItem --> result
    useItem --> result
    turnOn --> result
    turnOff --> result
    talk --> result
    waitListen --> result
    clarification --> result
    refused --> finalResult
    confirm --> finalResult

    result --> finish["Resolver.finish(result)"]
    finish --> advance{"duration > 0?"}
    advance -- yes --> clock["world.State.Advance(seconds)\nupdate time and fire scheduled events"]
    advance -- no --> kayaApply["state.Kaya.Apply(result)\nupdate stress/trust/fear"]
    clock --> kayaApply
    kayaApply --> finalResult["final ActionResult"]

    finalResult --> print["printResult(result)"]
    print --> kayaText["Kaya: visible facts/events"]
    kayaText --> objective{"state.CurrentRoomID == stairwell?"}
    objective -- yes --> complete["Prototype objective complete"]
    objective -- no --> readInput
```

## Resolver Dispatch

`Resolver.Resolve()` is the central game-action router. The parser only says what the player probably meant. The resolver decides what is possible in the current world.

```mermaid
flowchart LR
    intent["intent.Intent"] --> resolve["Resolver.Resolve()"]

    resolve --> risk["intentDanger()"]
    risk --> canAttempt["state.Kaya.CanAttempt()"]
    canAttempt --> gate{"autonomy result?"}
    gate -- refused --> refused["kaya_refused"]
    gate -- confirmation --> confirm["kaya_needs_confirmation"]
    gate -- no --> inspect["ActionInspect\ninspect()"]
    gate -- no --> move["ActionMove\nmove()"]
    gate -- no --> search["ActionSearch\nsearch()"]
    gate -- no --> take["ActionTakeItem\ntakeItem()"]
    gate -- no --> use["ActionUseItem\nuseItem()"]
    gate -- no --> on["ActionTurnOn\nturnOn()"]
    gate -- no --> off["ActionTurnOff\nturnOff()"]
    gate -- no --> talk["ActionTalk\ntalk()"]
    gate -- no --> listen["ActionListen\nmakeResult()"]
    gate -- no --> wait["ActionWait\nmakeResult()"]
    gate -- no --> unknown["ActionUnknown\nclarification()"]

    inspect --> worldRead["read room, visible objects, exits"]
    move --> worldMove["check exits and doors\nmutate CurrentRoomID"]
    search --> discover["resolve visible object\ndiscover contained items"]
    take --> inventory["require discovered + portable\nadd inventory"]
    use --> doorLight["use flashlight or key\nmutate light/door state"]
    on --> lightOn["require flashlight\nActiveLight = true"]
    off --> lightOff["require flashlight\nActiveLight = false"]
    talk --> answer["inventory, item location, or simple reply"]

    worldRead --> finish["finish()"]
    worldMove --> finish
    discover --> finish
    inventory --> finish
    doorLight --> finish
    lightOn --> finish
    lightOff --> finish
    answer --> finish
    listen --> finish
    wait --> finish
    unknown --> output["ActionResult"]
    finish --> output
    refused --> output
    confirm --> output
```

## Kaya Autonomy And Emotion

Phase 4 adds a deterministic autonomy layer. It checks risky actions before execution and updates Kaya's emotional state after successful engine results.

```mermaid
flowchart TD
    intent["intent.Intent"] --> risk["Resolver.intentDanger()"]
    risk --> danger{"DangerLevel"}

    danger --> canAttempt["kaya.State.CanAttempt(danger)"]
    canAttempt --> decision{"AutonomyDecision"}

    decision -- allowed --> execute["execute action normally"]
    decision -- needs confirmation --> ask["kaya_needs_confirmation\nno time passes"]
    decision -- refused --> refuse["kaya_refused\nno state mutation"]

    execute --> result["game.ActionResult"]
    result --> finish["Resolver.finish()"]
    finish --> time["world.State.Advance()\ncollect events"]
    time --> apply["kaya.State.Apply(result)\napply deltas and event danger"]
    apply --> emotion["DominantEmotion()\ncalm/uneasy/scared/panicked/exhausted"]
```

## File Map

- `cmd/kaya/main.go`: console entrypoint, play loop, playtest runner, printing.
- `internal/intent/parser.go`: LLM call, JSON parsing, repair, light normalization.
- `internal/intent/prompt.go`: instructions and examples for the LLM intent parser.
- `internal/actions/resolver.go`: turns an intent into game behavior.
- `internal/kaya/autonomy.go`: refusal, confirmation, and emotional state updates.
- `internal/kaya/state.go`: stress, trust, fear, pain, exhaustion.
- `internal/world/state.go`: current room, inventory, discoveries, light, visible objects.
- `internal/world/clock.go`: time advancement and scheduled events.
- `internal/scenario/prototype.go`: prototype map, rooms, objects, items, doors.

## Mental Model

The loop is:

```text
player text
-> LLM intent JSON
-> Go validates/normalizes
-> Kaya autonomy checks risky actions
-> Resolver checks world rules
-> State changes
-> Kaya stress/fear/trust updates
-> Kaya speaks visible facts
-> repeat
```

The important boundary is:

```text
LLM decides intention.
Go decides truth.
```
