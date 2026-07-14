# Scalable Intent Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace phrase-driven intent correction with evidence-backed typed semantic actions, generic incremental world grounding, one pre-execution repair, and stateful clarification.

**Architecture:** Add the typed path beside the legacy `TurnPlan` path so every task builds and passes independently. Migrate session, CLI, and playtests only after compiler, grounder, executor, and clarification behavior are separately proven; then delete contextual phrase correction and shrink offline fallback.

**Tech Stack:** Go 1.26, standard library JSON/schema handling, Ollama structured generation, existing Kaya world/action/turn/playtest packages.

## Global Constraints

- The model emits mentions and exact source evidence, never authoritative world IDs.
- Semantic repair occurs at most once and finishes before any world mutation.
- Grounding uses current perception incrementally and never chooses an arbitrary first match.
- Room names, object names, item names, and transcript phrases must not appear in production parser rules.
- The offline fallback remains limited to basic movement, room awareness, inventory, quit, and an unavailable-model response.
- Preserve 60-second parse/compose deadlines, fact-locked responses, deterministic world execution, and existing transition invariants.
- Do not stage or overwrite the pre-existing `docs/engine-milestones.md` worktree edit.

---

### Task 1: Model DTO And Typed Semantic Compiler

**Files:**
- Create: `internal/intent/model_plan.go`
- Create: `internal/intent/semantic.go`
- Create: `internal/intent/compiler.go`
- Create: `internal/intent/compiler_test.go`
- Modify: `internal/intent/schema.go`
- Modify: `internal/intent/prompt.go`

**Interfaces:**
- Produces: `ParseModelPlanJSON(string) (ModelTurnPlan, error)`
- Produces: `CompileModelPlan(message string, model ModelTurnPlan) (SemanticPlan, []ValidationError)`
- Produces: typed `SemanticAction` implementations and `SemanticPlan`.

- [ ] **Step 1: Add failing compiler contract tests**

Cover a valid use action, valid compound evidence, missing/forbidden slots, unsupported kinds, evidence absent from the message, and duplicate evidence:

```go
func TestCompileModelPlanRejectsContradictorySlots(t *testing.T) {
    model := ModelTurnPlan{Actions: []ModelAction{{
        Kind: "search", TargetMention: "desk", ItemMention: "flashlight",
        Evidence: "search the desk",
    }}}
    _, problems := CompileModelPlan("search the desk", model)
    requireProblemCode(t, problems, "forbidden_slot")
}

func TestCompileModelPlanBuildsTypedUse(t *testing.T) {
    model := ModelTurnPlan{Actions: []ModelAction{{
        Kind: "use", ItemMention: "the key", TargetMention: "that door",
        Quantity: "one", Evidence: "use the key to unlock that door",
    }}}
    plan, problems := CompileModelPlan("use the key to unlock that door", model)
    if len(problems) != 0 { t.Fatal(problems) }
    if _, ok := plan.Actions[0].(UseAction); !ok { t.Fatalf("%T", plan.Actions[0]) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/intent -run 'TestCompileModelPlan' -count=1 -v`

Expected: FAIL because the model DTO, compiler, and typed actions do not exist.

- [ ] **Step 3: Add DTO, schema, typed actions, and compiler**

Use these core types:

```go
type ModelAction struct {
    Kind, TargetMention, ItemMention, Direction, State, Evidence string
    Quantity TargetMode
}

type ValidationError struct {
    Action int    `json:"action"`
    Field  string `json:"field"`
    Code   string `json:"code"`
    Message string `json:"message"`
}

type SemanticAction interface {
    ActionKind() Action
    SourceEvidence() string
    semanticAction()
}

type Reference struct {
    Mention string
    Quantity TargetMode
}

type UseAction struct {
    Item Reference
    Target Reference
    Evidence string
}

type SemanticPlan struct {
    Actions []SemanticAction
    Questions []FactQuestion
    RawText string
    NeedsClarification bool
    ClarificationQuestion string
}
```

Implement action-specific structs for move, inspect, search, take, use, toggle, wait, talk, listen, and explore. Compile only executable kinds. Normalize whitespace for evidence comparison but require every evidence value to occur in the original message and reject duplicate action evidence.

- [ ] **Step 4: Add decoder-shape and fuzz tests**

