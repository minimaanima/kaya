# Phase 5 Randomized Run Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate seeded prototype runs and accept them only when a BFS witness reaches the stairwell and the real resolver replays that witness successfully.

**Architecture:** `internal/rungen` owns versioned randomness, placement enumeration, symbolic validation, replay, and generation without importing `scenario`. `internal/scenario` supplies dependency-free content; `internal/runscenario` assembles that content into a prototype `rungen.Definition`. The CLI requests a generated run, prints reproducibility data, then gives its untouched state to the existing resolver loop.

**Tech Stack:** Go standard library, existing `world`, `intent`, `actions`, and `scenario` packages, table-driven Go tests, local Ollama `qwen3.5:4b` only for manual intent-parser verification.

## Global Constraints

- Keep generation and validation independent from Ollama.
- Use private version-1 SplitMix64, unbiased rejection sampling, and Fisher-Yates; do not use global `math/rand` state.
- Sort all IDs before assigning indexes or enumerating combinations.
- Cap placement combinations at 4,096, required items at 64, relevant doors at 64, and BFS states at 10,000.
- Reject unsupported generator versions and fail closed when no proof replays.
- Preserve fixed flashlight/key locations in `scenario.NewPrototypeWorld()` while adding all six candidate objects.
- Use no new external dependency.
- Write each behavior test first, run it to observe the expected failure, then implement only enough production code to pass.
- Run shell commands through `rtk`.

**Execution amendment:** During replay integration, the original `scenario -> rungen` adapter caused an `actions` test import cycle because `rungen` imports `actions`. The adapter was moved to `internal/runscenario`; all implementation and integration references use `runscenario.PrototypeDefinition()`.

---

### Task 1: Versioned Randomness And Placement Combination Order

**Files:**

- Create: `internal/rungen/types.go`
- Create: `internal/rungen/errors.go`
- Create: `internal/rungen/rng.go`
- Create: `internal/rungen/combinations.go`
- Create: `internal/rungen/rng_test.go`
- Create: `internal/rungen/combinations_test.go`

**Interfaces:**

- Consumes: `game.RoomID`, `game.ItemID`, `game.ObjectID`, `intent.Intent`, `world.State`.
- Produces: `RunConfig`, `Definition`, `ItemRule`, `PlacementCandidate`, `Placement`, `WitnessStep`, `ValidationResult`, `GeneratedRun`, generator error sentinels, `CurrentGeneratorVersion`, `placementCombinations`, and `shufflePlacements`.

- [ ] **Step 1: Write failing SplitMix64 golden-vector and bounded-index tests**

```go
func TestSplitMix64GoldenSequence(t *testing.T) {
	rng := splitMix64{state: 0}
	want := []uint64{
		0xe220a8397b1dcdaf,
		0x6e789e6aa1b965f4,
		0x06c45d188009454f,
	}
	for i, expected := range want {
		if got := rng.next(); got != expected {
			t.Fatalf("next %d = %#x, want %#x", i, got, expected)
		}
	}
}

func TestSplitMix64BoundedStaysInsideRange(t *testing.T) {
	rng := splitMix64{state: 17}
	for i := 0; i < 10_000; i++ {
		got := rng.bounded(9)
		if got >= 9 {
			t.Fatalf("bounded(9) = %d", got)
		}
	}
}
```

- [ ] **Step 2: Run the focused test and confirm RED**

Run: `rtk proxy go test ./internal/rungen -run 'TestSplitMix64' -count=1`

Expected: build failure because `splitMix64` does not exist.

- [ ] **Step 3: Add the core public types and stable RNG**

```go
const (
	CurrentGeneratorVersion   = 1
	MaxPlacementCombinations = 4096
	MaxValidationStates       = 10_000
)

var (
	ErrInvalidDefinition  = errors.New("invalid run definition")
	ErrUnsupportedVersion = errors.New("unsupported generator version")
	ErrNoPlayableRun      = errors.New("no playable run")
	ErrValidationLimit    = errors.New("validation state limit reached")
)

type RunConfig struct {
	Seed             int64
	GeneratorVersion int
}

type Definition struct {
	ScenarioID      string
	ScenarioVersion int
	Build           func() *world.State
	StartRoom       game.RoomID
	WinRoom         game.RoomID
	LightItem       game.ItemID
	ItemRules       []ItemRule
}

type ItemRule struct {
	ItemID     game.ItemID
	Candidates []PlacementCandidate
}

type PlacementCandidate struct {
	ObjectID game.ObjectID
}

type Placement struct {
	ItemID   game.ItemID
	ObjectID game.ObjectID
}

type WitnessStep struct {
	Intent          intent.Intent
	ExpectedOutcome string
}

type ValidationResult struct {
	Valid         bool
	Reason        string
	VisitedStates int
	Witness       []WitnessStep
}

type GeneratedRun struct {
	Seed             int64
	GeneratorVersion int
	ScenarioID       string
	ScenarioVersion  int
	State            *world.State
	Placements       []Placement
	Validation       ValidationResult
}

type splitMix64 struct{ state uint64 }

func (r *splitMix64) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *splitMix64) bounded(n uint64) uint64 {
	threshold := -n % n
	for {
		value := r.next()
		if value >= threshold {
			return value % n
		}
	}
}
```

`bounded` is private and is called only with `n > 0` from the shuffle loop.

- [ ] **Step 4: Run the focused test and confirm GREEN**

Run: `rtk proxy go test ./internal/rungen -run 'TestSplitMix64' -count=1`

Expected: both SplitMix64 tests pass.

- [ ] **Step 5: Write failing Cartesian-product and shuffle tests**

