package intent

type Action string

const (
	ActionUnknown   Action = "unknown"
	ActionMove      Action = "move"
	ActionInspect   Action = "inspect"
	ActionSearch    Action = "search"
	ActionTakeItem  Action = "take_item"
	ActionUseItem   Action = "use_item"
	ActionTalk      Action = "talk"
	ActionWait      Action = "wait"
	ActionHide      Action = "hide"
	ActionListen    Action = "listen"
	ActionThrow     Action = "throw"
	ActionForceOpen Action = "force_open"
	ActionTurnOn    Action = "turn_on"
	ActionTurnOff   Action = "turn_off"
)

type Intent struct {
	Action                Action   `json:"action"`
	Target                string   `json:"target"`
	Item                  string   `json:"item"`
	Direction             string   `json:"direction"`
	Modifiers             []string `json:"modifiers"`
	Confidence            float64  `json:"confidence"`
	RawText               string   `json:"rawText"`
	NeedsClarification    bool     `json:"needsClarification"`
	ClarificationQuestion string   `json:"clarificationQuestion"`
}

func (i Intent) IsConfident(minimum float64) bool {
	return i.Confidence >= minimum
}

func (a Action) Valid() bool {
	switch a {
	case ActionUnknown,
		ActionMove,
		ActionInspect,
		ActionSearch,
		ActionTakeItem,
		ActionUseItem,
		ActionTalk,
		ActionWait,
		ActionHide,
		ActionListen,
		ActionThrow,
		ActionForceOpen,
		ActionTurnOn,
		ActionTurnOff:
		return true
	default:
		return false
	}
}