```go
func FuzzCompileModelPlanNeverPanics(f *testing.F) {
    f.Add("look around", "inspect", "look around")
    f.Fuzz(func(t *testing.T, message, kind, evidence string) {
        CompileModelPlan(message, ModelTurnPlan{Actions: []ModelAction{{Kind: kind, Evidence: evidence}}})
    })
}
```

Run: `go test ./internal/intent -run 'Test(ParseModelPlan|CompileModelPlan)' -fuzz FuzzCompileModelPlanNeverPanics -fuzztime=2s`

Expected: PASS with no panic and all contract tests green.

- [ ] **Step 5: Commit**

```powershell
git add internal/intent/model_plan.go internal/intent/semantic.go internal/intent/compiler.go internal/intent/compiler_test.go internal/intent/schema.go internal/intent/prompt.go
git commit -m "feat: add typed semantic intent compiler"
```

---

### Task 2: Semantic Parser With One Structured Repair

**Files:**
- Create: `internal/intent/semantic_parser_test.go`
- Modify: `internal/intent/parser.go`
- Modify: `internal/intent/prompt.go`

**Interfaces:**
- Consumes: `ParseModelPlanJSON`, `CompileModelPlan`, `SemanticPlan` from Task 1.
- Produces: `ParseSemanticWithProvenance(context.Context, string, game.PerceptionSnapshot) (SemanticPlan, SemanticProvenance, error)`.

- [ ] **Step 1: Add failing parser lifecycle tests**

Test valid first-pass compilation, invalid first pass followed by valid repair, invalid repair becoming clarification, and exactly two generator calls maximum. Assert the repair payload contains structured validation errors and the original message.

```go
func TestParseSemanticRepairsContractFailureOnce(t *testing.T) {
    generator := &fakeGenerator{responses: []string{invalidSearchWithItemJSON, validSearchJSON}}
    plan, provenance, err := NewParser(generator).ParseSemanticWithProvenance(
        context.Background(), "search the desk", game.PerceptionSnapshot{},
    )
    if err != nil { t.Fatal(err) }
    if generator.calls != 2 || provenance.Source != ParseSourceRepair { t.Fatal(provenance) }
    if _, ok := plan.Actions[0].(SearchAction); !ok { t.Fatalf("%T", plan.Actions[0]) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/intent -run TestParseSemantic -count=1 -v`

Expected: FAIL because semantic parsing and provenance are absent.

- [ ] **Step 3: Implement semantic parse/compile/repair**

Add `SemanticProvenance` with raw DTO, source, validation errors, repair reason, and fallback error. Generate with `ModelTurnPlanSchema`, compile once, and call the repair generator only when decoding or semantic compilation fails. Compile repaired output with the same function. Never call `normalizeContextualPlan` from the typed path.

```go
type SemanticProvenance struct {
    Source ParseSource
    RawPlan ModelTurnPlan
    HasRawPlan bool
    ValidationErrors []ValidationError
    RepairReason error
    FallbackError error
}
```

- [ ] **Step 4: Verify parser package**

Run: `go test ./internal/intent -count=1`

Expected: PASS; legacy parser tests remain green while the typed path has independent coverage.

- [ ] **Step 5: Commit**

```powershell
git add internal/intent/parser.go internal/intent/prompt.go internal/intent/semantic_parser_test.go
git commit -m "feat: validate and repair semantic plans"
```

---

### Task 3: Generic Entity Grounder

**Files:**
- Create: `internal/grounding/types.go`
- Create: `internal/grounding/grounder.go`
- Create: `internal/grounding/grounder_test.go`
- Modify: `internal/world/state.go`
- Modify: `internal/world/perception.go`

**Interfaces:**
- Consumes: typed semantic actions from Task 1 and current `world.State`.
- Produces: `Ground(action intent.SemanticAction, binding *Binding) Result`.
- Produces: candidate-preserving `Clarification` without mutating world state.

- [ ] **Step 1: Add failing synthetic-world tests**

Build worlds with arbitrary renamed objects/items/doors. Cover exact names, aliases, pronouns, plural referents, exact-match precedence over substring matches, missing references, and equal-score ambiguity. Assert grounding does not mutate the world.