```go
func TestPlacementCombinationsAreStableAndComplete(t *testing.T) {
	rules := []ItemRule{
		{ItemID: "flashlight", Candidates: []PlacementCandidate{{ObjectID: "floor"}, {ObjectID: "desk"}, {ObjectID: "chair"}}},
		{ItemID: "key", Candidates: []PlacementCandidate{{ObjectID: "doctor_door"}, {ObjectID: "cabinet"}, {ObjectID: "doctor_cabinet"}}},
	}

	got, err := placementCombinations(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 9 {
		t.Fatalf("combinations = %d, want 9", len(got))
	}
	wantFirst := []Placement{{ItemID: "flashlight", ObjectID: "chair"}, {ItemID: "key", ObjectID: "cabinet"}}
	if !reflect.DeepEqual(got[0], wantFirst) {
		t.Fatalf("first = %+v, want %+v", got[0], wantFirst)
	}
}

func TestShufflePlacementsRepeatsForSeed(t *testing.T) {
	rules := []ItemRule{
		{ItemID: "flashlight", Candidates: []PlacementCandidate{{ObjectID: "desk"}, {ObjectID: "floor"}, {ObjectID: "chair"}}},
		{ItemID: "key", Candidates: []PlacementCandidate{{ObjectID: "cabinet"}, {ObjectID: "doctor"}, {ObjectID: "locker"}}},
	}
	combinations, err := placementCombinations(rules)
	if err != nil {
		t.Fatal(err)
	}
	clone := func(source [][]Placement) [][]Placement {
		result := make([][]Placement, len(source))
		for i := range source {
			result[i] = append([]Placement(nil), source[i]...)
		}
		return result
	}
	a := clone(combinations)
	b := clone(combinations)
	shufflePlacements(a, 12345)
	shufflePlacements(b, 12345)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same seed produced different orders")
	}
}

func TestPlacementCombinationsRejectsProductAboveLimit(t *testing.T) {
	rules := make([]ItemRule, 13)
	for i := range rules {
		rules[i] = ItemRule{
			ItemID: game.ItemID(fmt.Sprintf("item_%02d", i)),
			Candidates: []PlacementCandidate{{ObjectID: "a"}, {ObjectID: "b"}},
		}
	}
	if _, err := placementCombinations(rules); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}
```

- [ ] **Step 6: Run combination tests and confirm RED**

Run: `rtk proxy go test ./internal/rungen -run 'TestPlacementCombinations|TestShufflePlacements' -count=1`

Expected: build failure because the combination functions do not exist.

- [ ] **Step 7: Implement sorted Cartesian products and Fisher-Yates**

```go
func placementCombinations(rules []ItemRule) ([][]Placement, error) {
	normalized := append([]ItemRule(nil), rules...)
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].ItemID < normalized[j].ItemID })
	total := 1
	for i := range normalized {
		normalized[i].Candidates = append([]PlacementCandidate(nil), normalized[i].Candidates...)
		sort.Slice(normalized[i].Candidates, func(a, b int) bool {
			return normalized[i].Candidates[a].ObjectID < normalized[i].Candidates[b].ObjectID
		})
		if len(normalized[i].Candidates) == 0 || total > MaxPlacementCombinations/len(normalized[i].Candidates) {
			return nil, fmt.Errorf("%w: placement combinations exceed %d", ErrInvalidDefinition, MaxPlacementCombinations)
		}
		total *= len(normalized[i].Candidates)
	}

	result := make([][]Placement, 0, total)
	var visit func(int, []Placement)
	visit = func(ruleIndex int, current []Placement) {
		if ruleIndex == len(normalized) {
			result = append(result, append([]Placement(nil), current...))
			return
		}
		rule := normalized[ruleIndex]
		for _, candidate := range rule.Candidates {
			visit(ruleIndex+1, append(current, Placement{ItemID: rule.ItemID, ObjectID: candidate.ObjectID}))
		}
	}
	visit(0, nil)
	return result, nil
}

func shufflePlacements(combinations [][]Placement, seed int64) {
	rng := splitMix64{state: uint64(seed)}
	for i := len(combinations) - 1; i > 0; i-- {
		j := int(rng.bounded(uint64(i + 1)))
		combinations[i], combinations[j] = combinations[j], combinations[i]
	}
}
```

- [ ] **Step 8: Run package tests and commit**

Run: `rtk proxy go test ./internal/rungen -count=1`

Expected: all `rungen` tests pass.

```text
rtk git add internal/rungen
rtk git commit -m "feat: add deterministic run randomness"
```

---

### Task 2: Definition Validation And Safe Placement Application

**Files:**

- Create: `internal/rungen/definition.go`
- Create: `internal/rungen/placement.go`
- Create: `internal/rungen/definition_test.go`
- Create: `internal/rungen/placement_test.go`

**Interfaces:**

- Consumes: Task 1 `Definition`, `Placement`, `CurrentGeneratorVersion`.
- Produces: `ValidateDefinition(Definition) error` and `ApplyPlacements(*world.State, []Placement) error`; consumes Task 1 error sentinels.

- [ ] **Step 1: Write failing definition-validation tests**

```go
func TestValidateDefinitionRejectsDuplicateCandidate(t *testing.T) {
	definition := validTestDefinition()
	definition.ItemRules[0].Candidates = []PlacementCandidate{{ObjectID: "desk"}, {ObjectID: "desk"}}
	if err := ValidateDefinition(definition); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}

func TestValidateDefinitionRejectsNonSearchableCandidate(t *testing.T) {
	definition := validTestDefinition()
	state := definition.Build()
	object := state.Objects["desk"]
	object.Searchable = false
	state.Objects["desk"] = object
	definition.Build = func() *world.State { return state }
	if err := ValidateDefinition(definition); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}

func validTestDefinition() Definition {
	return Definition{
		ScenarioID: "test", ScenarioVersion: 1,
		StartRoom: "start", WinRoom: "win", LightItem: "flashlight",
		Build: func() *world.State {
			state := world.NewState("start")
			state.Rooms["start"] = world.Room{ID: "start", Objects: []game.ObjectID{"desk", "floor"}, Exits: []world.Exit{{Direction: "north", To: "win"}}}
			state.Rooms["win"] = world.Room{ID: "win"}
			state.Objects["desk"] = world.Object{ID: "desk", Name: "Desk", Searchable: true}
			state.Objects["floor"] = world.Object{ID: "floor", Name: "Floor", Searchable: true}
			state.Items["flashlight"] = world.Item{ID: "flashlight", Name: "Flashlight", Portable: true}
			return state
		},
		ItemRules: []ItemRule{{ItemID: "flashlight", Candidates: []PlacementCandidate{{ObjectID: "desk"}, {ObjectID: "floor"}}}},
	}
}
```

