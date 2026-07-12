# Prototype Vertical Slice Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a reusable stateful playtest harness and use it to prove the complete Reception to Storage Room to Emergency Stairwell escape loop across 1,000 deterministic sessions and fixed live-model playthroughs.

**Architecture:** Extract the shared turn-processing path from the CLI into `internal/session`. Build `internal/playtest` around generated runs, immutable snapshots, transition invariants, reviewed phrase banks, and Markdown transcript rendering; the engine remains authoritative and Ollama tests stay opt-in.

**Tech Stack:** Go 1.26 standard library, existing `internal/intent`, `internal/turn`, `internal/response`, `internal/rungen`, and Ollama adapters.

## Global Constraints

- Do not add rooms, items, puzzles, monsters, endings, persistence, cloud inference, or model-specific prompt changes.
- Cover Reception, Storage Room, Emergency Stairwell, flashlight, brass key, locked door, time, scheduled events, existing autonomy, and objective completion.
- Exercise all nine valid flashlight/key placement combinations.
- Run at least 1,000 ordinary deterministic sessions without Ollama.
- Keep live Ollama sessions opt-in and expose raw/resolved plans plus generator, repair, and fallback provenance.
- Check invariants after every turn and stop immediately with a reproducible transcript on failure.
- Preserve the existing console behavior and output unless a failing regression proves it incorrect.
- Add every reproduced engine defect as a failing test before changing production behavior.
- Finish with no unresolved Critical or Important findings.

---

### Task 1: Shared Turn Processor

**Files:**
- Create: `internal/session/processor.go`
- Create: `internal/session/processor_test.go`
- Modify: `cmd/kaya/main.go`
- Test: `internal/session/processor_test.go`
- Test: `cmd/kaya/main_test.go`

**Interfaces:**
- Consumes: `intent.Parser.ParseWithProvenance`, `turn.NewExecutor`, `response.Composer.Compose`, and `world.State.PerceptionSnapshot`.
- Produces: `session.Parser`, `session.Composer`, `session.ProcessedTurn`, `session.ProcessTurn`, and `session.ResultDuration`.

- [ ] **Step 1: Write a failing processor test**

Create a deterministic parser and composer in `internal/session/processor_test.go`:

```go
type fallbackParser struct{}

func (fallbackParser) ParseWithProvenance(
    _ context.Context,
    message string,
    _ game.PerceptionSnapshot,
) (intent.TurnPlan, intent.ParseProvenance, error) {
    plan := intent.FallbackPlan(message)
    return plan, intent.ParseProvenance{
        Source: intent.ParseSourceFallback,
        RawPlan: plan,
        HasRawPlan: true,
    }, nil
}

type fallbackComposer struct{}

func (fallbackComposer) Compose(_ context.Context, bundle turn.FactBundle) response.Response {
    return response.NewComposer(nil).Compose(context.Background(), bundle)
}

func TestProcessTurnUsesSharedStateAndCapturesProvenance(t *testing.T) {
    state := scenario.NewPrototypeWorld()
    got, err := ProcessTurn(context.Background(), "go east", state, fallbackParser{}, fallbackComposer{})
    if err != nil {
        t.Fatal(err)
    }
    if got.Plan.Actions[0].Intent.Action != intent.ActionMove {
        t.Fatalf("plan = %#v", got.Plan)
    }
    if got.Provenance.Source != intent.ParseSourceFallback {
        t.Fatalf("provenance = %#v", got.Provenance)
    }
    if state.CurrentRoomID != scenario.RoomStorage {
        t.Fatalf("room = %q", state.CurrentRoomID)
    }
    if got.DurationSeconds != 20 || state.NowSeconds != 20 {
        t.Fatalf("duration=%d time=%d", got.DurationSeconds, state.NowSeconds)
    }
}
```

- [ ] **Step 2: Verify RED**

Run:

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go test ./internal/session -run TestProcessTurnUsesSharedStateAndCapturesProvenance -v
```

Expected: build FAIL because `ProcessTurn` and its package do not exist.

- [ ] **Step 3: Implement the processor**

Create `internal/session/processor.go`:

```go
package session

import (
    "context"
    "fmt"
    "time"

    "kaya/internal/game"
    "kaya/internal/intent"
    "kaya/internal/response"
    "kaya/internal/turn"
    "kaya/internal/world"
)

