# Context-Aware Semantic Turns Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add realistic darkness knowledge, contextual multi-action parsing, sequential multi-target execution, typed engine facts, and fact-locked natural Kaya responses.

**Architecture:** `world.State` remains the source of perception truth. An LLM creates a bounded `intent.TurnPlan` from a safe snapshot; `turn.Executor` resolves and executes it through the existing single-action resolver; `response.Composer` renders only the resulting approved fact bundle and falls back deterministically on any model or validation failure.

**Tech Stack:** Go 1.x standard library, Ollama `/api/generate`, local `qwen3.5:4b`, existing deterministic Kaya engine, table-driven Go tests.

## Global Constraints

- The LLM never receives `world.State`, hidden objects, unknown exits, undiscovered items, placements, future events, or locked-door internals.
- The LLM never resolves engine IDs, mutates state, advances time, applies autonomy, or decides outcomes.
- At most four actions and four questions are accepted per player message.
- Plural targets execute in authored room order; each target receives separate time, events, and autonomy handling.
- Completed target actions are never rolled back when a later target fails, refuses, or needs clarification.
- Every valid tactile exploration attempt costs exactly 30 seconds.
- Recent referent memory retains at most three perception-safe groups.
- Response drafts must cite approved fact IDs; invalid drafts use deterministic fallback.
- All shell commands use `rtk`.
- No new third-party Go dependency is required.

## File Structure

- Create `internal/game/perception.go`: neutral snapshot, referent, fact, and action-status types.
- Modify `internal/game/result.go`: typed facts and explicit result status.
- Create `internal/world/exits.go`: known-exit knowledge and tactile discovery.
- Create `internal/world/perception.go`: safe snapshot construction and referent memory.
- Create `internal/world/observation.go`: typed observable object facts.
- Modify `internal/world/state.go`, `object.go`: state fields and object observation data.
- Modify `internal/actions/resolver.go`: known-exit enforcement, explore action, observation recording, typed result status.
- Create `internal/intent/turn_plan.go`, `schema.go`, `fallback.go`: semantic plan contract, JSON schema, deterministic fallback.
- Modify `internal/intent/parser.go`, `prompt.go`, `intent.go`: contextual plan parsing and `explore`.
- Modify `internal/llm/ollama.go`: reusable JSON-schema generation.
- Create `internal/turn/types.go`, `executor.go`, `facts.go`: sequential execution and fact bundles.
- Create `internal/response/types.go`, `fallback.go`, `composer.go`, `prompt.go`, `validator.go`: safe response generation.
- Modify `internal/scenario/prototype.go`: initialize perception and author doctor life-status facts.
- Modify `cmd/kaya/main.go`: orchestrate plan, execution, and response composition.
- Update matching `_test.go` files and `docs/engine-milestones.md`.

---

### Task 1: Known exits and tactile exploration

**Files:**
- Create: `internal/world/exits.go`
- Modify: `internal/world/state.go`
- Modify: `internal/intent/intent.go`
- Modify: `internal/actions/resolver.go`
- Modify: `internal/scenario/prototype.go`
- Test: `internal/world/state_test.go`
- Test: `internal/actions/resolver_test.go`

**Interfaces:**
- Produces: `State.ObserveRoom(roomID game.RoomID, enteredFrom game.RoomID) error`
- Produces: `State.KnownExits() ([]world.Exit, error)` through the existing `AvailableExits` name
- Produces: `State.DiscoverNextUnknownExit() (world.Exit, bool, error)`
- Produces: `intent.ActionExplore`
- Produces outcomes: `exit_discovered`, `no_unknown_exits`, `exit_unknown`

- [ ] **Step 1: Write failing darkness and exploration tests**

```go
func TestDarkRoomKnowsOnlyRouteBack(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := actions.NewResolver(state)
	if got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"}); got.Outcome != "moved" {
		t.Fatalf("move outcome = %q", got.Outcome)
	}
	exits, err := state.AvailableExits()
	if err != nil {
		t.Fatal(err)
	}
	if len(exits) != 1 || exits[0].Direction != "west" {
		t.Fatalf("dark exits = %#v, want west only", exits)
	}
	got := resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "north"})
	if got.Outcome != "exit_unknown" || state.CurrentRoomID != scenario.RoomStorage {
		t.Fatalf("unknown move = %q room=%q", got.Outcome, state.CurrentRoomID)
	}
}

func TestExploreDiscoversAuthoredExitAndCostsThirtySeconds(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := actions.NewResolver(state)
	resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})
	before := state.NowSeconds
	got := resolver.Resolve(intent.Intent{Action: intent.ActionExplore, Target: "walls"})
	if got.Outcome != "exit_discovered" || got.DurationSeconds != 30 {
		t.Fatalf("explore = %#v", got)
	}
	if state.NowSeconds-before != 30 {
		t.Fatalf("elapsed = %d, want 30", state.NowSeconds-before)
	}
	exits, _ := state.AvailableExits()
	if len(exits) != 2 || exits[1].Direction != "north" {
		t.Fatalf("known exits = %#v", exits)
	}
}

func TestTurningOnLightRevealsAllCurrentRoomExits(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.AddInventory(scenario.ItemFlashlight)
	resolver := actions.NewResolver(state)
	resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})
	resolver.Resolve(intent.Intent{Action: intent.ActionTurnOn, Item: "flashlight"})
	exits, _ := state.AvailableExits()
	if len(exits) != 2 { t.Fatalf("lit exits = %#v, want west and north", exits) }
}

func TestExploringKnownRoomStillCostsThirtySecondsAndFiresEvents(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.ScheduleEvent(10, game.WorldEvent{Type: game.EventSound, Description: "pipe knocks"})
	got := actions.NewResolver(state).Resolve(intent.Intent{Action: intent.ActionExplore})
	if got.Outcome != "no_unknown_exits" || got.DurationSeconds != 30 || len(got.Events) != 1 {
		t.Fatalf("explore = %#v", got)
	}
}
```

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `rtk proxy go test ./internal/world ./internal/actions -run 'Test(DarkRoomKnowsOnlyRouteBack|ExploreDiscoversAuthoredExitAndCostsThirtySeconds)' -count=1`

Expected: FAIL because all exits are currently returned and `ActionExplore` is undefined.

- [ ] **Step 3: Implement known-exit state in authored order**

Add to `world.State` and initialize in `NewState`:

```go
KnownExitDirections map[game.RoomID]map[string]bool
```

Create `internal/world/exits.go`:

