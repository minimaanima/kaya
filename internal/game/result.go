package game

type DangerLevel string

const (
	DangerNone     DangerLevel = "none"
	DangerLow      DangerLevel = "low"
	DangerModerate DangerLevel = "moderate"
	DangerHigh     DangerLevel = "high"
	DangerLethal   DangerLevel = "lethal"
)

type WorldEventType string

const (
	EventSound        WorldEventType = "sound"
	EventLightChange  WorldEventType = "light_change"
	EventHazardChange WorldEventType = "hazard_change"
	EventCreatureMove WorldEventType = "creature_move"
	EventItemRevealed WorldEventType = "item_revealed"
	EventDoorChanged  WorldEventType = "door_changed"
)

type Fact struct {
	Text string
}

type WorldEvent struct {
	Type        WorldEventType
	Description string
	Danger      DangerLevel
}

type ActionResult struct {
	StartedAtSeconds      int
	DurationSeconds       int
	Outcome               string
	VisibleFacts          []Fact
	Events                []WorldEvent
	StressDelta           int
	TrustDelta            int
	FearDelta             int
	PainDelta             int
	ExhaustionDelta       int
	Danger                DangerLevel
	NeedsClarification    bool
	ClarificationQuestion string
}