```go
func TestGroundUseReturnsDoorAmbiguityWithoutChoosingFirst(t *testing.T) {
    state := syntheticWorldWithTwoDoorsAndKey()
    got := New(state).Ground(intent.UseAction{
        Item: intent.Reference{Mention: "key"},
        Target: intent.Reference{Mention: "door"},
    }, nil)
    if got.Clarification == nil || len(got.Clarification.Candidates) != 2 { t.Fatal(got) }
    if state.Doors[doorA].State != world.DoorLocked { t.Fatal("grounding mutated world") }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/grounding -count=1 -v`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement ranked generic resolution**

Use candidate kinds `object`, `item`, `door`, and `exit`. Rank exact normalized name, exact alias, recent referent, then unique token match. Return all top-scoring ties. Do not iterate a map and select its first match. Resolve only entities permitted by current visibility, discovery, inventory, and known exits.

- [ ] **Step 4: Add property and fuzz coverage**

Fuzz candidate order and aliases; the selected ID or ambiguity set must remain invariant under map/slice ordering.

Run: `go test ./internal/grounding -count=20`

Expected: PASS on every randomized ordering.

- [ ] **Step 5: Commit**

```powershell
git add internal/grounding internal/world/state.go internal/world/perception.go
git commit -m "feat: ground semantic references generically"
```

---

### Task 4: Incremental Typed Execution

**Files:**
- Create: `internal/turn/semantic_executor.go`
- Create: `internal/turn/semantic_executor_test.go`
- Modify: `internal/turn/types.go`
- Modify: `internal/actions/resolver.go`

**Interfaces:**
- Consumes: `intent.SemanticPlan` and `grounding.Ground`.
- Produces: `ExecuteSemantic(plan intent.SemanticPlan, start int, binding *grounding.Binding) SemanticExecution`.
- Produces: `SemanticExecution.Pending` with action index, unresolved role, candidates, and remaining plan.

- [ ] **Step 1: Add failing execution tests**

Cover search-then-take where the item is unavailable before search, move-then-inspect in the destination room, ambiguous second action after a successful first action, and canonical ID selection passed to existing deterministic action behavior.

```go
func TestExecuteSemanticGroundsEachActionAfterPreviousMutation(t *testing.T) {
    state := scenario.NewPrototypeWorld()
    plan := semanticSearchThenTake("desk", "flashlight")
    got := NewExecutor(state).ExecuteSemantic(plan, 0, nil)
    if got.Pending != nil || len(got.Result.Outcomes) != 2 { t.Fatal(got) }
    if !state.HasItem(scenario.ItemFlashlight) { t.Fatal("flashlight not taken") }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/turn -run TestExecuteSemantic -count=1 -v`

Expected: FAIL because typed execution is absent.

- [ ] **Step 3: Implement sequential ground-and-execute**

Before each action, construct a grounder from the current world. Convert grounded IDs to canonical names only at the existing resolver boundary. Stop on clarification without replaying completed outcomes. A binding resumes exactly the pending action. Preserve autonomy, duration, event, fact-bundle, and stop-on-failure semantics.

- [ ] **Step 4: Verify turn and action packages**

Run: `go test ./internal/turn ./internal/actions -count=1`

Expected: PASS, including all legacy execution tests.

- [ ] **Step 5: Commit**

```powershell
git add internal/turn/semantic_executor.go internal/turn/semantic_executor_test.go internal/turn/types.go internal/actions/resolver.go
git commit -m "feat: execute grounded semantic actions incrementally"
```

---

### Task 5: Stateful Generic Clarification

**Files:**
- Create: `internal/intent/clarification.go`
- Create: `internal/session/session.go`
- Create: `internal/session/clarification_test.go`
- Modify: `internal/intent/schema.go`
- Modify: `internal/intent/parser.go`
- Modify: `internal/session/processor.go`

**Interfaces:**
- Produces: `ParseClarification(context.Context, string, []CandidateView) (ClarificationDecision, error)`.
- Produces: `session.New(state, parser, composer) *Session` and `(*Session).ProcessTurn(context.Context, string) (ProcessedTurn, error)`.

- [ ] **Step 1: Add failing multi-turn tests**

Cover selection by exact name, alias, ordinal, plural/all, confirmation, and cancellation. Verify an unrelated new command cancels pending state and parses normally. Verify `yes`, `no`, and `both` are not swallowed as conversation while clarification is pending.