```go
package world

import (
	"fmt"

	"kaya/internal/game"
)

func (s *State) ObserveRoom(roomID, enteredFrom game.RoomID) error {
	room, ok := s.Rooms[roomID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrRoomNotFound, roomID)
	}
	if s.KnownExitDirections == nil {
		s.KnownExitDirections = make(map[game.RoomID]map[string]bool)
	}
	if s.KnownExitDirections[roomID] == nil {
		s.KnownExitDirections[roomID] = make(map[string]bool)
	}
	for _, exit := range room.Exits {
		if s.ActiveLight || !room.NeedsLight() || exit.To == enteredFrom {
			s.KnownExitDirections[roomID][exit.Direction] = true
		}
	}
	return nil
}

func (s *State) AvailableExits() ([]Exit, error) {
	room, err := s.CurrentRoom()
	if err != nil {
		return nil, err
	}
	known := s.KnownExitDirections[room.ID]
	exits := make([]Exit, 0, len(room.Exits))
	for _, exit := range room.Exits {
		if known[exit.Direction] {
			exits = append(exits, exit)
		}
	}
	return exits, nil
}

func (s *State) DiscoverNextUnknownExit() (Exit, bool, error) {
	room, err := s.CurrentRoom()
	if err != nil {
		return Exit{}, false, err
	}
	for _, exit := range room.Exits {
		if !s.KnownExitDirections[room.ID][exit.Direction] {
			if s.KnownExitDirections[room.ID] == nil {
				s.KnownExitDirections[room.ID] = make(map[string]bool)
			}
			s.KnownExitDirections[room.ID][exit.Direction] = true
			return exit, true, nil
		}
	}
	return Exit{}, false, nil
}
```

Remove the old `AvailableExits` body from `state.go`. Call `state.ObserveRoom(RoomReception, "")` at the end of `NewPrototypeTemplate`.

- [ ] **Step 4: Enforce exit knowledge and add exploration action**

Add `ActionExplore Action = "explore"` and include it in `Action.Valid`. In the resolver switch add:

```go
case intent.ActionExplore:
	result = r.explore()
```

Change `move` to iterate over `r.state.AvailableExits()`. If the direction matches an authored room exit but not a known exit, return:

```go
return failed("exit_unknown", "I cannot safely find that route in the dark.")
```

After changing rooms in `moveThroughExit`, call:

```go
if err := r.state.ObserveRoom(exit.To, from); err != nil {
	return failed("move_failed", err.Error())
}
```

After setting `ActiveLight = true` in `turnOn` and flashlight `useItem`, call `ObserveRoom(r.state.CurrentRoomID, "")`. Add:

```go
func (r Resolver) explore() game.ActionResult {
	exit, found, err := r.state.DiscoverNextUnknownExit()
	if err != nil {
		return failed("explore_failed", err.Error())
	}
	if !found {
		return makeResult("no_unknown_exits", 30, "I feel along the walls but cannot find another exit.")
	}
	return makeResult("exit_discovered", 30, "I feel along the wall and find a way "+exit.Direction+".")
}
```

Change `ResolveDoor`, key-door inference, and any player-facing exit listing to use `AvailableExits`; a guessed door behind an unknown exit must remain unresolved. Add a leave-and-return test proving discovered exit knowledge survives room changes.

- [ ] **Step 5: Run package and generator replay tests**

Run: `rtk proxy go test ./internal/world ./internal/actions ./internal/scenario ./internal/rungen -count=1`

Expected: PASS, including Phase 5 witness replay.

- [ ] **Step 6: Commit**

```text
rtk git add internal/world internal/intent/intent.go internal/actions/resolver.go internal/scenario/prototype.go
rtk git commit -m "feat: track known exits in darkness"
```

---

### Task 2: Perception snapshots, referents, and observable facts

**Files:**
- Create: `internal/game/perception.go`
- Create: `internal/world/perception.go`
- Create: `internal/world/observation.go`
- Modify: `internal/game/result.go`
- Modify: `internal/world/state.go`
- Modify: `internal/world/object.go`
- Modify: `internal/actions/resolver.go`
- Modify: `internal/scenario/prototype.go`
- Test: `internal/world/perception_test.go`
- Test: `internal/world/observation_test.go`

**Interfaces:**
- Produces: `State.PerceptionSnapshot() (game.PerceptionSnapshot, error)`
- Produces: `State.RememberObjects([]game.ObjectID)` and `State.RememberItems([]game.ItemID)`
- Produces: `State.ResolveObjectGroup(target string, all bool) (ObjectResolution, error)`
- Produces: `State.ObserveObject(objectID game.ObjectID, method ObservationMethod) []game.Fact`
- Produces: typed `game.Fact`, `game.FactKind`, `game.FactID`, and `game.ActionStatus`

- [ ] **Step 1: Write failing snapshot and referent tests**

```go
func TestPerceptionSnapshotHidesDarkWorldData(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	resolver := actions.NewResolver(state)
	resolver.Resolve(intent.Intent{Action: intent.ActionMove, Direction: "east"})
	snapshot, err := state.PerceptionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.VisibleObjects) != 0 {
		t.Fatalf("visible objects = %#v", snapshot.VisibleObjects)
	}
	if len(snapshot.KnownExits) != 1 || snapshot.KnownExits[0].Direction != "west" {
		t.Fatalf("known exits = %#v", snapshot.KnownExits)
	}
	encoded, _ := json.Marshal(snapshot)
	if bytes.Contains(encoded, []byte("brass_key")) || bytes.Contains(encoded, []byte("north")) {
		t.Fatalf("snapshot leaked hidden state: %s", encoded)
	}
}

func TestPluralPronounUsesLatestPerceivedGroup(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil { t.Fatal(err) }
	state.RememberObjects([]game.ObjectID{scenario.ObjectBodyCabinet, scenario.ObjectBodyDoor})
	got, err := state.ResolveObjectGroup("them", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 2 || got.Matches[0].ID != scenario.ObjectBodyCabinet || got.Matches[1].ID != scenario.ObjectBodyDoor {
		t.Fatalf("matches = %#v", got.Matches)
	}
}
```

- [ ] **Step 2: Run tests and confirm failure**

Run: `rtk proxy go test ./internal/world -run 'Test(PerceptionSnapshotHidesDarkWorldData|PluralPronounUsesLatestPerceivedGroup)' -count=1`

Expected: FAIL because snapshot and referent APIs do not exist.

- [ ] **Step 3: Add neutral perception and fact contracts**

Create `internal/game/perception.go`:

```go
package game

type FactID string
type FactKind string

const (
	FactAction          FactKind = "action"
	FactRoomDescription FactKind = "room_description"
	FactVisibleObjects  FactKind = "visible_objects"
	FactKnownExits      FactKind = "known_exits"
	FactItemDiscovery   FactKind = "item_discovery"
	FactLifeStatus      FactKind = "life_status"
	FactElapsedTime     FactKind = "elapsed_time"
	FactEvent           FactKind = "event"
	FactFailure         FactKind = "failure"
	FactClarification   FactKind = "clarification"
	FactEmotion         FactKind = "emotion"
)

type ActionStatus string

const (
	ActionSucceeded     ActionStatus = "succeeded"
	ActionFailed        ActionStatus = "failed"
	ActionRefused       ActionStatus = "refused"
	ActionClarification ActionStatus = "clarification"
)

type PerceivedObject struct { ID ObjectID `json:"id"`; Name string `json:"name"`; Aliases []string `json:"aliases"` }
type PerceivedExit struct { Direction string `json:"direction"` }
type PerceivedItem struct { ID ItemID `json:"id"`; Name string `json:"name"`; Aliases []string `json:"aliases"` }
type ReferentGroup struct { ObjectIDs []ObjectID `json:"objectIds,omitempty"`; ItemIDs []ItemID `json:"itemIds,omitempty"` }
type PerceptionSnapshot struct {
	RoomName string `json:"roomName"`
	HasUsefulLight bool `json:"hasUsefulLight"`
	VisibleObjects []PerceivedObject `json:"visibleObjects"`
	KnownExits []PerceivedExit `json:"knownExits"`
	Inventory []PerceivedItem `json:"inventory"`
	RecentReferents []ReferentGroup `json:"recentReferents"`
}
```

Expand `game.Fact` and `game.ActionResult` without removing existing fields:

```go
type Fact struct {
	ID FactID `json:"id"`
	Kind FactKind `json:"kind"`
	Subject string `json:"subject"`
	Value string `json:"value"`
	Text string `json:"text"`
	Required bool `json:"required"`
}

type ActionResult struct {
	Status ActionStatus
	TargetObjectIDs []ObjectID
	StartedAtSeconds int
	DurationSeconds int
	Outcome string
	VisibleFacts []Fact
	Events []WorldEvent
	StressDelta int
	TrustDelta int
	FearDelta int
	PainDelta int
	ExhaustionDelta int
	Danger DangerLevel
	NeedsClarification bool
	ClarificationQuestion string
}
```

Set `makeResult` to `ActionSucceeded`, `failed` to `ActionFailed`, clarification helpers to `ActionClarification`, and autonomy refusal to `ActionRefused`. `makeResult` creates `FactAction` facts with `Value`, `Text`, and `Required:true`; `failed` uses `FactFailure`; clarification uses `FactClarification`. Convert room description, visible-object, known-exit, and autonomy facts to their typed kinds with `Required:true`; no existing player-visible resolver fact may remain with an empty kind or `Required:false`.

- [ ] **Step 4: Implement safe snapshots and bounded referents**

Add `RecentReferents []game.ReferentGroup` to `State`. In `internal/world/perception.go`, build visible objects in `room.Objects` order, known exits in `room.Exits` order, inventory sorted by item ID, and copy only the last three valid referent groups.

Implement these entrypoints:

```go
func (s *State) PerceptionSnapshot() (game.PerceptionSnapshot, error)
func (s *State) RememberObjects(ids []game.ObjectID)
func (s *State) RememberItems(ids []game.ItemID)
func (s *State) ResolveObjectGroup(target string, all bool) (ObjectResolution, error)
```

`ResolveObjectGroup` resolves `it/that` from the newest singular object group, resolves `they/them/those/both` from the newest plural group, and otherwise matches visible objects. When `all` is false, multiple matches remain ambiguous. When `all` is true, return all matches in room order. For plural nouns, retry matching after removing a trailing `s` from the last target word.

- [ ] **Step 5: Add authored observable facts**

Add to `world.Object`:

```go
ObservableFacts []ObservableFact
```

Create `internal/world/observation.go`:

```go
package world

import (
	"kaya/internal/game"
)

type ObservationMethod string

const (
	ObservationInspect ObservationMethod = "inspect"
	ObservationSearch ObservationMethod = "search"
)

type ObservableFact struct {
	Kind game.FactKind
	Value string
	Text string
	RevealOn []ObservationMethod
}

func (s *State) ObserveObject(objectID game.ObjectID, method ObservationMethod) []game.Fact
func (s *State) ObservedFact(objectID game.ObjectID, kind game.FactKind) (game.Fact, bool)
```

Store observations in `State.ObservedObjectFacts map[game.ObjectID]map[game.FactKind]game.Fact`. Author both doctors with `FactLifeStatus`, value `dead`, revealable by inspect or search. When inspect/search resolves an object, call `ObserveObject`, append returned facts, set `TargetObjectIDs`, and remember the object. Room inspection remembers the complete visible object group.

- [ ] **Step 6: Run world, action, and scenario tests**

Run: `rtk proxy go test ./internal/game ./internal/world ./internal/actions ./internal/scenario -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```text
rtk git add internal/game internal/world internal/actions/resolver.go internal/scenario/prototype.go
rtk git commit -m "feat: expose perception-safe world context"
```

---

### Task 3: JSON-schema Ollama generation

**Files:**
- Modify: `internal/llm/ollama.go`
- Modify: `internal/llm/ollama_test.go`

**Interfaces:**
- Produces: `GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, schema any) (string, error)`
- Preserves: `Generate(ctx, systemPrompt, userPrompt string) (string, error)`

- [ ] **Step 1: Write a failing schema forwarding test**

```go
func TestOllamaClientGenerateJSONForwardsSchema(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []string{"actions"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil { t.Fatal(err) }
		format, ok := request.Format.(map[string]any)
		if !ok || format["type"] != "object" { t.Fatalf("format = %#v", request.Format) }
		if request.Think == nil || *request.Think { t.Fatalf("think = %#v", request.Think) }
		_, _ = w.Write([]byte(`{"response":"{\"actions\":[]}"}`))
	}))
	defer server.Close()
	client, _ := NewOllamaClient("qwen3.5:4b", WithOllamaBaseURL(server.URL))
	if _, err := client.GenerateJSON(context.Background(), "system", "user", schema); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run the test and confirm failure**

Run: `rtk proxy go test ./internal/llm -run TestOllamaClientGenerateJSONForwardsSchema -count=1`

Expected: FAIL because `GenerateJSON` does not exist and `Format` is a string.

- [ ] **Step 3: Generalize Ollama structured output**

Change `ollamaGenerateRequest.Format` to `any`. Implement:

```go
func (c *OllamaClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return c.generate(ctx, systemPrompt, userPrompt, "json")
}

func (c *OllamaClient) GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, schema any) (string, error) {
	if schema == nil {
		return "", errors.New("json schema is required")
	}
	return c.generate(ctx, systemPrompt, userPrompt, schema)
}

func (c *OllamaClient) generate(ctx context.Context, systemPrompt, userPrompt string, format any) (string, error) {
	think := false
	body := ollamaGenerateRequest{
		Model: c.model, System: systemPrompt, Prompt: userPrompt,
		Stream: false, Format: format, Think: &think,
		Options: map[string]any{"temperature": 0},
	}
	encoded, err := json.Marshal(body)
	if err != nil { return "", fmt.Errorf("encode ollama request: %w", err) }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(encoded))
	if err != nil { return "", fmt.Errorf("create ollama request: %w", err) }
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil { return "", fmt.Errorf("call ollama: %w", err) }
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil { return "", fmt.Errorf("read ollama response: %w", err) }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama returned %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	var generated ollamaGenerateResponse
	if err := json.Unmarshal(responseBody, &generated); err != nil { return "", fmt.Errorf("decode ollama response: %w", err) }
	if generated.Response == "" { return "", errors.New("ollama returned empty response") }
	return generated.Response, nil
}
```