type Parser interface {
    ParseWithProvenance(context.Context, string, game.PerceptionSnapshot) (intent.TurnPlan, intent.ParseProvenance, error)
}

type Composer interface {
    Compose(context.Context, turn.FactBundle) response.Response
}

type ProcessedTurn struct {
    Plan            intent.TurnPlan
    Provenance      intent.ParseProvenance
    Result          turn.Result
    Response        response.Response
    DurationSeconds int
}

func ProcessTurn(ctx context.Context, message string, state *world.State, parser Parser, composer Composer) (ProcessedTurn, error) {
    if state == nil || parser == nil || composer == nil {
        return ProcessedTurn{}, fmt.Errorf("session dependencies must not be nil")
    }
    snapshot, err := state.PerceptionSnapshot()
    if err != nil {
        return ProcessedTurn{}, fmt.Errorf("snapshot world: %w", err)
    }
    parseCtx, cancelParse := context.WithTimeout(ctx, 60*time.Second)
    plan, provenance, err := parser.ParseWithProvenance(parseCtx, message, snapshot)
    cancelParse()
    if err != nil {
        return ProcessedTurn{}, err
    }
    result := turn.NewExecutor(state).Execute(plan)
    responseCtx, cancelResponse := context.WithTimeout(ctx, 60*time.Second)
    composed := composer.Compose(responseCtx, result.FactBundle(message))
    cancelResponse()
    return ProcessedTurn{
        Plan: plan, Provenance: provenance, Result: result, Response: composed,
        DurationSeconds: ResultDuration(result),
    }, nil
}

func ResultDuration(result turn.Result) int {
    total := 0
    for _, outcome := range result.Outcomes {
        total += outcome.Result.DurationSeconds
    }
    return total
}
```

- [ ] **Step 4: Delegate the CLI wrapper to the shared processor**

In `cmd/kaya/main.go`, import `kaya/internal/session`, alias `processedTurn` to `session.ProcessedTurn`, and replace the body of `processPlayerTurn`:

```go
type processedTurn = session.ProcessedTurn

func processPlayerTurn(ctx context.Context, message string, state *world.State, parser turnParser, _ turn.Executor, composer responseComposer) (processedTurn, error) {
    adapter := provenanceParser{parser: parser}
    return session.ProcessTurn(ctx, message, state, adapter, composer)
}

type provenanceParser struct{ parser turnParser }

func (p provenanceParser) ParseWithProvenance(ctx context.Context, message string, snapshot game.PerceptionSnapshot) (intent.TurnPlan, intent.ParseProvenance, error) {
    if parser, ok := p.parser.(interface {
        ParseWithProvenance(context.Context, string, game.PerceptionSnapshot) (intent.TurnPlan, intent.ParseProvenance, error)
    }); ok {
        return parser.ParseWithProvenance(ctx, message, snapshot)
    }
    plan, err := p.parser.Parse(ctx, message, snapshot)
    return plan, intent.ParseProvenance{}, err
}
```

Change `resultDuration` to delegate to `session.ResultDuration`. Retain its name so current CLI tests continue to compile.

- [ ] **Step 5: Verify GREEN**

Run:

```powershell
go test ./internal/session -v
go test ./cmd/kaya
```

Expected: PASS with unchanged CLI behavior.

- [ ] **Step 6: Commit**

```powershell
git add internal/session/processor.go internal/session/processor_test.go cmd/kaya/main.go
git commit -m "refactor: share turn processing with playtests"
```

---

### Task 2: Stateful Runner, Snapshots, and Invariants

**Files:**
- Create: `internal/playtest/types.go`
- Create: `internal/playtest/snapshot.go`
- Create: `internal/playtest/runner.go`
- Create: `internal/playtest/invariants.go`
- Create: `internal/playtest/runner_test.go`
- Create: `internal/playtest/invariants_test.go`
- Create: `internal/playtest/test_helpers_test.go`

**Interfaces:**
- Consumes: `session.ProcessTurn` and `rungen.GeneratedRun`.
- Produces: `playtest.Runner`, `playtest.Session`, `playtest.Step`, `playtest.Snapshot`, `playtest.Violation`, `playtest.Capture`, `playtest.CheckState`, and `playtest.CheckTransition`.

- [ ] **Step 1: Write failing snapshot and transition tests**

Add tests that require:

```go
func TestCaptureAndCheckTransitionAcceptValidMove(t *testing.T) {
    generated := mustGeneratedRun(t, 1)
    runner := NewRunner(runscenario.PrototypeDefinition(), generated, fallbackParser{}, fallbackComposer{})
    step, err := runner.Step(context.Background(), "go east")
    if err != nil {
        t.Fatal(err)
    }
    if violations := CheckTransition(runscenario.PrototypeDefinition(), step); len(violations) != 0 {
        t.Fatalf("violations = %#v", violations)
    }
}

