# Prototype Vertical Slice Hardening Report

## Status

BLOCKED for live Ollama certification. The deterministic slice, all nine generated placements, and three completed manual console routes pass. The latest recorded opt-in live run passed 1/3 seeds: seed 12 completed, while seeds 10 and 11 still produced response fallback after their one allowed repair omitted a required elapsed-time fact. Phase 8 is therefore not marked complete.

The final whole-change audit findings are fixed and covered offline. The final-fix pass did not run `KAYA_LIVE_SLICE_TESTS=1`; the latest live evidence below remains the certification authority and is not being relabeled as a passing result.

## Commands And Results

```powershell
Remove-Item Env:KAYA_LIVE_SLICE_TESTS -ErrorAction SilentlyContinue
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go test ./internal/playtest -run TestOllamaPrototypeCompletePlaythroughs -v -count=1
```

PASS with one skipped live test; package time `1.118s`, observed wall time `2.0283432s`. The skip occurs before `llm.NewOllamaClient` is constructed.

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go test ./internal/playtest -run TestPrototypeThousandPhraseVariedSessionsReachObjective -v -count=1
```

PASS: `TestPrototypeThousandPhraseVariedSessionsReachObjective`, 1,000 deterministic sessions, 9/9 placements, and both unlock phrases; test time `0.45s`, package time `1.550s`, observed wall time `2.4968632s`.

```powershell
Remove-Item Env:KAYA_LIVE_SLICE_TESTS -ErrorAction SilentlyContinue
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go test ./... -count=1
```

PASS: 15 packages total, 13 test-bearing packages passed, 2 packages had no test files; observed wall time `2.4670785s`. This includes the 1,000-session test in `internal/playtest`.

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go vet ./...
```

PASS with no diagnostics; observed wall time `0.4135468s`.

## Placement Coverage

The deterministic 1,000-session proof exercised every flashlight/key pair:

| Flashlight | Brass key | Covered |
| --- | --- | --- |
| Reception Desk | Doctor Near Cabinet | yes |
| Reception Desk | Doctor Near Door | yes |
| Reception Desk | Storage Cabinet | yes |
| Reception Floor | Doctor Near Cabinet | yes |
| Reception Floor | Doctor Near Door | yes |
| Reception Floor | Storage Cabinet | yes |
| Collapsed Chair | Doctor Near Cabinet | yes |
| Collapsed Chair | Doctor Near Door | yes |
| Collapsed Chair | Storage Cabinet | yes |

## Live Ollama Gate

Runtime configuration observed by `TestOllamaPrototypeCompletePlaythroughs`: model `qwen3.5:4b`, base URL `http://localhost:11434`. The `ollama` executable was not on `PATH`, but the configured local service answered the test requests. No `KAYA_OLLAMA_MODEL` or `KAYA_OLLAMA_URL` override was present.

### Latest Recorded Run

The latest recorded live run used the exact-copy repair contract and remained BLOCKED after `1m53.5880495s`. Seed 12 passed; seeds 10 and 11 failed at storage awareness when the repaired JSON omitted the required elapsed-time fact. Parser fallback/errors remained `0/0` for every seed.

| Seed | Result | Response provenance |
| --- | --- | --- |
| 10 | BLOCKED at storage awareness: initial `unsupported_claim`, repair `missing_required_fact` | 6 turns; response generated/fallback `5/1`; repair attempts/successes `4/3` |
| 11 | BLOCKED at the same storage-awareness step: initial `unsupported_claim`, repair `missing_required_fact` | 6 turns; response generated/fallback `5/1`; repair attempts/successes `4/3` |
| 12 | PASS | 9 turns; response generated/fallback `9/0`; repair attempts/successes `6/6` |

The final-fix audit strengthened offline acceptance: every processed live turn must now have source `model` or `repair`, a captured raw plan, exact processed/raw/resolved/generated count equality, and zero parser fallback, provenance errors, or response fallback. No new live run was made.

### Initial Run

