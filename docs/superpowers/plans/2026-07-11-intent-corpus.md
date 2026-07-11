# Shared Intent Corpus Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one approximately 50-case natural-language corpus that verifies exact normalized intent plans deterministically and evaluates the configured Ollama model on demand.

**Architecture:** A test-only corpus in package `intent_test` defines semantic expectations independently of confidence and raw model text. A normal unit test passes every case through `intent.FallbackPlan`; an opt-in integration test passes the same cases through `intent.Parser`, collects all mismatches, and enforces 90 percent exact-match accuracy.

**Tech Stack:** Go 1.26, standard `testing` package, existing `internal/intent` parser, existing `internal/llm` Ollama client.

## Global Constraints

- The deterministic suite must run without Ollama as part of `go test ./...`.
- The live suite runs only when `KAYA_OLLAMA_EVAL` is truthy.
- Both suites consume exactly the same corpus.
- Compare action, target, item, direction, modifiers, target mode, questions, and clarification state; ignore confidence, raw text, and clarification wording.
- The live suite processes all cases before reporting failure.
- The live exact-match threshold is 90 percent.
- Corpus entries are hand-curated and must have explicit expected plans.

---

### Task 1: Shared semantic corpus and deterministic contract

**Files:**
- Create: `internal/intent/intent_corpus_test.go`
- Modify: `internal/intent/fallback.go`
- Test: `internal/intent/intent_corpus_test.go`

**Interfaces:**
- Consumes: `intent.FallbackPlan(string) intent.TurnPlan`, `intent.Action`, `intent.TargetMode`, and `game.FactKind`.
- Produces: package-local `intentCorpus`, `semanticPlanFrom`, `compareSemanticPlans`, and `validateSemanticPlan`, reused by the Ollama evaluation in Task 2.

- [ ] **Step 1: Create the corpus types and exact semantic projection**

Create `internal/intent/intent_corpus_test.go` in package `intent_test` with these types and helpers:

```go
type corpusAction struct {
	Action     intent.Action
	Target     string
	Item       string
	Direction  string
	Modifiers  []string
	TargetMode intent.TargetMode
}

type corpusQuestion struct {
	Kind       game.FactKind
	Target     string
	TargetMode intent.TargetMode
}

type corpusPlan struct {
	Actions            []corpusAction
	Questions          []corpusQuestion
	NeedsClarification bool
}

type intentCorpusCase struct {
	Name    string
	Message string
	Want    corpusPlan
}

func action(kind intent.Action, target string) corpusAction {
	return corpusAction{Action: kind, Target: target, TargetMode: intent.TargetSingle}
}

func itemAction(kind intent.Action, item string) corpusAction {
	return corpusAction{Action: kind, Item: item, TargetMode: intent.TargetSingle}
}

func move(direction string) corpusAction {
	return corpusAction{Action: intent.ActionMove, Direction: direction, TargetMode: intent.TargetSingle}
}

func semanticPlanFrom(plan intent.TurnPlan) corpusPlan {
	got := corpusPlan{NeedsClarification: plan.NeedsClarification}
	for _, planned := range plan.Actions {
		got.Actions = append(got.Actions, corpusAction{
			Action: planned.Intent.Action, Target: planned.Intent.Target,
			Item: planned.Intent.Item, Direction: planned.Intent.Direction,
			Modifiers: append([]string(nil), planned.Intent.Modifiers...),
			TargetMode: planned.TargetMode,
		})
	}
	for _, question := range plan.Questions {
		got.Questions = append(got.Questions, corpusQuestion{
			Kind: question.Kind, Target: question.Target, TargetMode: question.TargetMode,
		})
	}
	return got
}
```

Use `reflect.DeepEqual` in `compareSemanticPlans(want, got corpusPlan) string`; return an empty string for equality and otherwise return `fmt.Sprintf("want: %#v\n got: %#v", want, got)`. Before comparison, pass both plans through `normalizeCorpusPlan`, which replaces nil action, question, and modifier slices with empty slices. This makes structurally equivalent expectations compare identically without weakening any semantic field.

Add `validateSemanticPlan(plan corpusPlan) error` that rejects more than four actions or questions, invalid actions, invalid target modes, `TargetAll` on actions other than inspect/search, executable plans with zero actions, and clarification plans whose only action is not `ActionUnknown`.

- [ ] **Step 2: Add the curated corpus**

Define `var intentCorpus = []intentCorpusCase{...}` with the following exact messages and expectations. All omitted target modes are `TargetSingle`; all omitted fields and modifier lists are empty.