func TestCheckStateRejectsDuplicatedItem(t *testing.T) {
    state := scenario.NewPrototypeWorld()
    state.AddInventory(scenario.ItemFlashlight)
    violations := CheckState(state)
    if !hasViolation(violations, "item_multiple_locations") {
        t.Fatalf("violations = %#v", violations)
    }
}

func TestClarificationCannotAdvanceTimeOrMutateWorld(t *testing.T) {
    generated := mustGeneratedRun(t, 2)
    runner := NewRunner(runscenario.PrototypeDefinition(), generated, fallbackParser{}, fallbackComposer{})
    step, err := runner.Step(context.Background(), "do it")
    if err != nil {
        t.Fatal(err)
    }
    if violations := CheckTransition(runscenario.PrototypeDefinition(), step); len(violations) != 0 {
        t.Fatalf("violations = %#v", violations)
    }
    if step.After.Time != step.Before.Time {
        t.Fatalf("time changed: %#v", step)
    }
}
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/playtest -run 'TestCapture|TestCheckState|TestClarification' -v`.

Expected: build FAIL because the playtest package API is missing.

- [ ] **Step 3: Implement structured records**

In `types.go` define:

```go
type Snapshot struct {
    CurrentRoom, PreviousRoom game.RoomID
    Time int
    Inventory, Discovered []game.ItemID
    ObjectItems map[game.ObjectID][]game.ItemID
    DoorStates map[game.DoorID]world.DoorState
    RemainingEventTimes []int
    ActiveLight bool
    Kaya kaya.State
}

type Step struct {
    Number int
    Player string
    Before Snapshot
    Turn session.ProcessedTurn
    After Snapshot
    ObjectiveEmitted bool
    Violations []Violation
}

type Session struct {
    ScenarioID string
    ScenarioVersion, GeneratorVersion int
    Seed int64
    Placements []rungen.Placement
    Steps []Step
    ObjectiveEmissions int
}

type Violation struct { Code, Detail string }
```

- [ ] **Step 4: Implement deterministic capture**

`Capture` sorts every map-derived slice by ID, deep-copies contained items and door states, and records remaining event trigger times in ascending order. Add `SameWorld` comparing room, inventory, discovery, contained items, doors, and light while intentionally ignoring Kaya emotion and clock.

- [ ] **Step 5: Implement Runner**

`Runner` stores definition, generated run, parser, composer, session, and objective state:

```go
func NewRunner(def rungen.Definition, run rungen.GeneratedRun, parser session.Parser, composer session.Composer) *Runner
func (r *Runner) Step(ctx context.Context, message string) (Step, error)
func (r *Runner) Session() Session
func (r *Runner) State() *world.State
```

`Step` captures before, calls `session.ProcessTurn`, captures after, emits the objective only on the first transition into `def.WinRoom`, runs both invariant functions, appends the record, and returns an error containing violation codes when any invariant fails.

- [ ] **Step 6: Implement invariant checks**

`CheckState` verifies existing rooms, portable inventory, unique item location, remaining events after current time, and sorted event times.

`CheckTransition(def rungen.Definition, step Step)` verifies:

```go
expectedTime := step.Before.Time + step.Turn.DurationSeconds
if step.After.Time != expectedTime { violation("time_duration_mismatch", ...) }
if clarificationOrRefusal(step.Turn.Result) && !SameWorld(step.Before, step.After) {
    violation("nonexecuted_action_mutated_world", ...)
}
if step.ObjectiveEmitted && step.After.CurrentRoom != def.WinRoom {
    violation("objective_outside_win_room", ...)
}
```

Also check taken-item removal, locked movement, single scheduled-event emission, and objective count through the structured outcome and snapshot data.

In `test_helpers_test.go` define `mustGeneratedRun`, a local
`fallbackParser` implementing `session.Parser` with
`intent.FallbackPlan`, a deterministic fallback composer, and
`hasViolation`. These helpers are shared only by `internal/playtest` tests and
do not enter production APIs.

- [ ] **Step 7: Verify GREEN**

Run `go test ./internal/playtest -run 'TestCapture|TestCheckState|TestClarification' -v` and then `go test ./internal/playtest`.

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add internal/playtest
git commit -m "test: add stateful playtest runner and invariants"
```