```powershell
$env:KAYA_LIVE_SLICE_TESTS = '1'
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go test ./internal/playtest -run TestOllamaPrototypeCompletePlaythroughs -v -count=1
```

The initial run found a real validator defect: approved entities followed by punctuation were rejected as `unknown_entity`. `TestComposerAcceptsApprovedEntitiesAdjacentToPunctuation` was RED, then GREEN after punctuation-normalized whole-word matching. An interim neutral-token lexicon expansion briefly accepted fact-cited paraphrase, but review removed that unsafe broad acceptance. Current behavior is strict: `TestComposerRepairsNeutralParaphraseWithExactFactText` proves the paraphrase is rejected first and then recovered only by the one-pass exact-text repair; `TestComposerRejectsUnfoundedTakeClaim` and direct validator regressions preserve the fact lock for invented state or action claims. Existing unknown-entity, unknown-claim, predicate, and movement rejection tests remain green.

The initial live run was BLOCKED: 0/3 seed subtests passed, with observed wall time `30.9070275s`. All parser turns seen before failure used the generator with raw plans captured and zero parser fallback/provenance errors. The response validator correctly rejected additional prose not present in the facts, for example `while I hold it steady` and `my eyes adjust to the dark`; accepting such prose merely to suppress fallback would weaken the fact-locking acceptance criterion.

| Seed | Placements | Last observed provenance before failure | Result |
| --- | --- | --- | --- |
| 10 | Flashlight -> Collapsed Chair; Brass Key -> Doctor Near Door | 2 turns; generator-used 2; repaired 0; canonicalized 2; raw/resolved 2/2; parse fallback/errors 0/0; response generated/fallback 1/1 | BLOCKED at the chair search by `unsupported_claim` for `Thirty-five seconds pass.` |
| 11 | Flashlight -> Reception Desk; Brass Key -> Storage Cabinet | 3 turns; generator-used 3; repaired 0; canonicalized 3; raw/resolved 3/3; parse fallback/errors 0/0; response generated/fallback 2/1 | BLOCKED at flashlight pickup by `unsupported_claim` for fact-adjacent additions including `while I hold it steady` and `my eyes adjust to the dark.` |
| 12 | Flashlight -> Reception Floor; Brass Key -> Doctor Near Door | 2 turns; generator-used 2; repaired 0; canonicalized 2; raw/resolved 2/2; parse fallback/errors 0/0; response generated/fallback 1/1 | BLOCKED at floor search by `unsupported_claim` for `Thirty-five seconds pass.` |

Every failure rendered the complete Markdown stateful transcript, including placements, player input, raw and resolved plans, parser provenance, response metadata, before/after snapshots, diffs, and invariant results. No generated transcript was added to git.

## Manual Console Evidence

Each run used `go run ./cmd/kaya play --seed <seed> --parse-log` with `KAYA_DEBUG=1`. Inputs differ from `PrototypeWinningMessages`; parse logs and debug output were observed directly.

### Seed 10

Placements: Flashlight -> Collapsed Chair; Brass Key -> Doctor Near Door. Completion: PASS, `Prototype objective complete.`

```text
> can you check the collapsed chair for me?
Kaya: I search the Collapsed Chair. I find Flashlight. 35 seconds pass.
> grab the flashlight
Kaya: I pick up Flashlight. Five seconds pass.
> switch the flashlight on
Kaya: I turn on the flashlight. 3 seconds pass.
> move east
Kaya: I move east into Storage Room. A pitch-black storage room with overturned cabinets and a chemical smell. 20 seconds pass. Somewhere deeper in the building, metal scrapes against concrete.
> inspect the doctor
Kaya: Which one do you mean: Doctor Near Cabinet, Doctor Near Door?
> both
Kaya: I search the Doctor Near Cabinet but find nothing useful. The doctor is dead. 30 seconds pass. I search the Doctor Near Door and I find Brass Key. The doctor is dead. 35 seconds pass.
> search both
Kaya: Which one do you mean: Doctor Near Cabinet, Doctor Near Door?
> grab the brass key
Kaya: I pick up Brass Key. 5 seconds pass.
> try brass key on the stairwell door
Kaya: The key turns in the lock. The Emergency Stairwell Door is unlocked. 8 seconds pass.
> move north
Kaya: I move north into Emergency Stairwell. A concrete stairwell beyond a locked fire door. 20 seconds pass.
Kaya: I am in the stairwell. This part is clear.
Prototype objective complete.
```