```go
func TestSessionResumesAmbiguousActionByOrdinal(t *testing.T) {
    s := newSessionAtTwoDoctors()
    first := mustTurn(t, s, "search the doctor")
    if first.Pending == nil { t.Fatal("missing clarification") }
    second := mustTurn(t, s, "the second one")
    if second.Result.Outcomes[0].TargetObjectID != scenario.ObjectDoctorNearDoor { t.Fatal(second) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/session -run 'TestSession(Resumes|Cancels|Confirms)' -count=1 -v`

Expected: FAIL because session clarification state is absent.

- [ ] **Step 3: Implement candidate-bound clarification state**

Store the semantic plan, action index, unresolved role, and candidate IDs in `Session`, not `world.State`. Send only candidate ordinal/name/aliases to the model. The decision schema contains `select`, `all`, `confirm`, `cancel`, or `new_command`, plus mention/ordinal. Resolve decisions against stored candidates and resume through `ExecuteSemantic`.

- [ ] **Step 4: Verify no replay or time duplication**

Add assertions that actions before clarification execute once, scheduled events fire once, and duration is counted once across both messages.

Run: `go test ./internal/session ./internal/turn -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/intent/clarification.go internal/intent/schema.go internal/intent/parser.go internal/session/session.go internal/session/processor.go internal/session/clarification_test.go
git commit -m "feat: preserve and resolve genuine clarification state"
```

---

### Task 6: Migrate CLI And Playtest Harness

**Files:**
- Modify: `cmd/kaya/main.go`
- Modify: `cmd/kaya/main_test.go`
- Modify: `internal/playtest/runner.go`
- Modify: `internal/playtest/types.go`
- Modify: `internal/playtest/transcript.go`
- Modify: relevant `internal/playtest/*_test.go`

**Interfaces:**
- Consumes: stateful `session.Session`, semantic parser provenance, and typed execution.
- Produces: semantic `--parse-log` and transcript evidence for raw DTO, validation/repair, grounding, and clarification.

- [ ] **Step 1: Add failing CLI/runner tests**

Assert one session instance survives across console messages, parse logs show typed actions and grounding provenance, playtest snapshots retain pending clarification, and transcript reproduction includes raw model DTO plus structured validation errors.

- [ ] **Step 2: Verify RED**

Run: `go test ./cmd/kaya ./internal/playtest -run 'Test(Semantic|Runner.*Clarification|RenderMarkdown.*Semantic)' -count=1 -v`

Expected: FAIL because CLI and runner still use legacy `TurnPlan` processing.

- [ ] **Step 3: Migrate runtime ownership**

Construct one `session.Session` per play run and one per playtest runner. Replace formatting/cloning helpers with semantic equivalents. Keep seed generation, world validation, response composition, and objective logic unchanged.

- [ ] **Step 4: Verify deterministic gameplay**

Run:

```powershell
go test ./cmd/kaya ./internal/playtest -count=1
go test ./internal/playtest -run 'Test(AdversarialPrototypeSessions|PrototypeThousandPhraseVariedSessionsReachObjective)' -count=1 -v
```

Expected: PASS; all 1,000 sessions and all nine placements complete.

- [ ] **Step 5: Commit**

```powershell
git add cmd/kaya internal/playtest
git commit -m "refactor: run gameplay through semantic sessions"
```

---

### Task 7: Remove Phrase-Driven Parser And Shrink Fallback

**Files:**
- Modify: `internal/intent/parser.go`
- Modify: `internal/intent/fallback.go`
- Modify: `internal/intent/parser_test.go`
- Modify: `internal/intent/intent_corpus_test.go`
- Delete legacy-only parser helpers and schemas after `rg` proves no callers.

**Interfaces:**
- Consumes: fully migrated semantic runtime from Task 6.
- Produces: `MinimalFallbackPlan` limited to the approved emergency command set.

- [ ] **Step 1: Add architecture-failure tests**