The fixture builds two rooms, one searchable object, one portable required item, and valid start/win rooms.

- [ ] **Step 2: Run validation tests and confirm RED**

Run: `rtk proxy go test ./internal/rungen -run 'TestValidateDefinition' -count=1`

Expected: build failure because the exported validation API does not exist.

- [ ] **Step 3: Implement complete structural validation**

```go
func ValidateDefinition(def Definition) error {
	if strings.TrimSpace(def.ScenarioID) == "" || def.ScenarioVersion <= 0 || def.Build == nil {
		return fmt.Errorf("%w: scenario identity and builder are required", ErrInvalidDefinition)
	}
	state := def.Build()
	if state == nil {
		return fmt.Errorf("%w: builder returned nil", ErrInvalidDefinition)
	}
	if _, ok := state.Rooms[def.StartRoom]; !ok {
		return fmt.Errorf("%w: start room %q missing", ErrInvalidDefinition, def.StartRoom)
	}
	if _, ok := state.Rooms[def.WinRoom]; !ok {
		return fmt.Errorf("%w: win room %q missing", ErrInvalidDefinition, def.WinRoom)
	}
	if len(def.ItemRules) == 0 || len(def.ItemRules) > 64 {
		return fmt.Errorf("%w: required item count must be 1..64", ErrInvalidDefinition)
	}
	seenItems := make(map[game.ItemID]bool, len(def.ItemRules))
	for _, rule := range def.ItemRules {
		item, ok := state.Items[rule.ItemID]
		if !ok || !item.Portable || seenItems[rule.ItemID] || len(rule.Candidates) == 0 {
			return fmt.Errorf("%w: invalid required item %q", ErrInvalidDefinition, rule.ItemID)
		}
		seenItems[rule.ItemID] = true
		seenObjects := make(map[game.ObjectID]bool, len(rule.Candidates))
		for _, candidate := range rule.Candidates {
			object, ok := state.Objects[candidate.ObjectID]
			if !ok || !object.Searchable || seenObjects[candidate.ObjectID] {
				return fmt.Errorf("%w: invalid candidate %q for %q", ErrInvalidDefinition, candidate.ObjectID, rule.ItemID)
			}
			seenObjects[candidate.ObjectID] = true
		}
	}
	if def.LightItem != "" && !seenItems[def.LightItem] {
		return fmt.Errorf("%w: light item %q is not required", ErrInvalidDefinition, def.LightItem)
	}
	_, err := placementCombinations(def.ItemRules)
	return err
}
```

- [ ] **Step 4: Run validation tests and confirm GREEN**

Run: `rtk proxy go test ./internal/rungen -run 'TestValidateDefinition' -count=1`

Expected: validation tests pass.

- [ ] **Step 5: Write failing placement tests**

```go
func TestApplyPlacementsPlacesEachItemOnce(t *testing.T) {
	state := validTestDefinition().Build()
	err := ApplyPlacements(state, []Placement{{ItemID: "flashlight", ObjectID: "desk"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Objects["desk"].ContainedItems; !reflect.DeepEqual(got, []game.ItemID{"flashlight"}) {
		t.Fatalf("contained items = %v", got)
	}
}

func TestApplyPlacementsRejectsDuplicateItem(t *testing.T) {
	state := validTestDefinition().Build()
	err := ApplyPlacements(state, []Placement{{ItemID: "flashlight", ObjectID: "desk"}, {ItemID: "flashlight", ObjectID: "floor"}})
	if !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
}
```

- [ ] **Step 6: Run placement tests and confirm RED**

Run: `rtk proxy go test ./internal/rungen -run 'TestApplyPlacements' -count=1`

Expected: build failure because `ApplyPlacements` does not exist.

- [ ] **Step 7: Implement atomic placement validation and mutation**

`ApplyPlacements` first validates the state, every item/object ID, duplicate items, and whether a required item is already present anywhere. It builds updated object copies in memory. Only after every placement validates does it assign those copies back to `state.Objects`, preventing half-applied worlds.

```go
func ApplyPlacements(state *world.State, placements []Placement) error {
	if state == nil {
		return fmt.Errorf("%w: world is nil", ErrInvalidDefinition)
	}
	updates := make(map[game.ObjectID]world.Object)
	seen := make(map[game.ItemID]bool, len(placements))
	for _, placement := range placements {
		if seen[placement.ItemID] {
			return fmt.Errorf("%w: duplicate placement for %q", ErrInvalidDefinition, placement.ItemID)
		}
		seen[placement.ItemID] = true
		if _, ok := state.Items[placement.ItemID]; !ok {
			return fmt.Errorf("%w: item %q missing", ErrInvalidDefinition, placement.ItemID)
		}
		for _, existingObject := range state.Objects {
			for _, existingItem := range existingObject.ContainedItems {
				if existingItem == placement.ItemID {
					return fmt.Errorf("%w: item %q already placed", ErrInvalidDefinition, placement.ItemID)
				}
			}
		}
		object, ok := state.Objects[placement.ObjectID]
		if !ok || !object.Searchable {
			return fmt.Errorf("%w: candidate object %q invalid", ErrInvalidDefinition, placement.ObjectID)
		}
		if updated, ok := updates[placement.ObjectID]; ok {
			object = updated
		}
		object.ContainedItems = append(append([]game.ItemID(nil), object.ContainedItems...), placement.ItemID)
		updates[placement.ObjectID] = object
	}
	for objectID, object := range updates {
		state.Objects[objectID] = object
	}
	return nil
}
```

- [ ] **Step 8: Run package tests and commit**

Run: `rtk proxy go test ./internal/rungen -count=1`