Earlier seed-10 attempts also covered conversational chatter, interruption (`actually, no wait`), repeated search, and the invalid moon suggestion. They did not complete because `both doctors, please` produced a four-action clarification and `flashlight, please` retained the polite suffix as the item target. These are recorded as parser wording observations, not hidden by the successful route.

### Seed 11

Placements: Flashlight -> Reception Desk; Brass Key -> Storage Cabinet. Completion: PASS, `Prototype objective complete.`

```text
> hello, are you still with me?
Kaya: I feel along the walls but cannot find another exit. 30 seconds pass.
> could you search the reception desk?
Kaya: I search the Reception Desk. I find Flashlight. 35 seconds pass. Somewhere deeper in the building, metal scrapes against concrete.
> search the desk again
Kaya: I cannot see that here.
> grab flashlight
Kaya: I pick up Flashlight. Five seconds pass.
> turn the flashlight on
Kaya: I turn on the flashlight. 3 seconds pass.
> head east
Kaya: I move east into Storage Room. A pitch-black storage room with overturned cabinets and a chemical smell. 20 seconds pass.
> what is isnide the storage cabiner?
Kaya: I search the Storage Cabinet. I find Brass Key. 35 seconds pass.
> search the storage cabinet
Kaya: I search the Storage Cabinet. I find Brass Key. 35 seconds pass.
> maybe throw the moon at that cabinet
Kaya: I am not sure how to do that yet: throw. 2 seconds pass.
> pick up brass key
Kaya: I pick up Brass Key. 5 seconds pass.
> try brass key on the stairwell door
Kaya: The key turns in the lock. The Emergency Stairwell Door is unlocked. 8 seconds pass.
> go north
Kaya: I move north into Emergency Stairwell. A concrete stairwell beyond a locked fire door. 20 seconds pass.
Kaya: I am in the stairwell. This part is clear.
Prototype objective complete.
```

### Seed 12

Placements: Flashlight -> Reception Floor; Brass Key -> Doctor Near Door. Completion: PASS, `Prototype objective complete.`

```text
> hello, is the line still clear?
Kaya: I feel along the walls but cannot find another exit. 30 seconds pass.
> could you check the reception floor?
Kaya: I search the Reception Floor. I find Flashlight. 35 seconds pass. Somewhere deeper in the building, metal scrapes against concrete.
> search the reception floor
Kaya: I search the Reception Floor. I find Flashlight. 35 seconds pass.
> search the reception floor again
Kaya: I search the Reception Floor. I find Flashlight. 35 seconds pass.
> grab flashlight
Kaya: I pick up Flashlight. Five seconds pass.
> switch the flashlight on
Kaya: I turn on the flashlight. 3 seconds pass.
> move east
Kaya: I move east into Storage Room. A pitch-black storage room with overturned cabinets and a chemical smell. 20 seconds pass.
> inspect the doctor
Kaya: Which one do you mean: Doctor Near Cabinet, Doctor Near Door?
> both
Kaya: I search the Doctor Near Cabinet but find nothing useful. The doctor is dead. 30 seconds pass. I search the Doctor Near Door and I find Brass Key. The doctor is dead. 35 seconds pass.
> search both
Kaya: Which one do you mean: Doctor Near Cabinet, Doctor Near Door?
> search the doctor near door
Kaya: I search the Doctor Near Door. I find Brass Key. 35 seconds pass.
> take brass key
Kaya: I pick up Brass Key. 5 seconds pass.
> can you call an elevator from here?
Kaya: What do you want Kaya to do?
> use brass key on the emergency stairwell door
Kaya: The key turns in the lock. The Emergency Stairwell Door is unlocked. 8 seconds pass.
> walk north
Kaya: I move north into Emergency Stairwell. A concrete stairwell beyond a locked fire door. 20 seconds pass.
Kaya: I am in the stairwell. This part is clear.
Prototype objective complete.
```