- [ ] **Step 4: Run tests and commit**

Run: `rtk proxy go test ./internal/llm -count=1`

Expected: PASS.

```text
rtk git add internal/llm
rtk git commit -m "feat: support ollama json schemas"
```

---

### Task 4: Contextual `TurnPlan` parser with deterministic fallback

**Files:**
- Create: `internal/intent/turn_plan.go`
- Create: `internal/intent/schema.go`
- Create: `internal/intent/fallback.go`
- Modify: `internal/intent/parser.go`
- Modify: `internal/intent/prompt.go`
- Modify: `internal/intent/parser_test.go`
- Modify: `internal/intent/ollama_integration_test.go`

**Interfaces:**
- Consumes: `game.PerceptionSnapshot`
- Consumes: generator method `GenerateJSON(context.Context, string, string, any) (string, error)`
- Produces: `Parser.Parse(ctx, message, snapshot) (TurnPlan, error)`
- Produces: `ParseTurnPlanJSON(raw string) (TurnPlan, error)`
- Produces: `FallbackPlan(message string) TurnPlan`

- [ ] **Step 1: Write failing multi-action, limit, and fallback tests**

```go
func TestParserParsesPluralCompoundTurn(t *testing.T) {
	generator := &fakeGenerator{responses: []string{`{
		"actions":[{"intent":{"action":"search","target":"doctors","item":"","direction":"","modifiers":[],"confidence":0.96,"rawText":"search the doctors","needsClarification":false,"clarificationQuestion":""},"targetMode":"all"}],
		"questions":[{"kind":"life_status","target":"they","targetMode":"all"}],
		"confidence":0.96,"needsClarification":false,"clarificationQuestion":"","rawText":"search the doctors are they dead"
	}`}}
	parser := NewParser(generator)
	plan, err := parser.Parse(context.Background(), "search the doctors are they dead", game.PerceptionSnapshot{})
	if err != nil { t.Fatal(err) }
	if len(plan.Actions) != 1 || plan.Actions[0].TargetMode != TargetAll || len(plan.Questions) != 1 { t.Fatalf("plan = %#v", plan) }
}

func TestFallbackPlanExploresWalls(t *testing.T) {
	plan := FallbackPlan("feel along the walls for another exit")
	if len(plan.Actions) != 1 || plan.Actions[0].Intent.Action != ActionExplore { t.Fatalf("plan = %#v", plan) }
}

func TestParseTurnPlanRejectsMoreThanFourActions(t *testing.T) {
	_, err := ParseTurnPlanJSON(fiveActionPlanJSON)
	if !errors.Is(err, ErrPlanTooLarge) { t.Fatalf("error = %v", err) }
}

const fiveActionPlanJSON = `{
	"actions":[
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"},
		{"intent":{"action":"wait","target":"","item":"","direction":"","modifiers":[],"confidence":1,"rawText":"wait","needsClarification":false,"clarificationQuestion":""},"targetMode":"single"}
	],"questions":[],"confidence":1,"needsClarification":false,"clarificationQuestion":"","rawText":"wait five times"
}`
```

- [ ] **Step 2: Run focused parser tests and confirm failure**

Run: `rtk proxy go test ./internal/intent -run 'Test(ParserParsesPluralCompoundTurn|FallbackPlanExploresWalls|ParseTurnPlanRejectsMoreThanFourActions)' -count=1`

Expected: FAIL because `TurnPlan`, contextual parsing, and fallback do not exist.

- [ ] **Step 3: Define and validate the semantic plan**

Create `turn_plan.go` with:

```go
type TargetMode string
const ( TargetSingle TargetMode = "single"; TargetAll TargetMode = "all" )

type PlannedAction struct { Intent Intent `json:"intent"`; TargetMode TargetMode `json:"targetMode"` }
type FactQuestion struct { Kind game.FactKind `json:"kind"`; Target string `json:"target"`; TargetMode TargetMode `json:"targetMode"` }
type TurnPlan struct {
	Actions []PlannedAction `json:"actions"`
	Questions []FactQuestion `json:"questions"`
	Confidence float64 `json:"confidence"`
	NeedsClarification bool `json:"needsClarification"`
	ClarificationQuestion string `json:"clarificationQuestion"`
	RawText string `json:"rawText"`
}
```

`ParseTurnPlanJSON` must trim code fences, reject unknown fields with `json.Decoder.DisallowUnknownFields`, require EOF after one object, validate every action and target mode, allow question kinds `life_status` only in this slice, enforce confidence `0..1`, and return `ErrPlanTooLarge` above four actions or questions. Normalize every embedded `Intent` through the existing normalization function.

After validation, a plan below `0.40` confidence becomes a clarification plan with no executable actions and `What do you want Kaya to do?` when the model did not supply a question.

Create `schema.go` with the complete schema:

```go
var actionNames = []any{"unknown", "move", "inspect", "search", "take_item", "use_item", "talk", "wait", "hide", "listen", "throw", "force_open", "turn_on", "turn_off", "explore"}

var embeddedIntentSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"action", "target", "item", "direction", "modifiers", "confidence", "rawText", "needsClarification", "clarificationQuestion"},
	"properties": map[string]any{
		"action": map[string]any{"type": "string", "enum": actionNames},
		"target": map[string]any{"type": "string"},
		"item": map[string]any{"type": "string"},
		"direction": map[string]any{"type": "string"},
		"modifiers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"rawText": map[string]any{"type": "string"},
		"needsClarification": map[string]any{"type": "boolean"},
		"clarificationQuestion": map[string]any{"type": "string"},
	},
}

var TurnPlanSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"actions", "questions", "confidence", "needsClarification", "clarificationQuestion", "rawText"},
	"properties": map[string]any{
		"actions": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"intent", "targetMode"},
			"properties": map[string]any{
				"intent": embeddedIntentSchema,
				"targetMode": map[string]any{"type": "string", "enum": []any{"single", "all"}},
			},
		}},
		"questions": map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"kind", "target", "targetMode"},
			"properties": map[string]any{
				"kind": map[string]any{"type": "string", "enum": []any{"life_status"}},
				"target": map[string]any{"type": "string"},
				"targetMode": map[string]any{"type": "string", "enum": []any{"single", "all"}},
			},
		}},
		"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"needsClarification": map[string]any{"type": "boolean"},
		"clarificationQuestion": map[string]any{"type": "string"},
		"rawText": map[string]any{"type": "string"},
	},
}
```