Expected: all `rungen` tests pass.

```text
rtk git add internal/rungen
rtk git commit -m "feat: validate run definitions and placements"
```

---

### Task 3: Prototype Template And Six Placement Candidates

**Files:**

- Modify: `internal/scenario/prototype.go`
- Modify: `internal/scenario/prototype_test.go`

**Interfaces:**

- Consumes: Task 1 `rungen.Definition`, `ItemRule`, and `PlacementCandidate`.
- Produces: `NewPrototypeTemplate() *world.State`, `PrototypeRunDefinition() rungen.Definition`, three Reception flashlight candidates, and three Storage key candidates.

- [ ] **Step 1: Write failing template and definition tests**

```go
func TestNewPrototypeTemplateHasSixEmptyCandidateObjects(t *testing.T) {
	state := NewPrototypeTemplate()
	if len(state.Objects) != 6 {
		t.Fatalf("objects = %d, want 6", len(state.Objects))
	}
	for _, object := range state.Objects {
		for _, itemID := range object.ContainedItems {
			if itemID == ItemFlashlight || itemID == ItemBrassKey {
				t.Fatalf("template pre-placed %q in %q", itemID, object.ID)
			}
		}
	}
}

func TestNewPrototypeWorldKeepsFixedPlacements(t *testing.T) {
	state := NewPrototypeWorld()
	if !slices.Contains(state.Objects[ObjectReceptionDesk].ContainedItems, ItemFlashlight) {
		t.Fatal("fixed world flashlight not in reception desk")
	}
	if !slices.Contains(state.Objects[ObjectBodyCabinet].ContainedItems, ItemBrassKey) {
		t.Fatal("fixed world key not on doctor near cabinet")
	}
}

func TestPrototypeRunDefinitionHasThreeCandidatesPerItem(t *testing.T) {
	definition := PrototypeRunDefinition()
	if err := rungen.ValidateDefinition(definition); err != nil {
		t.Fatal(err)
	}
	if len(definition.ItemRules) != 2 || len(definition.ItemRules[0].Candidates) != 3 || len(definition.ItemRules[1].Candidates) != 3 {
		t.Fatalf("rules = %+v", definition.ItemRules)
	}
}
```

- [ ] **Step 2: Run scenario tests and confirm RED**

Run: `rtk proxy go test ./internal/scenario -count=1`

Expected: build failure because the template and definition functions do not exist.

- [ ] **Step 3: Split the template and add content-owned candidates**

Add constants:

```go
const (
	ObjectReceptionFloor  game.ObjectID = "reception_floor"
	ObjectCollapsedChair  game.ObjectID = "collapsed_chair"
	ObjectStorageCabinet  game.ObjectID = "storage_cabinet"
	PrototypeScenarioID                 = "prototype_escape"
	PrototypeScenarioVersion            = 1
)
```

`NewPrototypeTemplate` contains the existing rooms, door, items, event, and objects plus:

```go
state.Objects[ObjectReceptionFloor] = world.Object{
	ID: ObjectReceptionFloor, Name: "Reception Floor", Aliases: []string{"floor", "reception floor"},
	Description: "Broken tiles and fallen ceiling panels cover the floor.", Kind: world.ObjectSurface, Searchable: true,
}
state.Objects[ObjectCollapsedChair] = world.Object{
	ID: ObjectCollapsedChair, Name: "Collapsed Chair", Aliases: []string{"chair", "collapsed chair"},
	Description: "A collapsed office chair lies beneath a torn coat.", Kind: world.ObjectSurface, Searchable: true,
}
state.Objects[ObjectStorageCabinet] = world.Object{
	ID: ObjectStorageCabinet, Name: "Storage Cabinet", Aliases: []string{"cabinet", "storage cabinet"},
	Description: "A dented storage cabinet stands against the dark wall.", Kind: world.ObjectContainer, RequiresLight: true, Searchable: true,
}
```

Append the Reception objects to `RoomReception.Objects` and the cabinet to `RoomStorage.Objects`. Leave randomized required items unplaced in the template.

```go
func NewPrototypeWorld() *world.State {
	state := NewPrototypeTemplate()
	desk := state.Objects[ObjectReceptionDesk]
	desk.ContainedItems = []game.ItemID{ItemFlashlight}
	state.Objects[desk.ID] = desk
	body := state.Objects[ObjectBodyCabinet]
	body.ContainedItems = []game.ItemID{ItemBrassKey}
	state.Objects[body.ID] = body
	return state
}

func PrototypeRunDefinition() rungen.Definition {
	return rungen.Definition{
		ScenarioID: PrototypeScenarioID, ScenarioVersion: PrototypeScenarioVersion,
		Build: NewPrototypeTemplate, StartRoom: RoomReception, WinRoom: RoomStairwell, LightItem: ItemFlashlight,
		ItemRules: []rungen.ItemRule{
			{ItemID: ItemFlashlight, Candidates: []rungen.PlacementCandidate{{ObjectID: ObjectReceptionDesk}, {ObjectID: ObjectReceptionFloor}, {ObjectID: ObjectCollapsedChair}}},
			{ItemID: ItemBrassKey, Candidates: []rungen.PlacementCandidate{{ObjectID: ObjectBodyCabinet}, {ObjectID: ObjectBodyDoor}, {ObjectID: ObjectStorageCabinet}}},
		},
	}
}
```

- [ ] **Step 4: Run scenario and full regression tests**

Run: `rtk proxy go test ./internal/scenario ./internal/actions -count=1`

Expected: scenario and resolver tests pass after updating the old object-count assertion from three to six.

- [ ] **Step 5: Format and commit**

```text
rtk gofmt -w internal/scenario
rtk git add internal/scenario
rtk git commit -m "feat: add prototype placement candidates"
```

---

### Task 4: Symbolic BFS Playability Validator

**Files:**

- Modify: `internal/world/visibility.go`
- Modify: `internal/world/state.go`
- Modify: `internal/world/state_test.go`
- Create: `internal/rungen/validator.go`
- Create: `internal/rungen/validator_test.go`