```text
01 "Look around."                                  -> inspect
02 "whats around you"                              -> inspect
03 "What do you see?"                              -> inspect
04 "Is there anything around you?"                 -> inspect
05 "Inspect the room."                             -> inspect
06 "look at the reception desk"                    -> inspect target="reception desk"
07 "look on the desk"                              -> inspect target="desk"
08 "inspect the storage cabinet"                   -> inspect target="storage cabinet"
09 "look over the floor"                           -> inspect target="floor"
10 "what is on the desk"                           -> inspect target="desk"
11 "search the desk"                               -> search target="desk"
12 "check the drawers"                             -> search target="drawers"
13 "rummage through the cabinet"                   -> search target="cabinet"
14 "look through the doctor's coat"                -> search target="doctor's coat"
15 "is something inside the drawers"               -> search target="drawers"
16 "is there anything in the cabinet"              -> search target="cabinet"
17 "what's in your bag"                            -> talk target="inventory"
18 "do you have anything useful on you"            -> talk target="inventory"
19 "what are you carrying"                         -> talk target="inventory"
20 "do ypou have flashlight"                       -> talk item="flashlight"
21 "is there a flashlight"                         -> talk item="flashlight"
22 "where is the key"                              -> talk item="key"
23 "have you found the key"                        -> talk item="key"
24 "take the flashlight"                           -> take_item target="flashlight"
25 "grab the key"                                  -> take_item target="key"
26 "pick up the brick"                             -> take_item target="brick"
27 "took the key"                                  -> take_item target="key"
28 "turn on the flashlight"                        -> turn_on item="flashlight"
29 "switch on your torch"                          -> turn_on item="flashlight"
30 "turn off the light"                            -> turn_off item="flashlight"
31 "go east"                                       -> move direction="east"
32 "move north"                                    -> move direction="north"
33 "head west"                                     -> move direction="west"
34 "walk back"                                     -> move direction="back"
35 "north"                                         -> move direction="north"
36 "stay still"                                    -> wait
37 "wait here"                                     -> wait
38 "pause for a moment"                            -> wait
39 "listen at the door"                            -> listen target="door"
40 "get behind the cabinet and hide"               -> hide target="cabinet"
41 "throw the brick down the hallway"              -> throw target="hallway" item="brick"
42 "use the key on the emergency stairwell door"   -> use_item target="emergency stairwell door" item="key"
43 "feel along the walls for another exit"         -> explore
44 "run your hands along the wall"                  -> explore
45 "both"                                          -> search target="both" targetMode=all
46 "search the floor and take the flashlight"      -> search target="floor"; take_item target="flashlight"
47 "take the flashlight and go east"               -> take_item target="flashlight"; move direction="east"
48 "turn on the flashlight and look around"        -> turn_on item="flashlight"; inspect
49 "search the doctors are they dead"              -> search target="doctors" targetMode=all; question life_status target="doctors" targetMode=all
50 "what do you have in mind"                      -> unknown needsClarification=true
51 "do it"                                         -> unknown needsClarification=true
52 "search the doctor near the cabiner"             -> search target="doctor near the cabinet"
```

For case 40, store the two intended operations as one `hide` action because “get behind” describes the hiding method, not a separate movement command. For case 49, use `game.FactLifeStatus`.

- [ ] **Step 3: Write the deterministic test and verify RED**

Add:

```go
func TestDeterministicIntentCorpus(t *testing.T) {
	for _, tc := range intentCorpus {
		t.Run(tc.Name, func(t *testing.T) {
			got := semanticPlanFrom(intent.FallbackPlan(tc.Message))
			if err := validateSemanticPlan(got); err != nil {
				t.Fatalf("invalid semantic plan: %v; plan: %#v", err, got)
			}
			if diff := compareSemanticPlans(tc.Want, got); diff != "" {
				t.Fatalf("message %q mismatch:\n%s", tc.Message, diff)
			}
		})
	}
}
```