- [ ] **Step 4: Implement contextual generation, one repair, and fallback**

Use this interface:

```go
type StructuredGenerator interface {
	GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, schema any) (string, error)
}
```

Change parser flow to:

```go
func (p Parser) Parse(ctx context.Context, message string, snapshot game.PerceptionSnapshot) (TurnPlan, error) {
	message = strings.TrimSpace(message)
	if message == "" { return TurnPlan{}, ErrEmptyMessage }
	payload, err := json.Marshal(struct { Player string `json:"player"`; Perception game.PerceptionSnapshot `json:"perception"` }{message, snapshot})
	if err != nil { return TurnPlan{}, fmt.Errorf("encode parser input: %w", err) }
	if p.generator == nil { return FallbackPlan(message), nil }
	raw, err := p.generator.GenerateJSON(ctx, SystemPrompt, string(payload), TurnPlanSchema)
	if err != nil { return FallbackPlan(message), nil }
	plan, parseErr := ParseTurnPlanJSON(raw)
	if parseErr == nil { return plan, nil }
	repaired, repairErr := p.generator.GenerateJSON(ctx, RepairPrompt, raw, TurnPlanSchema)
	if repairErr != nil { return FallbackPlan(message), nil }
	plan, parseErr = ParseTurnPlanJSON(repaired)
	if parseErr != nil { return FallbackPlan(message), nil }
	return plan, nil
}
```

`FallbackPlan` must recognize, in order: tactile exploration; turn on/off flashlight; cardinal/back movement; room inspection; explicit search; take/pick up; wait; listen. Anything else returns `ActionUnknown`, confidence `0`, and `What do you want Kaya to do?`.

Replace `SystemPrompt` with the `TurnPlan` schema semantics: ordered actions, separate fact questions, plural `TargetAll`, singular ambiguity left to the engine, recent referents from the supplied snapshot, and no world invention.

- [ ] **Step 5: Update old parser tests to plan assertions**

Wrap each former single intent JSON response inside one `TurnPlan`, call `Parse(ctx, message, game.PerceptionSnapshot{})`, and assert `plan.Actions[0].Intent`. Keep tests for direction normalization, flashlight restoration, key use, repair count, and vague clarification.

- [ ] **Step 6: Run parser tests and commit**

Run: `rtk proxy go test ./internal/intent -count=1`

Expected: PASS; gated Ollama tests skip unless explicitly enabled.

```text
rtk git add internal/intent
rtk git commit -m "feat: parse context-aware semantic turns"
```

---

### Task 5: Sequential turn execution and fact queries

**Files:**
- Create: `internal/turn/types.go`
- Create: `internal/turn/executor.go`
- Create: `internal/turn/facts.go`
- Create: `internal/turn/executor_test.go`
- Modify: `internal/actions/resolver.go`

**Interfaces:**
- Consumes: `intent.TurnPlan`, `world.State`, existing `actions.Resolver`
- Produces: `turn.NewExecutor(state *world.State) Executor`
- Produces: `Executor.Execute(plan intent.TurnPlan) Result`
- Produces: `Result.FactBundle(playerMessage string) FactBundle`

- [ ] **Step 1: Write failing plural execution and partial-completion tests**

```go
func TestExecutorSearchesBothDoctorsInOrder(t *testing.T) {
	state := newLitStorageState(t)
	executor := NewExecutor(state)
	start := state.NowSeconds
	result := executor.Execute(intent.TurnPlan{
		Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionSearch, Target: "doctors"}, TargetMode: intent.TargetAll}},
		Questions: []intent.FactQuestion{{Kind: game.FactLifeStatus, Target: "they", TargetMode: intent.TargetAll}},
	})
	if len(result.Outcomes) != 2 { t.Fatalf("outcomes = %#v", result.Outcomes) }
	if result.Outcomes[0].TargetObjectID != scenario.ObjectBodyCabinet || result.Outcomes[1].TargetObjectID != scenario.ObjectBodyDoor { t.Fatalf("order = %#v", result.Outcomes) }
	if state.NowSeconds-start != 65 { t.Fatalf("elapsed = %d, want 65", state.NowSeconds-start) }
	if len(result.QuestionFacts) != 2 || result.QuestionFacts[0].Value != "dead" || result.QuestionFacts[1].Value != "dead" { t.Fatalf("facts = %#v", result.QuestionFacts) }
}

func TestExecutorPreservesFirstTargetWhenSecondRefuses(t *testing.T) {
	state := newLitStorageState(t)
	resolver := &sequenceResolver{state: state, results: []game.ActionResult{
		{Status: game.ActionSucceeded, Outcome: "searched_empty", DurationSeconds: 30},
		{Status: game.ActionRefused, Outcome: "kaya_refused"},
	}}
	executor := newExecutor(state, resolver)
	result := executor.Execute(doctorSearchPlan())
	if len(result.Outcomes) != 2 || result.Outcomes[0].Result.Status != game.ActionSucceeded || result.Outcomes[1].Result.Status != game.ActionRefused { t.Fatalf("outcomes = %#v", result.Outcomes) }
	if state.NowSeconds != 30 { t.Fatalf("time = %d, want committed first action time", state.NowSeconds) }
}

func newLitStorageState(t *testing.T) *world.State {
	t.Helper()
	state := scenario.NewPrototypeWorld()
	state.CurrentRoomID = scenario.RoomStorage
	state.PreviousRoomID = scenario.RoomReception
	state.ActiveLight = true
	if err := state.ObserveRoom(scenario.RoomStorage, scenario.RoomReception); err != nil { t.Fatal(err) }
	return state
}

func doctorSearchPlan() intent.TurnPlan {
	return intent.TurnPlan{Actions: []intent.PlannedAction{{
		Intent: intent.Intent{Action: intent.ActionSearch, Target: "doctors"},
		TargetMode: intent.TargetAll,
	}}}
}

type sequenceResolver struct { state *world.State; results []game.ActionResult }
func (r *sequenceResolver) Resolve(intent.Intent) game.ActionResult {
	result := r.results[0]
	r.results = r.results[1:]
	if result.DurationSeconds > 0 { r.state.Advance(result.DurationSeconds) }
	return result
}
```

Use an internal `actionResolver` interface with only `Resolve(intent.Intent) game.ActionResult`. `NewExecutor` wraps `actions.NewResolver(state)`; the unexported `newExecutor` accepts the test resolver. This tests partial commits without bypassing production autonomy.

- [ ] **Step 2: Run focused tests and confirm failure**

Run: `rtk proxy go test ./internal/turn -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Define turn results and fact bundles**

```go
type ActionOutcome struct {
	Intent intent.Intent
	TargetObjectID game.ObjectID
	Result game.ActionResult
}