**Interfaces:**

- Consumes: `Definition`, a freshly built and placed `world.State`, `world.Door.IsPassable`, and `world.Door.CanUnlockWith`.
- Produces: `world.CanSeeObject(room, object, activeLight) bool` and `rungen.Validate(definition, state) (ValidationResult, error)`.

- [ ] **Step 1: Write a failing pure visibility test**

```go
func TestCanSeeObjectUsesExplicitLightState(t *testing.T) {
	room := Room{Visibility: VisibilityPitchBlack}
	object := Object{RequiresLight: false}
	if CanSeeObject(room, object, false) {
		t.Fatal("pitch-black object visible without light")
	}
	if !CanSeeObject(room, object, true) {
		t.Fatal("object hidden with active light")
	}
}
```

- [ ] **Step 2: Run the visibility test and confirm RED**

Run: `rtk proxy go test ./internal/world -run TestCanSeeObjectUsesExplicitLightState -count=1`

Expected: build failure because the pure function does not exist.

- [ ] **Step 3: Extract the shared pure visibility rule**

```go
func CanSeeObject(room Room, object Object, activeLight bool) bool {
	if activeLight {
		return true
	}
	if room.Visibility == VisibilityPitchBlack {
		return false
	}
	if room.NeedsLight() {
		return !object.RequiresLight
	}
	return true
}

func (s *State) CanSeeObject(room Room, object Object) bool {
	return CanSeeObject(room, object, s != nil && s.ActiveLight)
}
```

- [ ] **Step 4: Run world tests and confirm GREEN**

Run: `rtk proxy go test ./internal/world -count=1`

Expected: all world tests pass.

- [ ] **Step 5: Write failing validator integration tests**

Use external package `rungen_test` so tests can import both `scenario` and `rungen` without creating a production import cycle.

```go
func TestValidateReturnsWitnessForPrototypePlacement(t *testing.T) {
	definition := scenario.PrototypeRunDefinition()
	state := definition.Build()
	placements := []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectReceptionDesk},
		{ItemID: scenario.ItemBrassKey, ObjectID: scenario.ObjectBodyCabinet},
	}
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatal(err)
	}
	result, err := rungen.Validate(definition, state)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || len(result.Witness) == 0 {
		t.Fatalf("validation = %+v", result)
	}
}

func TestValidateRejectsFlashlightInDarkStorage(t *testing.T) {
	definition := scenario.PrototypeRunDefinition()
	state := definition.Build()
	placements := []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectStorageCabinet},
		{ItemID: scenario.ItemBrassKey, ObjectID: scenario.ObjectBodyCabinet},
	}
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatal(err)
	}
	result, err := rungen.Validate(definition, state)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.Reason != "win room unreachable" {
		t.Fatalf("validation = %+v", result)
	}
}
```

- [ ] **Step 6: Run validator tests and confirm RED**

Run: `rtk proxy go test ./internal/rungen -run 'TestValidate' -count=1`

Expected: build failure because `Validate` does not exist.

- [ ] **Step 7: Implement deterministic symbolic BFS**

Use this comparable state and predecessor model:

```go
type symbolicState struct {
	Room       game.RoomID
	Discovered uint64
	Inventory  uint64
	Unlocked   uint64
	LightOn    bool
}

type predecessor struct {
	Previous symbolicState
	Step     WitnessStep
}

type transition struct {
	Next symbolicState
	Step WitnessStep
	Key  string
}
```

`newProofModel` sorts required item IDs and relevant door IDs, assigns each a bit, verifies no required item is placed more than once, maps every placed item to its object and room, and rejects more than 64 relevant doors.

`transitions` returns, sorted by `Key`, exactly these actions:

- Search each visible searchable object containing at least one undiscovered required item; discover all its required items and expect `searched_found_items`.
- Take each discovered portable required item in the current visible object; add its inventory bit and expect `item_taken`.
- Turn on `Definition.LightItem` when carried and light is off; expect `flashlight_on`.
- Unlock an adjacent locked door when its required key is carried; set its door bit and expect `door_unlocked`.
- Move through exits with no door, an initially passable door, or an unlocked door bit; expect `moved`.

The BFS loop is:

```go
func Validate(def Definition, state *world.State) (ValidationResult, error) {
	if err := ValidateDefinition(def); err != nil {
		return ValidationResult{}, err
	}
	model, err := newProofModel(def, state)
	if err != nil {
		return ValidationResult{}, err
	}
	start := symbolicState{Room: def.StartRoom}
	queue := []symbolicState{start}
	visited := map[symbolicState]bool{start: true}
	parents := make(map[symbolicState]predecessor)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.Room == def.WinRoom {
			return ValidationResult{Valid: true, VisitedStates: len(visited), Witness: buildWitness(start, current, parents)}, nil
		}
		for _, move := range model.transitions(current) {
			if visited[move.Next] {
				continue
			}
			if len(visited) >= MaxValidationStates {
				return ValidationResult{}, fmt.Errorf("%w: exceeded %d states", ErrValidationLimit, MaxValidationStates)
			}
			visited[move.Next] = true
			parents[move.Next] = predecessor{Previous: current, Step: move.Step}
			queue = append(queue, move.Next)
		}
	}
	return ValidationResult{Valid: false, Reason: "win room unreachable", VisitedStates: len(visited)}, nil
}
```

`buildWitness` walks parents from win to start and reverses the collected steps. All generated intents use exact world names: object name for search, item name for take/turn-on/use, door name for use, and exit direction for move.

- [ ] **Step 8: Add the remaining negative validator cases**

Add table rows that mutate a fresh prototype for: flashlight behind stairwell, key in stairwell, key missing, key in a non-searchable object, and stairwell door requiring an unavailable item. Each case must return either `Valid == false` or an `ErrInvalidDefinition`-wrapped structural error, never a valid witness.

- [ ] **Step 9: Run validator and regression tests, then commit**

