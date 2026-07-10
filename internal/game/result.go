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
	ID       FactID   `json:"id"`
	Kind     FactKind `json:"kind"`
	Subject  string   `json:"subject"`
	Value    string   `json:"value"`
	Text     string   `json:"text"`
	Required bool     `json:"required"`
}

type WorldEvent struct {
	Type        WorldEventType
	Description string
	Danger      DangerLevel
}

type ActionResult struct {
	Status                ActionStatus
	TargetObjectIDs       []ObjectID
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