type Result struct {
	Outcomes []ActionOutcome
	QuestionFacts []game.Fact
	StopReason string
	ClarificationQuestion string
	Emotion kaya.Emotion
}

type FactBundle struct {
	PlayerMessage string
	Outcomes []ActionOutcome
	Facts []game.Fact
	Emotion kaya.Emotion
}
```

`FactBundle` copies action facts and question facts, converts each elapsed duration and fired event to typed facts, assigns stable IDs `f001`, `f002`, and marks mutation, failure, refusal, clarification, time, and event facts required.

At the end of `Executor.Execute`, set `Result.Emotion = state.Kaya.DominantEmotion()`. `Result.FactBundle` copies that value into `FactBundle.Emotion`.

- [ ] **Step 4: Implement deterministic target expansion and stopping**

`Executor.Execute` must:

1. Validate all plan counts, action enums, target modes, and question kinds before mutation; invalid structure returns clarification with zero outcomes.
2. Return parser clarification without executing actions.
3. For `inspect` and `search`, call `ResolveObjectGroup(target, TargetMode == TargetAll)`.
4. Convert ambiguity and missing targets to one failed outcome without calling the resolver.
5. Clone the planned intent per resolved object and replace `Target` with the exact object name.
6. Call the existing resolver once per expanded target.
7. Append each outcome immediately.
8. Stop when `ActionResult.Status != game.ActionSucceeded`.
9. Remember the full resolved group after a plural action, even though individual resolver calls remembered singular objects.
10. Continue to fact questions after stopping physical execution.

For non-object actions, reject `TargetAll` with clarification and otherwise call the resolver once.

- [ ] **Step 5: Implement perception-gated questions**

For each `FactQuestion`, resolve its target group through recent referents. For every object, call `ObservedFact(object.ID, question.Kind)`. If present, append it with subject set to the object's display name. If absent, append:

```go
game.Fact{
	Kind: game.FactFailure,
	Subject: object.Name,
	Value: "unknown",
	Text: "I cannot tell whether "+object.Name+" is dead yet.",
	Required: true,
}
```

Never read the object's unauthorised `ObservableFacts` directly from the question layer.

- [ ] **Step 6: Run turn and regression tests**

Run: `rtk proxy go test ./internal/turn ./internal/actions ./internal/rungen -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```text
rtk git add internal/turn internal/actions/resolver.go
rtk git commit -m "feat: execute multi-target semantic turns"
```

---

### Task 6: Deterministic Kaya response renderer

**Files:**
- Create: `internal/response/types.go`
- Create: `internal/response/fallback.go`
- Create: `internal/response/fallback_test.go`

**Interfaces:**
- Consumes: `turn.FactBundle`
- Produces: `Fallback.Render(bundle turn.FactBundle) string`
- Produces: `Response{Text string, UsedFallback bool, FallbackReason string}`

- [ ] **Step 1: Write failing fallback ordering and completeness tests**

```go
func TestFallbackIncludesRequiredFactsOnceInOrder(t *testing.T) {
	bundle := turn.FactBundle{Facts: []game.Fact{
		{ID: "f001", Text: "I search Doctor Near Cabinet.", Required: true},
		{ID: "f002", Text: "Doctor Near Cabinet is dead.", Required: true},
		{ID: "f003", Text: "I search Doctor Near Door.", Required: true},
		{ID: "f004", Text: "Doctor Near Door is dead.", Required: true},
	}}
	got := (Fallback{}).Render(bundle)
	want := "I search Doctor Near Cabinet. Doctor Near Cabinet is dead. I search Doctor Near Door. Doctor Near Door is dead."
	if got != want { t.Fatalf("Render = %q, want %q", got, want) }
}

func TestFallbackUsesClarificationWhenNoFacts(t *testing.T) {
	got := (Fallback{}).Render(turn.FactBundle{})
	if got != "What do you want me to do?" { t.Fatalf("Render = %q", got) }
}
```

- [ ] **Step 2: Run test and confirm failure**

Run: `rtk proxy go test ./internal/response -run TestFallback -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the deterministic renderer**

```go
type Response struct {
	Text string
	UsedFallback bool
	FallbackReason string
	UsedFactIDs []game.FactID
}

type Fallback struct{}

func (Fallback) Render(bundle turn.FactBundle) string {
	seen := make(map[game.FactID]bool)
	parts := make([]string, 0, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		text := strings.TrimSpace(fact.Text)
		if !fact.Required || text == "" || seen[fact.ID] { continue }
		seen[fact.ID] = true
		parts = append(parts, ensureSentence(text))
	}
	if len(parts) == 0 { return "What do you want me to do?" }
	return strings.Join(parts, " ")
}
```

`ensureSentence` leaves `.`, `!`, and `?` endings intact and appends `.` otherwise. Do not reorder or regenerate facts.

- [ ] **Step 4: Run tests and commit**

Run: `rtk proxy go test ./internal/response -count=1`

Expected: PASS.

```text
rtk git add internal/response
rtk git commit -m "feat: render deterministic kaya responses"
```

---

### Task 7: Fact-locked Ollama response composition

**Files:**
- Create: `internal/response/prompt.go`
- Create: `internal/response/validator.go`
- Create: `internal/response/composer.go`
- Create: `internal/response/composer_test.go`

**Interfaces:**
- Consumes: schema generator `GenerateJSON(context.Context, string, string, any) (string, error)`
- Produces: `NewComposer(generator StructuredGenerator) Composer`
- Produces: `Composer.Compose(ctx, bundle) Response`

- [ ] **Step 1: Write failing valid-draft and rejection tests**

```go
func TestComposerAcceptsDraftCoveringRequiredFacts(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"I checked Doctor Near Cabinet. The doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if got.UsedFallback || got.Text != "I checked Doctor Near Cabinet. The doctor is dead." { t.Fatalf("response = %#v", got) }
}

func TestComposerRejectsUnknownFactID(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["secret"],"text":"There is a monster here."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "unknown_fact_id" { t.Fatalf("response = %#v", got) }
}

func TestComposerRejectsMissingRequiredFact(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001"],"text":"I checked the first doctor."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "missing_required_fact" { t.Fatalf("response = %#v", got) }
}

func TestComposerRejectsUnknownNamedEntity(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"I checked the doctor beside the Basement Door. The doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "unknown_entity" { t.Fatalf("response = %#v", got) }
}

func TestComposerFallsBackOnGeneratorError(t *testing.T) {
	gen := &fakeGenerator{err: errors.New("offline")}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.Text == "" { t.Fatalf("response = %#v", got) }
}

type fakeGenerator struct { raw string; err error }
func (f *fakeGenerator) GenerateJSON(context.Context, string, string, any) (string, error) { return f.raw, f.err }