Run: `rtk proxy go test ./internal/world ./internal/rungen ./internal/actions -count=1`

Expected: all tests pass.

```text
rtk gofmt -w internal/world internal/rungen
rtk git add internal/world internal/rungen
rtk git commit -m "feat: prove generated runs with bfs"
```

---

### Task 5: Authoritative Resolver Replay

**Files:**

- Create: `internal/rungen/replay.go`
- Create: `internal/rungen/replay_test.go`

**Interfaces:**

- Consumes: `Definition.Build`, `ApplyPlacements`, `WitnessStep`, and `actions.NewResolver`.
- Produces: `Replay(def Definition, placements []Placement, witness []WitnessStep) error`.

- [ ] **Step 1: Write failing successful-replay and autonomy-rejection tests**

```go
func TestReplayWitnessReachesPrototypeWinRoom(t *testing.T) {
	definition, placements, witness := provenPrototype(t)
	if err := rungen.Replay(definition, placements, witness); err != nil {
		t.Fatal(err)
	}
}

func provenPrototype(t *testing.T) (rungen.Definition, []rungen.Placement, []rungen.WitnessStep) {
	t.Helper()
	definition := scenario.PrototypeRunDefinition()
	placements := []rungen.Placement{
		{ItemID: scenario.ItemFlashlight, ObjectID: scenario.ObjectReceptionDesk},
		{ItemID: scenario.ItemBrassKey, ObjectID: scenario.ObjectBodyCabinet},
	}
	state := definition.Build()
	if err := rungen.ApplyPlacements(state, placements); err != nil {
		t.Fatal(err)
	}
	proof, err := rungen.Validate(definition, state)
	if err != nil || !proof.Valid {
		t.Fatalf("proof=%+v err=%v", proof, err)
	}
	return definition, placements, proof.Witness
}

func TestReplayRejectsAutonomyRefusal(t *testing.T) {
	definition, placements, witness := provenPrototype(t)
	build := definition.Build
	definition.Build = func() *world.State {
		state := build()
		state.Kaya = kaya.State{Stress: 85, Trust: 5, Fear: 80}
		return state
	}
	err := rungen.Replay(definition, placements, witness)
	if err == nil || !strings.Contains(err.Error(), "kaya_refused") {
		t.Fatalf("error = %v, want kaya_refused", err)
	}
}
```

- [ ] **Step 2: Run replay tests and confirm RED**

Run: `rtk proxy go test ./internal/rungen -run 'TestReplay' -count=1`

Expected: build failure because `Replay` does not exist.

- [ ] **Step 3: Implement resolver replay on a fresh world**

```go
func Replay(def Definition, placements []Placement, witness []WitnessStep) error {
	state := def.Build()
	if err := ApplyPlacements(state, placements); err != nil {
		return fmt.Errorf("apply replay placements: %w", err)
	}
	resolver := actions.NewResolver(state)
	for index, step := range witness {
		result := resolver.Resolve(step.Intent)
		if result.NeedsClarification || result.Outcome != step.ExpectedOutcome {
			return fmt.Errorf("replay step %d action %s: outcome %q, want %q", index+1, step.Intent.Action, result.Outcome, step.ExpectedOutcome)
		}
	}
	if state.CurrentRoomID != def.WinRoom {
		return fmt.Errorf("replay ended in %q, want %q", state.CurrentRoomID, def.WinRoom)
	}
	return nil
}
```

- [ ] **Step 4: Run replay, resolver, and autonomy tests**

Run: `rtk proxy go test ./internal/rungen ./internal/actions ./internal/kaya -count=1`

Expected: all tests pass.

- [ ] **Step 5: Format and commit**

```text
rtk gofmt -w internal/rungen
rtk git add internal/rungen
rtk git commit -m "feat: replay generation proofs through resolver"
```

---

### Task 6: Accepted-Run Generator, Exhaustive Proof, And Seed Sweep

**Files:**

- Create: `internal/rungen/generator.go`
- Create: `internal/rungen/generator_test.go`
- Create: `internal/rungen/prototype_integration_test.go`

**Interfaces:**

- Consumes: all prior `rungen` APIs and `scenario.PrototypeRunDefinition` from external tests.
- Produces: `Generate(config RunConfig, definition Definition) (GeneratedRun, error)` and `GenerationError` with `Unwrap() error` returning `ErrNoPlayableRun`.

- [ ] **Step 1: Write failing deterministic-generation tests**

```go
func TestGenerateSameSeedReturnsSameRun(t *testing.T) {
	definition := scenario.PrototypeRunDefinition()
	config := rungen.RunConfig{Seed: 12345, GeneratorVersion: rungen.CurrentGeneratorVersion}
	a, err := rungen.Generate(config, definition)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rungen.Generate(config, definition)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Placements, b.Placements) || !reflect.DeepEqual(a.Validation.Witness, b.Validation.Witness) {
		t.Fatalf("same seed differed: %+v / %+v", a, b)
	}
}

func TestGenerateReturnsUntouchedInitialState(t *testing.T) {
	run := generatePrototype(t, 7)
	if run.State.CurrentRoomID != scenario.RoomReception || run.State.NowSeconds != 0 || len(run.State.Inventory) != 0 || len(run.State.DiscoveredItems) != 0 {
		t.Fatalf("returned state was consumed: %+v", run.State)
	}
}

func TestGenerateRejectsUnsupportedVersion(t *testing.T) {
	_, err := rungen.Generate(
		rungen.RunConfig{Seed: 1, GeneratorVersion: 99},
		scenario.PrototypeRunDefinition(),
	)
	if !errors.Is(err, rungen.ErrUnsupportedVersion) {
		t.Fatalf("error = %v, want ErrUnsupportedVersion", err)
	}
}
```

- [ ] **Step 2: Run generation tests and confirm RED**

Run: `rtk proxy go test ./internal/rungen -run 'TestGenerate' -count=1`

Expected: build failure because `Generate` does not exist.

- [ ] **Step 3: Implement generate-shuffle-prove-replay-accept**

