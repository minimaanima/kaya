package playtest

import (
	"kaya/internal/game"
	"kaya/internal/kaya"
	"kaya/internal/rungen"
	"kaya/internal/session"
	"kaya/internal/world"
)

type Snapshot struct {
	CurrentRoom, PreviousRoom game.RoomID
	Time                      int
	Inventory, Discovered     []game.ItemID
	ObjectItems               map[game.ObjectID][]game.ItemID
	ObjectRevealedItems       map[game.ObjectID][]game.ItemID
	DoorStates                map[game.DoorID]world.DoorState
	RemainingEventTimes       []int
	ActiveLight               bool
	Kaya                      kaya.State
}

type Step struct {
	Number           int
	Player           string
	Before           Snapshot
	Turn             session.ProcessedTurn
	After            Snapshot
	ObjectiveEmitted bool
	Violations       []Violation
}

type Session struct {
	ScenarioID                        string
	ScenarioVersion, GeneratorVersion int
	Seed                              int64
	Placements                        []rungen.Placement
	Steps                             []Step
	ObjectiveEmissions                int
}

type Violation struct {
	Code, Detail string
}