func doctorBundle() turn.FactBundle {
	return turn.FactBundle{
		PlayerMessage: "search the doctors are they dead",
		Emotion: kaya.EmotionUneasy,
		Facts: []game.Fact{
			{ID: "f001", Kind: game.FactAction, Subject: "Doctor Near Cabinet", Value: "searched", Text: "I searched Doctor Near Cabinet.", Required: true},
			{ID: "f002", Kind: game.FactLifeStatus, Subject: "Doctor Near Cabinet", Value: "dead", Text: "Doctor Near Cabinet is dead.", Required: true},
		},
	}
}
```

- [ ] **Step 2: Run response tests and confirm failure**

Run: `rtk proxy go test ./internal/response -run TestComposer -count=1`

Expected: FAIL because composer and validator do not exist.

- [ ] **Step 3: Define the response schema and safe prompt**

```go
type ResponseDraft struct { Sentences []DraftSentence `json:"sentences"` }
type DraftSentence struct { FactIDs []game.FactID `json:"factIds"`; Text string `json:"text"` }
```

Define:

```go
var ResponseSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"sentences"},
	"properties": map[string]any{
		"sentences": map[string]any{
			"type": "array", "minItems": 1, "maxItems": 6,
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"factIds", "text"},
				"properties": map[string]any{
					"factIds": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}},
					"text": map[string]any{"type": "string", "minLength": 1, "maxLength": 300},
				},
			},
		},
	},
}
```

The prompt text is: `Write concise first-person dialogue as Dr. Kaya. Preserve action order. Every sentence must cite its supporting fact IDs. Include every required fact. Use only named entities present in the supplied facts. Add no room, exit, item, creature, injury, event, or outcome.`

Marshal only this input:

```go
func responseInput(bundle turn.FactBundle) any {
	return struct {
		PlayerMessage string `json:"playerMessage"`
		Facts []game.Fact `json:"facts"`
		Emotion kaya.Emotion `json:"emotion"`
	}{bundle.PlayerMessage, bundle.Facts, bundle.Emotion}
}
```

- [ ] **Step 4: Implement strict draft validation and fallback**

`validateDraft` must use `DisallowUnknownFields`, require EOF, reject empty or more than six sentences, reject total text above 900 characters, reject unknown fact IDs, require every required fact ID at least once, and reject capitalized multi-word entity labels not present in any approved fact `Subject`, `Value`, or `Text`.

Implement:

```go
func (c Composer) Compose(ctx context.Context, bundle turn.FactBundle) Response {
	fallback := c.fallback.Render(bundle)
	if c.generator == nil { return Response{Text: fallback, UsedFallback: true, FallbackReason: "generator_unavailable"} }
	payload, err := json.Marshal(responseInput(bundle))
	if err != nil { return Response{Text: fallback, UsedFallback: true, FallbackReason: "encode_input"} }
	raw, err := c.generator.GenerateJSON(ctx, SystemPrompt, string(payload), ResponseSchema)
	if err != nil { return Response{Text: fallback, UsedFallback: true, FallbackReason: "generate_failed"} }
	draft, reason := validateDraft(raw, bundle)
	if reason != "" { return Response{Text: fallback, UsedFallback: true, FallbackReason: reason} }
	parts := make([]string, len(draft.Sentences))
	used := make([]game.FactID, 0)
	seen := make(map[game.FactID]bool)
	for i, sentence := range draft.Sentences {
		parts[i] = strings.TrimSpace(sentence.Text)
		for _, id := range sentence.FactIDs { if !seen[id] { seen[id] = true; used = append(used, id) } }
	}
	return Response{Text: strings.Join(parts, " "), UsedFactIDs: used}
}
```

- [ ] **Step 5: Run response tests and commit**

Run: `rtk proxy go test ./internal/response ./internal/llm -count=1`

Expected: PASS.

```text
rtk git add internal/response
rtk git commit -m "feat: compose fact-locked kaya responses"
```

---

### Task 8: CLI orchestration and playtest logging

**Files:**
- Modify: `cmd/kaya/main.go`
- Modify: `cmd/kaya/main_test.go`
- Modify: `internal/intent/ollama_integration_test.go`

**Interfaces:**
- Consumes: `State.PerceptionSnapshot`, contextual parser, `turn.Executor`, `response.Composer`
- Produces: `processPlayerTurn(ctx, message, state, parser, executor, composer) processedTurn`

- [ ] **Step 1: Write a failing CLI turn test**

```go
func TestProcessPlayerTurnUsesPlanExecutorAndComposer(t *testing.T) {
	state := scenario.NewPrototypeWorld()
	parser := fakeTurnParser{plan: intent.TurnPlan{Actions: []intent.PlannedAction{{Intent: intent.Intent{Action: intent.ActionInspect}, TargetMode: intent.TargetSingle}}}}
	composer := fakeComposer{text: "The reception is damaged. I can go east."}
	got, err := processPlayerTurn(context.Background(), "what is around you", state, parser, turn.NewExecutor(state), composer)
	if err != nil { t.Fatal(err) }
	if got.Response.Text != composer.text || len(got.Result.Outcomes) != 1 { t.Fatalf("turn = %#v", got) }
}

type fakeTurnParser struct { plan intent.TurnPlan; err error }
func (f fakeTurnParser) Parse(context.Context, string, game.PerceptionSnapshot) (intent.TurnPlan, error) { return f.plan, f.err }

type fakeComposer struct { text string }
func (f fakeComposer) Compose(context.Context, turn.FactBundle) response.Response { return response.Response{Text: f.text} }
```

- [ ] **Step 2: Run the CLI test and confirm failure**

Run: `rtk proxy go test ./cmd/kaya -run TestProcessPlayerTurnUsesPlanExecutorAndComposer -count=1`

Expected: FAIL because the helper and interfaces do not exist.

- [ ] **Step 3: Extract turn orchestration**

Define small testable interfaces in `cmd/kaya/main.go`:

```go
type turnParser interface { Parse(context.Context, string, game.PerceptionSnapshot) (intent.TurnPlan, error) }
type responseComposer interface { Compose(context.Context, turn.FactBundle) response.Response }
type processedTurn struct { Plan intent.TurnPlan; Result turn.Result; Response response.Response }

