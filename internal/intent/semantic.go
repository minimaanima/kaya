package intent

type SemanticAction interface {
	ActionKind() Action
	SourceEvidence() string
	semanticAction()
}

type Reference struct {
	Mention  string
	Quantity TargetMode
}

type SemanticPlan struct {
	Actions               []SemanticAction
	Questions             []FactQuestion
	RawText               string
	NeedsClarification    bool
	ClarificationQuestion string
}

type MoveAction struct {
	Direction string
	Evidence  string
}

func (a MoveAction) ActionKind() Action     { return ActionMove }
func (a MoveAction) SourceEvidence() string { return a.Evidence }
func (MoveAction) semanticAction()          {}

type InspectAction struct {
	Target   Reference
	Evidence string
}

func (a InspectAction) ActionKind() Action     { return ActionInspect }
func (a InspectAction) SourceEvidence() string { return a.Evidence }
func (InspectAction) semanticAction()          {}

type SearchAction struct {
	Target   Reference
	Evidence string
}

func (a SearchAction) ActionKind() Action     { return ActionSearch }
func (a SearchAction) SourceEvidence() string { return a.Evidence }
func (SearchAction) semanticAction()          {}

type TakeAction struct {
	Target   Reference
	Evidence string
}

func (a TakeAction) ActionKind() Action     { return ActionTakeItem }
func (a TakeAction) SourceEvidence() string { return a.Evidence }
func (TakeAction) semanticAction()          {}

type UseAction struct {
	Item     Reference
	Target   Reference
	Evidence string
}

func (a UseAction) ActionKind() Action     { return ActionUseItem }
func (a UseAction) SourceEvidence() string { return a.Evidence }
func (UseAction) semanticAction()          {}

type ToggleAction struct {
	Item     Reference
	State    string
	Evidence string
}

func (a ToggleAction) ActionKind() Action {
	switch a.State {
	case "on":
		return ActionTurnOn
	case "off":
		return ActionTurnOff
	default:
		return ActionUnknown
	}
}
func (a ToggleAction) SourceEvidence() string { return a.Evidence }
func (ToggleAction) semanticAction()          {}

type WaitAction struct {
	Evidence string
}

func (a WaitAction) ActionKind() Action     { return ActionWait }
func (a WaitAction) SourceEvidence() string { return a.Evidence }
func (WaitAction) semanticAction()          {}

type TalkAction struct {
	Target   Reference
	Evidence string
}

func (a TalkAction) ActionKind() Action     { return ActionTalk }
func (a TalkAction) SourceEvidence() string { return a.Evidence }
func (TalkAction) semanticAction()          {}

type ListenAction struct {
	Target   Reference
	Evidence string
}

func (a ListenAction) ActionKind() Action     { return ActionListen }
func (a ListenAction) SourceEvidence() string { return a.Evidence }
func (ListenAction) semanticAction()          {}

type ExploreAction struct {
	Target   Reference
	Evidence string
}

func (a ExploreAction) ActionKind() Action     { return ActionExplore }
func (a ExploreAction) SourceEvidence() string { return a.Evidence }
func (ExploreAction) semanticAction()          {}