Run:

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go test ./internal/intent -run TestDeterministicIntentCorpus -v
```

Expected: FAIL on currently unsupported semantic cases, including targeted listen, hide, throw, use-item, life-status question, and any extraction mismatch. Confirm that ordinary existing phrases pass before editing production code.

- [ ] **Step 4: Add the minimum fallback parsing needed by the corpus**

Modify `internal/intent/fallback.go` without changing the public API:

- Add branches before generic movement/search handling for `hide`, `throw`, and `use ... on ...`.
- Preserve the target for `listen at <target>`.
- Treat explicit plural object wording as `TargetAll` only when the player says `both` or `all`; keep `search the doctors` singular so runtime resolution can ask which doctor.
- Recognize `search the doctors are they dead` as a search action plus `FactLifeStatus` question.
- Extend target cleanup to remove connective phrases such as `through the` after `rummage`.
- Keep typo normalization in `normalizePlayerText`; add only corpus-observed corrections.

Use focused helpers with these signatures:

```go
func extractListenTarget(message string) string
func extractHideTarget(message string) string
func extractThrowParts(message string) (item string, target string)
func extractUseItemParts(message string) (item string, target string)
func isLifeStatusSearch(message string) bool
```

For the life-status case, return:

```go
TurnPlan{
	Actions: []PlannedAction{{
		Intent: Intent{Action: ActionSearch, Target: "doctors", Confidence: 0.8,
			RawText: message, Modifiers: []string{}},
		TargetMode: TargetAll,
	}},
	Questions: []FactQuestion{{Kind: game.FactLifeStatus, Target: "doctors", TargetMode: TargetAll}},
	Confidence: 0.8,
	RawText: message,
}
```

Add `kaya/internal/game` to the imports in `fallback.go` for `game.FactLifeStatus`.

- [ ] **Step 5: Verify GREEN and protect existing parser behavior**

Run:

```powershell
go test ./internal/intent -run TestDeterministicIntentCorpus -v
go test ./internal/intent
```

Expected: all corpus cases pass, followed by all existing intent tests passing.

- [ ] **Step 6: Commit the deterministic contract**

```powershell
git add internal/intent/intent_corpus_test.go internal/intent/fallback.go
git commit -m "test: add deterministic intent corpus"
```

---

### Task 2: Opt-in Ollama corpus evaluation

**Files:**
- Modify: `internal/intent/ollama_integration_test.go`
- Test: `internal/intent/ollama_integration_test.go`

**Interfaces:**
- Consumes: `intentCorpus`, `semanticPlanFrom`, `compareSemanticPlans`, and `validateSemanticPlan` from Task 1; existing `llm.NewOllamaClient` and `intent.NewParser`.
- Produces: `TestOllamaIntentCorpus` and a complete exact-match accuracy report.

- [ ] **Step 1: Write evaluator-helper tests and verify RED**

In `intent_corpus_test.go`, add tests for a package-local `corpusEvaluation`:

```go
func TestCorpusEvaluationCountsMatchesAndErrors(t *testing.T) {
	eval := corpusEvaluation{Total: 3}
	eval.RecordMatch()
	eval.RecordMismatch("wrong action")
	eval.RecordError("timeout")

	if eval.Matches != 1 || len(eval.Mismatches) != 1 || len(eval.Errors) != 1 {
		t.Fatalf("evaluation = %#v", eval)
	}
	if got := eval.Accuracy(); got != 100.0/3.0 {
		t.Fatalf("accuracy = %f, want %f", got, 100.0/3.0)
	}
}

func TestCorpusEvaluationFailsBelowThreshold(t *testing.T) {
	eval := corpusEvaluation{Total: 10, Matches: 8}
	if !eval.Fails(90) {
		t.Fatal("80 percent evaluation should fail a 90 percent threshold")
	}
	eval.Matches = 9
	if eval.Fails(90) {
		t.Fatal("90 percent evaluation should pass a 90 percent threshold")
	}
}
```

Run `go test ./internal/intent -run TestCorpusEvaluation -v`.

Expected: build FAIL because `corpusEvaluation` is undefined.

- [ ] **Step 2: Implement the evaluation accumulator**

Add to `intent_corpus_test.go`:

```go
type corpusEvaluation struct {
	Total      int
	Matches    int
	Mismatches []string
	Errors     []string
}