func processPlayerTurn(ctx context.Context, message string, state *world.State, parser turnParser, executor turn.Executor, composer responseComposer) (processedTurn, error) {
	snapshot, err := state.PerceptionSnapshot()
	if err != nil { return processedTurn{}, err }
	parseCtx, cancelParse := context.WithTimeout(ctx, 60*time.Second)
	plan, err := parser.Parse(parseCtx, message, snapshot)
	cancelParse()
	if err != nil { return processedTurn{}, err }
	result := executor.Execute(plan)
	bundle := result.FactBundle(message)
	responseCtx, cancelResponse := context.WithTimeout(ctx, 60*time.Second)
	composed := composer.Compose(responseCtx, bundle)
	cancelResponse()
	return processedTurn{Plan: plan, Result: result, Response: composed}, nil
}
```

- [ ] **Step 4: Wire `play`, `intent`, and `playtest`**

In `runPlay`, construct one `turn.Executor` and one `response.Composer` from the existing Ollama client. For each message use a 60-second parsing context and a separate 60-second response context through `processPlayerTurn`. Print total elapsed time once, then `Kaya: <response text>`. Print response fallback reason only with the existing `debug:` prefix.

Change `runIntent` to build `scenario.NewPrototypeWorld().PerceptionSnapshot()`, parse a `TurnPlan`, and print that JSON.

Change playtest step logging from one `Intent`/`ActionResult` to `TurnPlan`/`turn.Result`/`response.Response`. Suspicion checks must flag parser clarification, failed/refused outcomes, and response fallback, while expected action/outcome checks inspect the first planned action and first outcome.

- [ ] **Step 5: Add the user's regression script**

Add this fixed-seed playtest sequence and expectations:

```go
[]playtestMessage{
	{Player: "what is around you", Expect: playtestExpectation{FirstAction: intent.ActionInspect, FirstOutcome: "inspected_room", OutcomeCount: 1}},
	{Player: "what is on the desk", Expect: playtestExpectation{FirstAction: intent.ActionInspect, FirstOutcome: "inspected_object", OutcomeCount: 1}},
	{Player: "look inside the drawers", Expect: playtestExpectation{FirstAction: intent.ActionSearch, FirstOutcome: "searched_found_items", OutcomeCount: 1}},
	{Player: "take the flashlight", Expect: playtestExpectation{FirstAction: intent.ActionTakeItem, FirstOutcome: "item_taken", OutcomeCount: 1}},
	{Player: "go east", Expect: playtestExpectation{FirstAction: intent.ActionMove, FirstOutcome: "moved", OutcomeCount: 1}},
	{Player: "whats around you", Expect: playtestExpectation{FirstAction: intent.ActionInspect, FirstOutcome: "inspected_room", OutcomeCount: 1, ForbidFactText: "north"}},
	{Player: "turn on the flashlight", Expect: playtestExpectation{FirstAction: intent.ActionTurnOn, FirstOutcome: "flashlight_on", OutcomeCount: 1}},
	{Player: "look around", Expect: playtestExpectation{FirstAction: intent.ActionInspect, FirstOutcome: "inspected_room", OutcomeCount: 1, RequireFactText: "north"}},
	{Player: "search the doctors are they dead", Expect: playtestExpectation{FirstAction: intent.ActionSearch, FirstOutcome: "searched_found_items", OutcomeCount: 2, QuestionKind: game.FactLifeStatus, QuestionCount: 2}},
}

type playtestExpectation struct {
	FirstAction intent.Action
	FirstOutcome string
	OutcomeCount int
	QuestionKind game.FactKind
	QuestionCount int
	RequireFactText string
	ForbidFactText string
}
```

Assert the dark inspection exposes west but not north; the lit inspection exposes both doctors and north; the final turn has two outcomes and two `life_status=dead` facts.

- [ ] **Step 6: Run CLI and integration regressions**

Run: `rtk proxy go test ./cmd/kaya ./internal/intent ./internal/turn ./internal/response -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```text
rtk git add cmd/kaya internal/intent/ollama_integration_test.go
rtk git commit -m "feat: run contextual conversational turns"
```

---

### Task 9: Qwen gates, full verification, and milestone status

**Files:**
- Create: `internal/response/ollama_integration_test.go`
- Modify: `internal/intent/ollama_integration_test.go`
- Modify: `docs/intent-parser-prompts.md`
- Modify: `docs/engine-milestones.md`

**Interfaces:**
- Verifies installed `qwen3.5:4b` against real contextual plans and response drafts.
- Records Phase 6 and Phase 7 as complete for this first slice only after all gates pass.

- [ ] **Step 1: Add gated real-model cases**

Under `KAYA_RUN_OLLAMA_TESTS=1`, test these exact inputs with a storage snapshot containing two visible doctors:

```text
inspect the doctor
inspect both doctors
do they have anything
search them
search the doctors are they dead
feel along the walls for another exit
what is isnide the storage cabiner
```

Required assertions: singular doctor produces engine ambiguity after execution; `both/them/the doctors` use `TargetAll`; the compound sentence has one search action plus one life-status question; wall wording maps to `explore`; the cabinet typo resolves to `Storage Cabinet` after engine resolution.

Add a response integration case whose bundle contains two searched doctors, two dead facts, and elapsed time. Assert non-empty natural text, every returned draft fact ID is approved, and no unknown named entity is present. The test must skip when the environment variable is absent.

- [ ] **Step 2: Run deterministic suite**

Run: `rtk proxy go test ./... -count=1`

Expected: PASS.

- [ ] **Step 3: Run real Qwen suite**

Run: `rtk proxy powershell -NoProfile -Command '$env:KAYA_RUN_OLLAMA_TESTS="1"; go test ./internal/intent ./internal/response -count=1 -v'`

Expected: PASS with `qwen3.5:4b` running locally.

- [ ] **Step 4: Run race detector and vet**

Run: `rtk proxy powershell -NoProfile -Command '$env:CGO_ENABLED="1"; $env:CC="C:\msys64\ucrt64\bin\gcc.exe"; go test -race ./... -count=1'`

Expected: PASS.

Run: `rtk proxy go vet ./...`

Expected: no output and exit code 0.

- [ ] **Step 5: Run fixed-seed manual playthrough**

Run: `rtk proxy go run ./cmd/kaya play --seed 12345`

Enter the regression messages from Task 8, then continue: take the generated key, use it on the Emergency Stairwell Door, and go north.

Expected: darkness reveals only west before the flashlight; both doctors are processed separately; Kaya naturally states both life statuses; the run reaches `Prototype objective complete.`

- [ ] **Step 6: Update docs with verified status**

In `docs/engine-milestones.md`, set:

```text
Phase 6 status: Complete for the first context-aware, multi-action intent slice.
Phase 7 status: Complete for the first fact-locked Ollama response slice with deterministic fallback.
```

Update `docs/intent-parser-prompts.md` with the final `TurnPlan` fields, perception allowlist, four-action/four-question limits, repair/fallback path, response fact-ID contract, and the exact gated-test command.

- [ ] **Step 7: Run final diff checks and commit**

Run: `rtk git diff --check`

Expected: no output.

```text
rtk git add internal/intent/ollama_integration_test.go internal/response/ollama_integration_test.go docs/intent-parser-prompts.md docs/engine-milestones.md
rtk git commit -m "docs: complete contextual semantic turn slice"
```

- [ ] **Step 8: Confirm clean branch state**

Run: `rtk git status --short --branch`

Expected: no changed files; branch is ahead only by the implementation commits.