Add a test that walks production files in `internal/intent` and rejects scenario constants or known prototype entity names. Add fallback table tests proving only directions, room awareness, inventory, and model-unavailable behavior execute offline; other wording must not silently become gameplay.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/intent -run 'Test(NoScenarioVocabularyInProductionIntent|MinimalFallback)' -count=1 -v`

Expected: FAIL on existing doctor, cabinet, stairwell, flashlight/key, typo, and phrase-canonicalization branches.

- [ ] **Step 3: Delete contextual correction and narrow fallback**

Remove `normalizeContextualPlan`, `canonicalFallbackPlan`, transcript-specific typo substitutions, item/object names, and phrase-specific force rules. Retain only the documented emergency grammar. Convert the recent room-search, plural-doctor, and unlock examples into model DTO/compiler or live-evaluation cases rather than production rules.

- [ ] **Step 4: Verify no hidden second parser remains**

Run:

```powershell
rg -n "doctor|cabinet|stairwell|flashlight|brass key|isnide|cabiner" internal/intent -g "*.go"
go test ./internal/intent ./internal/session ./internal/playtest -count=1
```

Expected: `rg` matches test fixtures only; all tests pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/intent internal/session internal/playtest
git commit -m "refactor: remove phrase-driven intent correction"
```

---

### Task 8: Scalability Proof And Final Acceptance

**Files:**
- Create: `internal/grounding/synthetic_test.go`
- Create: `internal/intent/semantic_corpus_test.go`
- Modify: `internal/intent/ollama_integration_test.go`
- Modify: `internal/playtest/ollama_integration_test.go`
- Modify: `docs/engine-milestones.md` only after preserving and reconciling the user's existing edit.

**Interfaces:**
- Produces: offline scalability proof plus opt-in raw-model acceptance evidence.

- [ ] **Step 1: Add generated synthetic-world proof**

Generate at least 100 worlds with renamed objects, items, doors, aliases, collisions, visibility, and exits. Assert unique references ground to the same semantic roles, ties clarify, missing references do not mutate state, and candidate ordering is deterministic.

- [ ] **Step 2: Add held-out semantic corpus**

Create at least 100 paraphrases spanning every executable action, compounds, questions, pronouns, explicit plural targets, interruptions, and genuine ambiguity. Score raw DTO compilation separately from repaired compilation. Do not use deterministic phrase correction in scoring.

- [ ] **Step 3: Define and run offline gates**

Run:

```powershell
go test ./... -count=1
go vet ./...
go test ./internal/grounding -count=20
go test ./internal/playtest -run 'Test(AdversarialPrototypeSessions|PrototypeThousandPhraseVariedSessionsReachObjective)' -count=1 -v
```

Expected: all pass; 100 synthetic worlds and 1,000 gameplay sessions report zero invariant violations.

- [ ] **Step 4: Run opt-in Ollama semantic gate**

Run with `qwen3.5:4b` and unchanged 60-second deadlines:

```powershell
$env:KAYA_LIVE_SEMANTIC_TESTS = '1'
go test ./internal/intent -run TestOllamaHeldOutSemanticCorpus -count=1 -v
```

Acceptance: at least 90% of held-out messages compile from raw model DTOs, 100% compile after the single repair or safely clarify, zero unsupported actions execute, and no phrase-specific canonicalization contributes to the score.

- [ ] **Step 5: Run complete live playthrough gate**

Run:

```powershell
$env:KAYA_LIVE_SLICE_TESTS = '1'
go test ./internal/playtest -run TestOllamaPrototypeCompletePlaythroughs -count=1 -v
```

Acceptance: all configured seeds complete with model/repair provenance, exact raw/resolved counts, and zero parser or response fallback. If this unchanged strict gate fails, keep Phase 8 blocked and record exact evidence rather than weakening policy.

- [ ] **Step 6: Commit evidence and milestone status**

Stage only reviewed files after reconciling the pre-existing milestone edit:

```powershell
git add internal/grounding/synthetic_test.go internal/intent/semantic_corpus_test.go internal/intent/ollama_integration_test.go internal/playtest/ollama_integration_test.go docs/engine-milestones.md
git commit -m "test: prove scalable semantic intent pipeline"
```

---

## Final Review Gate

- [ ] Review the complete design range for Critical/Important defects.
- [ ] Fix every Critical/Important finding and rerun the reviewer.
- [ ] Run `go test ./... -count=1`, `go vet ./...`, `git diff --check`, and `git status --short` from the final clean state.
- [ ] Confirm the goal only when typed parsing, one repair, generic grounding, stateful clarification, phrase-rule removal, offline proof, and live semantic acceptance all have direct evidence.