```go
func Generate(config RunConfig, def Definition) (GeneratedRun, error) {
	if config.GeneratorVersion != CurrentGeneratorVersion {
		return GeneratedRun{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, config.GeneratorVersion)
	}
	if err := ValidateDefinition(def); err != nil {
		return GeneratedRun{}, err
	}
	combinations, err := placementCombinations(def.ItemRules)
	if err != nil {
		return GeneratedRun{}, err
	}
	shufflePlacements(combinations, config.Seed)
	reasons := make([]string, 0, len(combinations))
	for _, placements := range combinations {
		proofState := def.Build()
		if err := ApplyPlacements(proofState, placements); err != nil {
			reasons = append(reasons, err.Error())
			continue
		}
		validation, err := Validate(def, proofState)
		if err != nil {
			reasons = append(reasons, err.Error())
			continue
		}
		if !validation.Valid {
			reasons = append(reasons, validation.Reason)
			continue
		}
		if err := Replay(def, placements, validation.Witness); err != nil {
			reasons = append(reasons, err.Error())
			continue
		}
		playerState := def.Build()
		if err := ApplyPlacements(playerState, placements); err != nil {
			return GeneratedRun{}, err
		}
		return GeneratedRun{
			Seed: config.Seed, GeneratorVersion: config.GeneratorVersion,
			ScenarioID: def.ScenarioID, ScenarioVersion: def.ScenarioVersion,
			State: playerState, Placements: append([]Placement(nil), placements...), Validation: validation,
		}, nil
	}
	return GeneratedRun{}, GenerationError{Attempts: len(combinations), Reasons: reasons}
}
```

`GenerationError.Error` reports the attempt count and joins at most the first ten concise reasons. `GenerationError.Unwrap` returns `ErrNoPlayableRun`.

```go
type GenerationError struct {
	Attempts int
	Reasons  []string
}

func (e GenerationError) Error() string {
	limit := len(e.Reasons)
	if limit > 10 {
		limit = 10
	}
	return fmt.Sprintf("%s after %d attempts: %s", ErrNoPlayableRun, e.Attempts, strings.Join(e.Reasons[:limit], "; "))
}

func (e GenerationError) Unwrap() error { return ErrNoPlayableRun }
```

- [ ] **Step 4: Write exhaustive nine-combination and 1,000-seed tests**

```go
func TestEveryPrototypePlacementCombinationProvesAndReplays(t *testing.T) {
	definition := scenario.PrototypeRunDefinition()
	for _, flashlight := range definition.ItemRules[0].Candidates {
		for _, key := range definition.ItemRules[1].Candidates {
			placements := []rungen.Placement{{ItemID: scenario.ItemFlashlight, ObjectID: flashlight.ObjectID}, {ItemID: scenario.ItemBrassKey, ObjectID: key.ObjectID}}
			state := definition.Build()
			if err := rungen.ApplyPlacements(state, placements); err != nil {
				t.Fatal(err)
			}
			proof, err := rungen.Validate(definition, state)
			if err != nil || !proof.Valid {
				t.Fatalf("placements %+v: proof=%+v err=%v", placements, proof, err)
			}
			if err := rungen.Replay(definition, placements, proof.Witness); err != nil {
				t.Fatalf("placements %+v: %v", placements, err)
			}
		}
	}
}

func TestPrototypeSeedSweep(t *testing.T) {
	definition := scenario.PrototypeRunDefinition()
	seen := make(map[string]bool)
	for seed := int64(1); seed <= 1000; seed++ {
		run, err := rungen.Generate(rungen.RunConfig{Seed: seed, GeneratorVersion: rungen.CurrentGeneratorVersion}, definition)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		seen[fmt.Sprint(run.Placements)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("1,000 seeds produced %d unique placements", len(seen))
	}
}

func generatePrototype(t *testing.T, seed int64) rungen.GeneratedRun {
	t.Helper()
	run, err := rungen.Generate(
		rungen.RunConfig{Seed: seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
		scenario.PrototypeRunDefinition(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return run
}
```

- [ ] **Step 5: Run integration tests and confirm GREEN**

Run: `rtk proxy go test ./internal/rungen -count=1`

Expected: all nine combinations and all 1,000 seeds pass.

- [ ] **Step 6: Format and commit**

```text
rtk gofmt -w internal/rungen
rtk git add internal/rungen
rtk git commit -m "feat: generate only proven playable runs"
```

---

### Task 7: Seeded CLI And Qwen Default

**Files:**

- Modify: `cmd/kaya/main.go`
- Modify: `cmd/kaya/main_test.go`

**Interfaces:**

- Consumes: `rungen.Generate`, `scenario.PrototypeRunDefinition`, and `GeneratedRun.State`.
- Produces: `parsePlayOptions`, `newRunSeed`, `printRunDebug`, `kaya play --seed <int64>`, and `defaultOllamaModel = "qwen3.5:4b"`.

- [ ] **Step 1: Write failing seed-option tests**

```go
func TestParsePlayOptionsUsesExplicitSeed(t *testing.T) {
	options, err := parsePlayOptions([]string{"--seed", "-42"}, func() (int64, error) { return 99, nil })
	if err != nil {
		t.Fatal(err)
	}
	if options.Seed != -42 {
		t.Fatalf("seed = %d, want -42", options.Seed)
	}
}

func TestParsePlayOptionsGeneratesMissingSeed(t *testing.T) {
	options, err := parsePlayOptions(nil, func() (int64, error) { return 99, nil })
	if err != nil || options.Seed != 99 {
		t.Fatalf("options=%+v err=%v", options, err)
	}
}

func TestParsePlayOptionsRejectsPositionals(t *testing.T) {
	if _, err := parsePlayOptions([]string{"extra"}, func() (int64, error) { return 1, nil }); err == nil {
		t.Fatal("expected positional argument error")
	}
}
```

- [ ] **Step 2: Run CLI tests and confirm RED**

Run: `rtk proxy go test ./cmd/kaya -run 'TestParsePlayOptions' -count=1`