func (e *corpusEvaluation) RecordMatch() { e.Matches++ }
func (e *corpusEvaluation) RecordMismatch(message string) {
	e.Mismatches = append(e.Mismatches, message)
}
func (e *corpusEvaluation) RecordError(message string) {
	e.Errors = append(e.Errors, message)
}
func (e corpusEvaluation) Accuracy() float64 {
	if e.Total == 0 { return 0 }
	return 100 * float64(e.Matches) / float64(e.Total)
}
func (e corpusEvaluation) Fails(threshold float64) bool {
	return len(e.Errors) > 0 || e.Accuracy() < threshold
}
```

Run `go test ./internal/intent -run TestCorpusEvaluation -v`.

Expected: PASS.

- [ ] **Step 3: Add the opt-in live evaluator**

Add `TestOllamaIntentCorpus` to `internal/intent/ollama_integration_test.go`. Enable it when `KAYA_OLLAMA_EVAL` is one of `1`, `true`, `yes`, or `on`, case-insensitively. Reuse the existing model and URL environment variables.

```go
func TestOllamaIntentCorpus(t *testing.T) {
	if !truthyEnv("KAYA_OLLAMA_EVAL") {
		t.Skip("set KAYA_OLLAMA_EVAL=1 to run the Ollama intent corpus")
	}
	model := envOrDefault("KAYA_OLLAMA_MODEL", "qwen3.5:4b")
	baseURL := envOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)
	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil { t.Fatal(err) }
	parser := intent.NewParser(client)
	eval := corpusEvaluation{Total: len(intentCorpus)}

	for _, tc := range intentCorpus {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		plan, parseErr := parser.Parse(ctx, tc.Message, game.PerceptionSnapshot{})
		cancel()
		if parseErr != nil {
			eval.RecordError(fmt.Sprintf("%s (%q): %v", tc.Name, tc.Message, parseErr))
			continue
		}
		got := semanticPlanFrom(plan)
		if validErr := validateSemanticPlan(got); validErr != nil {
			eval.RecordError(fmt.Sprintf("%s (%q): invalid plan: %v", tc.Name, tc.Message, validErr))
			continue
		}
		if diff := compareSemanticPlans(tc.Want, got); diff != "" {
			eval.RecordMismatch(fmt.Sprintf("%s (%q):\n%s", tc.Name, tc.Message, diff))
			continue
		}
		eval.RecordMatch()
	}

	for _, mismatch := range eval.Mismatches { t.Logf("MISMATCH: %s", mismatch) }
	for _, parseError := range eval.Errors { t.Logf("ERROR: %s", parseError) }
	t.Logf("intent corpus: %d/%d exact matches, %d mismatches, %d errors, %.1f%% accuracy",
		eval.Matches, eval.Total, len(eval.Mismatches), len(eval.Errors), eval.Accuracy())
	if eval.Fails(90) {
		t.Fatalf("Ollama intent corpus failed: accuracy %.1f%%, threshold 90.0%%, errors %d",
			eval.Accuracy(), len(eval.Errors))
	}
}
```

Add `fmt` to the integration-test imports and:

```go
func truthyEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Verify default skip behavior**

Run:

```powershell
Remove-Item Env:KAYA_OLLAMA_EVAL -ErrorAction SilentlyContinue
go test ./internal/intent -run TestOllamaIntentCorpus -v
```

Expected: PASS with `SKIP` and the enablement message; no Ollama request is made.

- [ ] **Step 5: Run the live evaluation**

Run:

```powershell
$env:KAYA_OLLAMA_EVAL = "1"
go test ./internal/intent -run TestOllamaIntentCorpus -v -count=1
```

Expected: all 52 cases execute, every mismatch is logged, the summary reports exact-match accuracy, and the test passes at 90 percent or higher with zero parser errors. If it fails, preserve the report and fix only deterministic normalization errors demonstrated by corpus cases; do not weaken expected plans to match incorrect model output.

- [ ] **Step 6: Commit the Ollama evaluator**

```powershell
git add internal/intent/intent_corpus_test.go internal/intent/ollama_integration_test.go
git commit -m "test: evaluate intent corpus with Ollama"
```

---

### Task 3: Full verification and usage documentation

**Files:**
- Modify: `docs/intent-parser-prompts.md`
- Test: all Go packages.

**Interfaces:**
- Consumes: the deterministic and Ollama corpus tests from Tasks 1 and 2.
- Produces: documented commands for future parser regression work.

- [ ] **Step 1: Document the two corpus modes**

Append a section named `Intent Corpus` to `docs/intent-parser-prompts.md` containing:

````markdown
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

The live test reports every mismatch and requires at least 90 percent exact-match
accuracy with zero parser errors. Add each parser regression to `intentCorpus`
with an explicit expected semantic plan.
````

- [ ] **Step 2: Run formatting and focused tests**

```powershell
gofmt -w internal/intent/intent_corpus_test.go internal/intent/ollama_integration_test.go internal/intent/fallback.go
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go test ./internal/intent
```

Expected: PASS.

- [ ] **Step 3: Run full repository verification**

```powershell
go test ./...
go vet ./...
git diff --check
```

Expected: every package passes, `go vet` reports no findings, and `git diff --check` reports no whitespace errors. The live Ollama suite remains skipped unless explicitly enabled.

- [ ] **Step 4: Commit documentation and final verification state**

```powershell
git add docs/intent-parser-prompts.md
git commit -m "docs: explain intent corpus evaluation"
```