---

### Task 3: Phrase Banks and 1,000 Winning Sessions

**Files:**
- Create: `internal/playtest/phrases.go`
- Create: `internal/playtest/prototype.go`
- Create: `internal/playtest/prototype_test.go`

**Interfaces:**
- Consumes: `playtest.Runner` and prototype placements.
- Produces: `PhraseBank`, `PrototypeWinningMessages`, and `RunPrototypeSession`.

- [ ] **Step 1: Write the failing 1,000-session test**

```go
func TestPrototypeThousandPhraseVariedSessionsReachObjective(t *testing.T) {
    placementsSeen := map[string]bool{}
    for seed := int64(1); seed <= 1000; seed++ {
        run := mustGeneratedRun(t, seed)
        placementsSeen[placementKey(run.Placements)] = true
        runner := NewRunner(runscenario.PrototypeDefinition(), run, fallbackParser{}, fallbackComposer{})
        messages, err := PrototypeWinningMessages(run, seed)
        if err != nil {
            t.Fatalf("seed %d: %v", seed, err)
        }
        for _, message := range messages {
            if _, err := runner.Step(context.Background(), message); err != nil {
                t.Fatalf("seed %d message %q: %v\nsession=%#v", seed, message, err, runner.Session())
            }
        }
        got := runner.Session()
        if runner.State().CurrentRoomID != scenario.RoomStairwell || got.ObjectiveEmissions != 1 {
            t.Fatalf("seed %d did not finish\nsession=%#v", seed, got)
        }
    }
    if len(placementsSeen) != 9 {
        t.Fatalf("covered %d placement combinations, want 9", len(placementsSeen))
    }
}
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/playtest -run TestPrototypeThousandPhraseVariedSessionsReachObjective -v`.

Expected: build FAIL because phrase and prototype helpers are missing.

- [ ] **Step 3: Define reviewed phrase banks**

Create immutable banks:

```go
var PrototypePhrases = PhraseBank{
    Awareness: []string{"look around", "what do you see", "whats around you", "is there anything around you"},
    Search: []string{"search the %s", "check the %s", "look inside the %s"},
    TakeFlashlight: []string{"take the flashlight", "grab the flashlight", "pick up the flashlight", "took the flashlight"},
    MoveEast: []string{"go east", "move east", "head east", "walk east"},
    LightOn: []string{"turn on the flashlight", "switch on the torch", "activate the light"},
    TakeKey: []string{"take the key", "grab the brass key", "pick up the key", "took the key"},
    Unlock: []string{"use the key on the emergency stairwell door", "try the key on the stairwell door"},
    MoveNorth: []string{"go north", "move north", "head north"},
}
```

Use a local SplitMix64 selector so phrase selection is stable across Go versions.

- [ ] **Step 4: Build placement-aware winning messages**

Resolve placement object names through `run.State.Objects` and emit this semantic sequence:

1. Awareness in Reception.
2. Search the flashlight placement.
3. Take flashlight.
4. Move east.
5. Activate flashlight.
6. Awareness in Storage.
7. Search the key placement.
8. Take key.
9. Use key on stairwell door.
10. Move north.

For one quarter of seeds, combine steps 3 and 4; for another quarter, combine steps 5 and 6. Compound text preserves that order.

- [ ] **Step 5: Verify GREEN and runtime**

Run:

```powershell
Measure-Command { go test ./internal/playtest -run TestPrototypeThousandPhraseVariedSessionsReachObjective -count=1 }
```

Expected: PASS, all nine placements observed, and completion fast enough for the ordinary suite. If runtime exceeds 10 seconds, profile before changing the 1,000-session requirement.

- [ ] **Step 6: Commit**