Expected: build failure because the parser does not exist.

- [ ] **Step 3: Implement flag parsing and cryptographic seed creation**

```go
const defaultOllamaModel = "qwen3.5:4b"

type playOptions struct{ Seed int64 }

func parsePlayOptions(args []string, generateSeed func() (int64, error)) (playOptions, error) {
	flags := flag.NewFlagSet("play", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	seed := flags.Int64("seed", 0, "reproducible run seed")
	if err := flags.Parse(args); err != nil {
		return playOptions{}, err
	}
	if flags.NArg() != 0 {
		return playOptions{}, fmt.Errorf("usage: kaya play [--seed <int64>]")
	}
	provided := false
	flags.Visit(func(current *flag.Flag) { provided = provided || current.Name == "seed" })
	if provided {
		return playOptions{Seed: *seed}, nil
	}
	generated, err := generateSeed()
	if err != nil {
		return playOptions{}, err
	}
	return playOptions{Seed: generated}, nil
}

func newRunSeed() (int64, error) {
	for {
		var bytes [8]byte
		if _, err := cryptorand.Read(bytes[:]); err != nil {
			return 0, fmt.Errorf("generate run seed: %w", err)
		}
		seed := int64(binary.LittleEndian.Uint64(bytes[:]) & math.MaxInt64)
		if seed != 0 {
			return seed, nil
		}
	}
}
```

- [ ] **Step 4: Run seed tests and confirm GREEN**

Run: `rtk proxy go test ./cmd/kaya -run 'TestParsePlayOptions' -count=1`

Expected: all option tests pass.

- [ ] **Step 5: Write failing generation-debug output test**

```go
func TestPrintRunDebugIncludesReproductionData(t *testing.T) {
	run := mustGenerateTestRun(t, 12345)
	var output strings.Builder
	printRunDebug(&output, run)
	for _, expected := range []string{"Run seed: 12345", "Generator: 1", "Flashlight:", "Brass Key:", "Validation: playable"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output %q missing %q", output.String(), expected)
		}
	}
}

func mustGenerateTestRun(t *testing.T, seed int64) rungen.GeneratedRun {
	t.Helper()
	run, err := rungen.Generate(
		rungen.RunConfig{Seed: seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
		scenario.PrototypeRunDefinition(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return run
}
```

- [ ] **Step 6: Wire generated state into play and print diagnostics**

Change the dispatch to `runPlay(os.Args[2:])`. Inside `runPlay`:

```go
options, err := parsePlayOptions(args, newRunSeed)
if err != nil {
	return err
}
run, err := rungen.Generate(
	rungen.RunConfig{Seed: options.Seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
	scenario.PrototypeRunDefinition(),
)
if err != nil {
	return fmt.Errorf("generate run: %w", err)
}
printRunDebug(os.Stdout, run)
state := run.State
resolver := actions.NewResolver(state)
```

`printRunDebug(io.Writer, GeneratedRun)` sorts placements by item ID, resolves human names through `run.State.Items` and `run.State.Objects`, and prints seed, generator version, each placement, visited states, and witness length.

Replace all three `mistral:latest` defaults with `defaultOllamaModel`. Update usage to `kaya <intent|play|playtest>` and `kaya play [--seed <int64>]`.

- [ ] **Step 7: Run CLI and full unit tests**

Run: `rtk proxy go test ./cmd/kaya ./... -count=1`

Expected: all tests pass.

- [ ] **Step 8: Format and commit**

```text
rtk gofmt -w cmd internal
rtk git add cmd/kaya/main.go cmd/kaya/main_test.go
rtk git commit -m "feat: expose reproducible run seeds"
```

---

### Task 8: Milestone Status And Final Verification

**Files:**

- Modify: `docs/engine-milestones.md`
- Modify: `docs/phase-5-randomized-run-generation.md`

**Interfaces:**

- Consumes: completed implementation and verification evidence.
- Produces: accurate Phase 5 status and manual reproduction instructions.

- [ ] **Step 1: Run deterministic verification before changing status**

```text
rtk proxy go test ./... -count=1
rtk proxy go test -race ./... -count=1
rtk proxy go vet ./...
rtk git diff --check
```

Expected: every command exits zero; normal unit tests report at least the previous 68 tests plus the new Phase 5 suite.

- [ ] **Step 2: Run the gated Ollama parser suite with the installed model**

Run from PowerShell:

```powershell
$env:KAYA_RUN_OLLAMA_TESTS='1'
$env:KAYA_OLLAMA_MODEL='qwen3.5:4b'
rtk proxy go test ./internal/intent -run TestOllamaNaturalLanguageIntents -count=1
```

Expected: the natural-language intent cases pass. If model variance fails, record the exact phrase and parser output; do not weaken deterministic generation tests.

- [ ] **Step 3: Perform a fixed-seed manual smoke test**

Run: `rtk proxy go run ./cmd/kaya play --seed 12345`

Verify startup prints seed `12345`, generator version `1`, placements, and a playable witness count. Complete the generated path using the shown placement objects and confirm `Prototype objective complete.`

- [ ] **Step 4: Mark the milestone using verified wording**

Under Phase 5 add:

```text
Status:

Complete for the first seeded, playability-proven prototype slice.
```

Append CLI examples and the exact proof pipeline to the Phase 5 document:

```text
kaya play --seed 12345

seeded placement
+ symbolic BFS witness
+ resolver replay
= accepted playable run
```

- [ ] **Step 5: Re-run documentation checks and commit**

Run: `rtk git diff --check`

Expected: no whitespace errors.

```text
rtk git add docs/engine-milestones.md docs/phase-5-randomized-run-generation.md
rtk git commit -m "docs: mark phase 5 prototype complete"
```

- [ ] **Step 6: Final clean-state evidence**

```text
rtk proxy go test ./... -count=1
rtk proxy go test -race ./... -count=1
rtk proxy go vet ./...
rtk git status --short --branch
```

Expected: all verification commands exit zero and the worktree is clean.