Manual response observations: `debug: unsupported_claim` appeared on multiple turns in all three runs, while deterministic engine facts and objective completion remained correct. No manual invariant failure occurred.

## Findings

Fixed defects:

- Punctuation-adjacent approved entity names no longer fail `unknown_entity`; regression: `TestComposerAcceptsApprovedEntitiesAdjacentToPunctuation`.
- Broad neutral-token acceptance was removed after review. `TestComposerRepairsNeutralParaphraseWithExactFactText` proves strict initial rejection plus one exact-text repair, while `TestComposerRejectsUnfoundedTakeClaim` and direct validator tests reject invented action/state prose.

Final-audit fixes:

- Pure greetings, acknowledgements, and connection chatter bypass model parsing through a conservative local conversation plan; inventory and item fact queries now consume zero game time and cannot fire scheduled events. Greeting-prefixed commands remain executable.
- Response validation approves entities, tokens, and number equivalents only from each sentence's cited facts while retaining bundle-wide required coverage.
- Transition invariants now cover undiscovered takes, flashlight/perception order, post-take rediscovery, intended-door-only unlocking, known exits, observed facts, and scheduled-event chronology.
- Generated and fallback responses preserve ordered sentence/fact evidence, so darkness validation is outcome-local without exempting fallback.
- Live acceptance rejects unknown source, missing raw plans, count mismatches, parser fallback/errors, and response fallback.
- Trailing `please` normalization, both CLI parser-adapter branches, parse/compose deadline cancellation, exact fact-bundle delivery, and direct `MinInt64`/`MaxInt64` option parsing have permanent coverage.

Adversarial coverage included conversational chatter, typos (`isnide`, `cabiner`), interruption, repeated searches, doctor ambiguity with `both`, invalid moon/elevator suggestions, darkness, scheduled sound events, flashlight use, locked-door unlocking, time advancement, autonomy-visible clarification, and objective completion.

## Files

Tracked final-fix changes span `internal/intent`, `internal/actions`, `internal/session`, `internal/response`, `internal/playtest`, `cmd/kaya`, and the milestone/report documentation.

Untracked final-fix evidence: `.superpowers/sdd/final-fix-report.md`.

## Post-Repair Run

Task 6 was resumed from blocked-state commit `7fe2891` with one model-agnostic response validate-and-repair attempt. The validator remains authoritative: the initial and repaired drafts both pass through `validateDraft`, and a failed repair still returns deterministic fallback.

New RED/GREEN regressions:

- `TestComposerAcceptsCitedNumberWordEquivalent` was RED for `Thirty-five seconds pass.` against an approved `35` elapsed-time fact and GREEN after generic cardinal-number equivalence was added. The old ungrounded-prose rejections remain covered by the repair tests.
- `TestComposerRepairsInvalidDraftIntoFactLockedResponse`, `TestComposerFallsBackWhenRepairDraftRemainsInvalid`, `TestComposerFallsBackWhenRepairGenerationFails`, and `TestComposerKeepsValidFirstDraftWithoutRepair` were RED for the missing repair contract and GREEN with exactly one repair call, exact-fact repair input, and explicit provenance.
- `TestRenderMarkdownIncludesReproductionEvidence` was RED for missing response repair provenance and GREEN after the transcript renders attempt/success plus initial/repair reasons and repair generation errors.
- `TestLiveProvenanceSummaryCountsResponseRepairs` was RED for missing repair counters and GREEN after the live summary logs response repair attempts and successes.

Offline verification after the repair update:

```powershell
go test ./internal/playtest -run 'Test(AdversarialPrototypeSessions|PrototypeThousandPhraseVariedSessionsReachObjective)' -v -count=1
go test ./... -count=1
```

PASS: all six adversarial subtests, the 1,000-session proof in `0.45s`, and the full 15-package suite. The default live gate also PASSed by skipping before client construction.