```powershell
git add internal/playtest/phrases.go internal/playtest/prototype.go internal/playtest/prototype_test.go
git commit -m "test: prove one thousand prototype sessions"
```

---

### Task 4: Adversarial and Conversation Invariants

**Files:**
- Create: `internal/playtest/adversarial_test.go`
- Create: `internal/playtest/response.go`
- Create: `internal/playtest/response_test.go`
- Modify: only the owning engine file when a new failing regression demonstrates a defect.

**Interfaces:**
- Consumes: Runner, snapshots, prototype phrase helpers.
- Produces: `CheckResponse` and permanent adversarial session regressions.

- [ ] **Step 1: Add table-driven adversarial sessions**

Define exact scripts and terminal assertions for:

```go
var adversarialCases = []struct {
    name string
    prepare func(*Runner)
    messages []string
    wantRoom game.RoomID
    wantOutcome string
}{
    {"take-before-discovery", nil, []string{"take the flashlight"}, scenario.RoomReception, "item_not_found"},
    {"locked-door-does-not-move", storageWithLight, []string{"go north"}, scenario.RoomStorage, "door_blocked"},
    {"dark-inspection-hides-objects", nil, []string{"go east", "look around"}, scenario.RoomStorage, "inspected_room"},
    {"ambiguous-doctor-remembers-both", storageWithLight, []string{"search the doctors", "both"}, scenario.RoomStorage, "searched_empty"},
    {"failed-first-action-stops-compound", nil, []string{"take the key and go east"}, scenario.RoomReception, "item_not_found"},
    {"repeated-search-after-take", nil, []string{"search the reception desk", "take the flashlight", "search the reception desk"}, scenario.RoomReception, "searched_empty"},
}
```

Use a fixed prototype world where necessary so each expected outcome is stable. Assert time, inventory, discovery, light, room, and door state after every step.

- [ ] **Step 2: Verify RED or establish characterization**

Run `go test ./internal/playtest -run TestAdversarial -v`.

Expected: each new test must either fail for a reproduced engine defect or pass as a characterization. Do not alter production code for a passing characterization.

- [ ] **Step 3: Add response invariant tests**

`CheckResponse(step Step, state *world.State) []Violation` rejects:

- Prefixes matching `(?i)^kaya\b`.
- Non-fallback responses whose `UsedFactIDs` are not in the turn fact bundle.
- Clarification responses that advanced time.
- Pitch-black room-awareness responses containing hidden object names or `north` before the light is active.
- Debug markers such as `debug:` in normal response text.

Add focused tests with synthetic Step values for each violation and one valid fallback response.

- [ ] **Step 4: Apply response checks in Runner**

After `session.ProcessTurn`, append `CheckResponse` results before storing the step. Include violation code and offending response text.

- [ ] **Step 5: Run the adversarial and 1,000-session suites**

```powershell
go test ./internal/playtest -run 'TestAdversarial|TestResponse|TestPrototypeThousand' -count=1 -v
```

Expected: PASS. Any newly reproduced engine defect follows RED-GREEN in its owning package, followed by this complete command.

- [ ] **Step 6: Commit**

```powershell
git add internal/playtest internal/actions internal/turn internal/response internal/world
git commit -m "test: harden prototype adversarial behavior"
```

Stage only files actually changed; do not create empty or unrelated engine edits.

---

### Task 5: Failure Transcript Renderer and CLI Playtest Reuse

**Files:**
- Create: `internal/playtest/transcript.go`
- Create: `internal/playtest/transcript_test.go`
- Modify: `cmd/kaya/main.go`
- Modify: `cmd/kaya/main_test.go`

**Interfaces:**
- Consumes: `playtest.Session` and `playtest.Step`.
- Produces: `playtest.RenderMarkdown` and CLI logs containing seed, placements, provenance, full snapshots, violations, and objective emissions.

- [ ] **Step 1: Write a failing transcript test**

Build a two-step synthetic session and assert output contains:

```go
for _, expected := range []string{
    "# Kaya Stateful Playtest",
    "Seed: `42`",
    "Flashlight",
    "Player: `look around`",
    "Parse source:",
    "Before:",
    "After:",
    "Objective emissions:",
} {
    if !strings.Contains(got, expected) { t.Fatalf("missing %q:\n%s", expected, got) }
}
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/playtest -run TestRenderMarkdown -v`.

