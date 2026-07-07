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
	Action                Action
	Target                string
	Item                  string
	Direction             string
	Modifiers             []string
	Confidence            float64
	RawText               string
	NeedsClarification    bool
	ClarificationQuestion string
}

func (i Intent) IsConfident(minimum float64) bool {
	return i.Confidence >= minimum
}