The post-repair live command remained BLOCKED after `1m20.3245446s`:

```powershell
$env:KAYA_LIVE_SLICE_TESTS = '1'
go test ./internal/playtest -run TestOllamaPrototypeCompletePlaythroughs -v -count=1
```

| Seed | Response repair provenance | Exact strict failure |
| --- | --- | --- |
| 10 | 4 attempts, 3 successes, 1 fallback; 6 generated parser turns, parser fallback/errors 0/0 | At storage awareness, initial `unsupported_claim`; repaired draft omitted a required fact and failed `missing_required_fact`. |
| 11 | 4 attempts, 3 successes, 1 fallback; 6 generated parser turns, parser fallback/errors 0/0 | Same storage-awareness repair failure: initial `unsupported_claim`; repair `missing_required_fact`. |
| 12 | 1 attempt, 0 successes, 1 fallback; 3 generated parser turns, parser fallback/errors 0/0 | On `pick up the flashlight then move east`, initial `missing_required_fact`; repaired draft added unsupported prose and failed `unsupported_claim`. |

Seed 10 and 11 repair draft excerpt, retained in the failing transcript:

```json
{"sentences":[{"factIds":["f001"],"text":"I stand in a pitch-black storage room with overturned cabinets and a chemical smell."},{"factIds":["f002","f003"],"text":"Doctor Near Cabinet, Doctor Near Door, Storage Cabinet surround me. I can go west or north."}]}
```

Seed 12 repair draft retained its required-fact schema but added uncited wording including `before I move east`, `fills my view`, and `while I feel uneasy`; strict validation rejected it. Every live failure printed the complete Markdown transcript with response repair provenance. Phase 8 remains BLOCKED.

## Exact-Copy Repair Contract

The one-pass repair contract was narrowed without changing the validator or model configuration. Repair input now exposes `requiredFacts` separately in original bundle order as exact `{id, text}` pairs, carries optional facts separately as context, and instructs the model to emit exactly one sentence per required fact, cite only that ID, and copy that text without additions or rephrasing.

Focused RED/GREEN evidence:

- `TestComposerRepairsInvalidDraftIntoFactLockedResponse` was RED because repair input lacked `requiredFacts`; it is GREEN with ordered exact ID/text assertions.
- `TestComposerFallsBackWhenRepairOmitsRequiredFact` proves that an incomplete exact-copy repair remains deterministic fallback with `missing_required_fact` after exactly two total generator calls.
- Existing repair-invalid tests continue to prove unsupported additions are rejected.

Latest recorded live command:

```powershell
$env:KAYA_LIVE_SLICE_TESTS = '1'
go test ./internal/playtest -run TestOllamaPrototypeCompletePlaythroughs -v -count=1
```

Result: BLOCKED after `1m53.5880495s`. Parser fallback/errors remained `0/0` for every seed.

| Seed | Result | Response provenance |
| --- | --- | --- |
| 10 | BLOCKED at storage awareness: initial `unsupported_claim`, repair `missing_required_fact` | 6 turns; response generated/fallback `5/1`; repair attempts/successes `4/3` |
| 11 | BLOCKED at the same storage-awareness step: initial `unsupported_claim`, repair `missing_required_fact` | 6 turns; response generated/fallback `5/1`; repair attempts/successes `4/3` |
| 12 | PASS | 9 turns; response generated/fallback `9/0`; repair attempts/successes `6/6` |

The exact seed-10 repaired JSON contained the three room facts but omitted the required elapsed-time fact:

```json
{"sentences":[{"factIds":["f001"],"text":"A pitch-black storage room with overturned cabinets and a chemical smell."},{"factIds":["f002"],"text":"I can see: Doctor Near Cabinet, Doctor Near Door, Storage Cabinet."},{"factIds":["f003"],"text":"I can go: west, north."}]}
```

The strict validator correctly rejected this with `missing_required_fact`. No retry, completion renderer, validator exemption, model change, or model-specific wording was added. Phase 8 remains BLOCKED.