Expected: build FAIL because `RenderMarkdown` is missing.

- [ ] **Step 3: Implement stable Markdown rendering**

Render fields in deterministic order. Include scenario/generator versions, seed, sorted placements, each message, raw/resolved action summaries, provenance source and errors, outcomes, events, response metadata, before/after snapshots, state diff, violations, and objective count. Escape player text and response text as fenced blocks rather than inline code.

- [ ] **Step 4: Replace duplicate CLI log formatting**

Keep `writePlaytestLog` as a compatibility wrapper, but have new stateful scripts call `playtest.RenderMarkdown`. Add `--seed` support to `playtest` using the existing play option parser and include the seed in the output filename. Do not alter `play` behavior.

- [ ] **Step 5: Verify GREEN**

Run:

```powershell
go test ./internal/playtest -run TestRenderMarkdown -v
go test ./cmd/kaya
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal/playtest/transcript.go internal/playtest/transcript_test.go cmd/kaya/main.go cmd/kaya/main_test.go
git commit -m "feat: render reproducible stateful playtests"
```

---

### Task 6: Live Playthroughs, Manual Runs, and Final Report

**Files:**
- Create: `internal/playtest/ollama_integration_test.go`
- Create: `docs/prototype-vertical-slice-report.md`
- Modify: `docs/engine-milestones.md`
- Test: all packages.

**Interfaces:**
- Consumes: all playtest APIs from Tasks 1-5.
- Produces: gated complete Ollama sessions and the final hardening report.

- [ ] **Step 1: Add the gated live test**

`TestOllamaPrototypeCompletePlaythroughs` skips unless `KAYA_LIVE_SLICE_TESTS` is truthy. For seeds `10`, `11`, and `12`:

- Generate the prototype run.
- Build placement-aware messages.
- Use `intent.NewParser` with the configured Ollama client.
- Use `response.NewComposer` with the same client.
- Execute the complete session.
- Require stairwell state and exactly one objective emission.
- Require zero `ParseSourceFallback` turns and zero provenance fallback errors.
- Log raw/resolved canonicalization counts and render the transcript on failure.

- [ ] **Step 2: Verify default skip**

Run:

```powershell
Remove-Item Env:KAYA_LIVE_SLICE_TESTS -ErrorAction SilentlyContinue
go test ./internal/playtest -run TestOllamaPrototypeCompletePlaythroughs -v
```

Expected: PASS with SKIP and no Ollama calls.

- [ ] **Step 3: Run all offline verification**

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go test ./...
go vet ./...
git diff --check
```

Expected: PASS with the 1,000-session test included.

- [ ] **Step 4: Run live playthrough verification**

```powershell
$env:KAYA_LIVE_SLICE_TESTS = "1"
go test ./internal/playtest -run TestOllamaPrototypeCompletePlaythroughs -v -count=1
```

Expected: all three seeds complete, zero hidden generator/decoding fallbacks, and failure output includes complete transcripts.

- [ ] **Step 5: Complete three manual console runs**

Run `go run ./cmd/kaya play --seed 10`, then seeds `11` and `12`. Use conversational variants, typos, interruptions, repeated searches, ambiguity, and invalid suggestions. Complete each run without copying the exact automated transcript.

Record the seed, placements, player transcript, completion result, discovered defects, and residual wording observations in `docs/prototype-vertical-slice-report.md`.

- [ ] **Step 6: Write the final report and milestone status**

The report contains:

- Nine placement combinations covered.
- 1,000-session command, duration, and pass count.
- Adversarial case list.
- Live seeds and provenance summary.
- Manual seeds and completion results.
- Fixed defects with regression-test names.
- Remaining Minor findings.
- Explicit statement that no Critical or Important findings remain.

Update Phase 8 in `docs/engine-milestones.md` to `Complete for the hardened prototype vertical slice` only when every acceptance criterion is evidenced.

- [ ] **Step 7: Final verification**

Run fresh:

```powershell
go test ./...
go vet ./...
git diff --check
git status --short
```

Expected: tests and vet pass; only the report and milestone documentation are uncommitted.

- [ ] **Step 8: Commit**

```powershell
git add internal/playtest/ollama_integration_test.go docs/prototype-vertical-slice-report.md docs/engine-milestones.md
git commit -m "test: certify hardened prototype vertical slice"
```
