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

type PerceivedObject struct {
	ID      ObjectID `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

type PerceivedExit struct {
	Direction string `json:"direction"`
}

type PerceivedItem struct {
	ID      ItemID   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

type ReferentGroup struct {
	ObjectIDs []ObjectID `json:"objectIds,omitempty"`
	ItemIDs   []ItemID   `json:"itemIds,omitempty"`
}

type PerceptionSnapshot struct {
	RoomName        string            `json:"roomName"`
	HasUsefulLight  bool              `json:"hasUsefulLight"`
	VisibleObjects  []PerceivedObject `json:"visibleObjects"`
	KnownExits      []PerceivedExit   `json:"knownExits"`
	Inventory       []PerceivedItem   `json:"inventory"`
	RecentReferents []ReferentGroup   `json:"recentReferents"`
}
