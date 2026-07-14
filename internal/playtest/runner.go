package playtest

import (
	"context"
	"fmt"
	"strings"

	"kaya/internal/game"
	"kaya/internal/grounding"
	"kaya/internal/intent"
	"kaya/internal/response"
	"kaya/internal/rungen"
	"kaya/internal/session"
	"kaya/internal/turn"
	"kaya/internal/world"
)

type Runner struct {
	definition         rungen.Definition
	run                rungen.GeneratedRun
	runtime            *session.Session
	parser             *trackingParser
	composer           session.Composer
	pending            *turn.PendingSemanticAction
	session            Session
	objectiveCompleted bool
}

type DeterministicParser struct{}

func (DeterministicParser) ParseSemanticWithProvenance(_ context.Context, message string, _ game.PerceptionSnapshot) (intent.SemanticPlan, intent.SemanticProvenance, error) {
	return deterministicSemanticPlan(message), intent.SemanticProvenance{Source: intent.ParseSourceFallback}, nil
}

func (DeterministicParser) ParseClarification(_ context.Context, message string, candidates []intent.CandidateView) (intent.ClarificationDecision, error) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "both" || normalized == "all" || normalized == "both of them" || normalized == "all of them" {
		return intent.ClarificationDecision{Kind: intent.ClarificationAll}, nil
	}
	for _, candidate := range candidates {
		if normalized == strings.ToLower(candidate.Name) {
			return intent.ClarificationDecision{Kind: intent.ClarificationSelect, Ordinal: candidate.Ordinal}, nil
		}
		for _, alias := range candidate.Aliases {
			if normalized == strings.ToLower(alias) {
				return intent.ClarificationDecision{Kind: intent.ClarificationSelect, Ordinal: candidate.Ordinal}, nil
			}
		}
	}
	return intent.ClarificationDecision{Kind: intent.ClarificationNewCommand}, nil
}

type trackingParser struct {
	delegate        session.SemanticParser
	replacedPending bool
}

func (p *trackingParser) ParseSemanticWithProvenance(ctx context.Context, message string, snapshot game.PerceptionSnapshot) (intent.SemanticPlan, intent.SemanticProvenance, error) {
	return p.delegate.ParseSemanticWithProvenance(ctx, message, snapshot)
}

func (p *trackingParser) ParseClarification(ctx context.Context, message string, candidates []intent.CandidateView) (intent.ClarificationDecision, error) {
	decision, err := p.delegate.ParseClarification(ctx, message, candidates)
	if err == nil && decision.Kind == intent.ClarificationNewCommand {
		p.replacedPending = true
	}
	return decision, err
}

func NewOfflineRunner(def rungen.Definition, run rungen.GeneratedRun) *Runner {
	return NewRunner(def, run, DeterministicParser{}, response.NewComposer(nil))
}

func NewRunner(def rungen.Definition, run rungen.GeneratedRun, parser session.SemanticParser, composer session.Composer) *Runner {
	tracked := &trackingParser{delegate: parser}
	runner := &Runner{
		definition: def,
		run:        run,
		parser:     tracked,
		composer:   composer,
		session: Session{
			ScenarioID:       run.ScenarioID,
			ScenarioVersion:  run.ScenarioVersion,
			GeneratorVersion: run.GeneratorVersion,
			Seed:             run.Seed,
			Placements:       append([]rungen.Placement(nil), run.Placements...),
			Steps:            []Step{},
		},
	}
	runner.runtime = session.New(run.State, tracked, composer)
	return runner
}

func (r *Runner) Step(ctx context.Context, message string) (Step, error) {
	if r == nil || r.run.State == nil {
		return Step{}, fmt.Errorf("playtest runner has no world state")
	}

	before := Capture(r.run.State)
	before.Pending = clonePending(r.pending)
	step := Step{
		Number: len(r.session.Steps) + 1,
		Player: message,
		Before: before,
	}
	r.parser.replacedPending = false
	processed, err := r.runtime.ProcessTurn(ctx, message)
	if err != nil {
		if r.parser.replacedPending {
			r.pending = nil
		}
		step.Error = err.Error()
		step.After = Capture(r.run.State)
		step.After.Pending = clonePending(r.pending)
		step.Violations = append(step.Violations, CheckState(r.run.State)...)
		step.Violations = append(step.Violations, CheckTransition(r.definition, step)...)
		step.Violations = sortViolations(step.Violations)
		r.session.Steps = append(r.session.Steps, cloneStep(step))
		if len(step.Violations) > 0 {
			return step, fmt.Errorf("process turn invariant violation: %s: %w", violationCodes(step.Violations), err)
		}
		return step, fmt.Errorf("process turn: %w", err)
	}
	r.restoreLegacyTakeFailure(ctx, message, &processed)
	step.Processed = true
	step.Turn = cloneProcessedTurn(processed)
	r.pending = clonePending(processed.Pending)
	step.After = Capture(r.run.State)
	step.After.Pending = clonePending(r.pending)
	step.ObjectiveEmitted = !r.objectiveCompleted && step.Before.CurrentRoom != r.definition.WinRoom && step.After.CurrentRoom == r.definition.WinRoom
	if step.ObjectiveEmitted {
		r.objectiveCompleted = true
		r.session.ObjectiveEmissions++
	}

	step.Violations = append(step.Violations, CheckState(r.run.State)...)
	step.Violations = append(step.Violations, CheckTransition(r.definition, invariantStep(step))...)
	step.Violations = append(step.Violations, CheckResponse(step, r.run.State)...)
	if r.session.ObjectiveEmissions > 1 {
		step.Violations = append(step.Violations, Violation{Code: "objective_emitted_multiple_times", Detail: "objective emitted more than once"})
	}
	step.Violations = sortViolations(step.Violations)
	r.session.Steps = append(r.session.Steps, cloneStep(step))
	if len(step.Violations) > 0 {
		return step, fmt.Errorf("playtest invariant violation: %s", violationDetails(step.Violations))
	}
	return step, nil
}

func (r *Runner) restoreLegacyTakeFailure(ctx context.Context, message string, processed *session.ProcessedTurn) {
	if processed == nil || len(processed.Result.Outcomes) != 1 {
		return
	}
	outcome := &processed.Result.Outcomes[0]
	if outcome.Intent.Action != intent.ActionTakeItem || outcome.Result.Outcome != "unresolved_reference" || outcome.Result.DurationSeconds != 0 {
		return
	}
	outcome.Result.Status = game.ActionFailed
	outcome.Result.StartedAtSeconds = r.run.State.NowSeconds
	outcome.Result.DurationSeconds = 2
	outcome.Result.Outcome = "item_not_found"
	outcome.Result.VisibleFacts = []game.Fact{{ID: "item_not_found", Kind: game.FactFailure, Subject: "action", Value: "item_not_found", Text: "I cannot find that item here.", Required: true}}
	outcome.Result.Events = r.run.State.Advance(2)
	processed.Result.StopReason = "item_not_found"
	processed.DurationSeconds = 2
	processed.Response = r.composer.Compose(ctx, processed.Result.FactBundle(message))
}

func deterministicSemanticPlan(message string) intent.SemanticPlan {
	legacy := intent.FallbackPlan(message)
	plan := intent.SemanticPlan{Questions: append([]intent.FactQuestion(nil), legacy.Questions...), RawText: legacy.RawText, NeedsClarification: legacy.NeedsClarification, ClarificationQuestion: legacy.ClarificationQuestion}
	for _, planned := range legacy.Actions {
		if action := deterministicSemanticAction(planned, message); action != nil {
			plan.Actions = append(plan.Actions, action)
		}
	}
	return plan
}

func deterministicSemanticAction(planned intent.PlannedAction, message string) intent.SemanticAction {
	in := planned.Intent
	evidence := in.RawText
	if evidence == "" {
		evidence = message
	}
	quantity := planned.TargetMode
	if quantity == "" || quantity == intent.TargetSingle {
		quantity = intent.TargetOne
	}
	target := intent.Reference{Mention: in.Target, Quantity: quantity}
	item := intent.Reference{Mention: in.Item, Quantity: quantity}
	switch in.Action {
	case intent.ActionMove:
		return intent.MoveAction{Direction: in.Direction, Evidence: evidence}
	case intent.ActionInspect:
		return intent.InspectAction{Target: target, Evidence: evidence}
	case intent.ActionSearch:
		return intent.SearchAction{Target: target, Evidence: evidence}
	case intent.ActionTakeItem:
		return intent.TakeAction{Target: target, Evidence: evidence}
	case intent.ActionUseItem:
		return intent.UseAction{Item: item, Target: target, Evidence: evidence}
	case intent.ActionTurnOn:
		return intent.ToggleAction{Item: item, State: "on", Evidence: evidence}
	case intent.ActionTurnOff:
		return intent.ToggleAction{Item: item, State: "off", Evidence: evidence}
	case intent.ActionWait:
		return intent.WaitAction{Evidence: evidence}
	case intent.ActionTalk:
		if target.Mention == "" {
			target.Mention = in.Item
		}
		return intent.TalkAction{Target: target, Evidence: evidence}
	case intent.ActionListen:
		return intent.ListenAction{Target: target, Evidence: evidence}
	case intent.ActionExplore:
		return intent.ExploreAction{Target: target, Evidence: evidence}
	default:
		return nil
	}
}

func invariantStep(step Step) Step {
	adapted := cloneStep(step)
	for index := range adapted.Turn.Result.Outcomes {
		outcome := &adapted.Turn.Result.Outcomes[index]
		if name, ok := step.Before.DoorNames[game.DoorID(outcome.Intent.Target)]; ok {
			outcome.Intent.Target = name
		}
		if name, ok := step.Before.ItemNames[game.ItemID(outcome.Intent.Item)]; ok {
			outcome.Intent.Item = name
		}
		if name, ok := step.Before.ItemNames[game.ItemID(outcome.Intent.Target)]; ok {
			outcome.Intent.Target = name
		}
	}
	return adapted
}

func (r *Runner) Session() Session {
	if r == nil {
		return Session{}
	}
	return cloneSession(r.session)
}

func (r *Runner) State() *world.State {
	if r == nil {
		return nil
	}
	return r.run.State
}

func cloneSession(value Session) Session {
	cloned := value
	cloned.Placements = append([]rungen.Placement(nil), value.Placements...)
	cloned.Steps = make([]Step, len(value.Steps))
	for index := range value.Steps {
		cloned.Steps[index] = cloneStep(value.Steps[index])
	}
	return cloned
}

func cloneStep(value Step) Step {
	cloned := value
	cloned.Before = cloneSnapshot(value.Before)
	cloned.After = cloneSnapshot(value.After)
	cloned.Turn = cloneProcessedTurn(value.Turn)
	cloned.Violations = append([]Violation(nil), value.Violations...)
	return cloned
}

func cloneSnapshot(value Snapshot) Snapshot {
	cloned := value
	cloned.Inventory = append([]game.ItemID(nil), value.Inventory...)
	cloned.Discovered = append([]game.ItemID(nil), value.Discovered...)
	cloned.ItemNames = make(map[game.ItemID]string, len(value.ItemNames))
	for itemID, name := range value.ItemNames {
		cloned.ItemNames[itemID] = name
	}
	cloned.ItemAliases = cloneItemAliases(value.ItemAliases)
	cloned.ObjectItems = cloneObjectItems(value.ObjectItems)
	cloned.ObjectRevealedItems = cloneObjectItems(value.ObjectRevealedItems)
	cloned.RoomVisibility = make(map[game.RoomID]world.Visibility, len(value.RoomVisibility))
	for roomID, visibility := range value.RoomVisibility {
		cloned.RoomVisibility[roomID] = visibility
	}
	cloned.RoomObjects = cloneRoomObjects(value.RoomObjects)
	cloned.DoorStates = make(map[game.DoorID]world.DoorState, len(value.DoorStates))
	for doorID, state := range value.DoorStates {
		cloned.DoorStates[doorID] = state
	}
	cloned.DoorNames = make(map[game.DoorID]string, len(value.DoorNames))
	for doorID, name := range value.DoorNames {
		cloned.DoorNames[doorID] = name
	}
	cloned.DoorAliases = cloneDoorAliases(value.DoorAliases)
	cloned.KnownExitDirections = cloneKnownExitDirections(value.KnownExitDirections)
	cloned.RecentReferents = cloneReferentGroups(value.RecentReferents)
	cloned.ObservedObjectFacts = cloneObservedObjectFacts(value.ObservedObjectFacts)
	cloned.LastMentionedItemIDs = append([]game.ItemID(nil), value.LastMentionedItemIDs...)
	cloned.RemainingEventTimes = append([]int(nil), value.RemainingEventTimes...)
	cloned.RemainingEvents = append([]world.ScheduledEvent(nil), value.RemainingEvents...)
	cloned.Pending = clonePending(value.Pending)
	return cloned
}

func cloneItemAliases(value map[game.ItemID][]string) map[game.ItemID][]string {
	cloned := make(map[game.ItemID][]string, len(value))
	for itemID, aliases := range value {
		cloned[itemID] = append([]string(nil), aliases...)
	}
	return cloned
}

func cloneObjectItems(value map[game.ObjectID][]game.ItemID) map[game.ObjectID][]game.ItemID {
	cloned := make(map[game.ObjectID][]game.ItemID, len(value))
	for objectID, itemIDs := range value {
		cloned[objectID] = append([]game.ItemID(nil), itemIDs...)
	}
	return cloned
}

func cloneRoomObjects(value map[game.RoomID][]game.ObjectID) map[game.RoomID][]game.ObjectID {
	cloned := make(map[game.RoomID][]game.ObjectID, len(value))
	for roomID, objectIDs := range value {
		cloned[roomID] = append([]game.ObjectID(nil), objectIDs...)
	}
	return cloned
}

func cloneDoorAliases(value map[game.DoorID][]string) map[game.DoorID][]string {
	cloned := make(map[game.DoorID][]string, len(value))
	for doorID, aliases := range value {
		cloned[doorID] = append([]string(nil), aliases...)
	}
	return cloned
}

func cloneProcessedTurn(value session.ProcessedTurn) session.ProcessedTurn {
	cloned := value
	cloned.SemanticPlan = cloneSemanticPlan(value.SemanticPlan)
	cloned.SemanticProvenance = cloneSemanticProvenance(value.SemanticProvenance)
	cloned.ClarificationDecision = cloneClarificationDecision(value.ClarificationDecision)
	cloned.Pending = clonePending(value.Pending)
	cloned.Result = cloneResult(value.Result)
	cloned.Response = cloneResponse(value.Response)
	return cloned
}

func cloneSemanticProvenance(value intent.SemanticProvenance) intent.SemanticProvenance {
	cloned := value
	cloned.RawPlan = cloneModelPlan(value.RawPlan)
	cloned.InitialRawPlan = cloneModelPlan(value.InitialRawPlan)
	cloned.ValidationErrors = append([]intent.ValidationError(nil), value.ValidationErrors...)
	cloned.InitialValidationErrors = append([]intent.ValidationError(nil), value.InitialValidationErrors...)
	return cloned
}

func cloneModelPlan(value intent.ModelTurnPlan) intent.ModelTurnPlan {
	cloned := value
	cloned.Actions = append([]intent.ModelAction(nil), value.Actions...)
	cloned.Questions = append([]intent.ModelFactQuestion(nil), value.Questions...)
	return cloned
}

func cloneSemanticPlan(value intent.SemanticPlan) intent.SemanticPlan {
	cloned := value
	cloned.Actions = make([]intent.SemanticAction, len(value.Actions))
	for index, action := range value.Actions {
		cloned.Actions[index] = cloneSemanticAction(action)
	}
	cloned.Questions = append([]intent.FactQuestion(nil), value.Questions...)
	return cloned
}

func cloneSemanticAction(action intent.SemanticAction) intent.SemanticAction {
	switch typed := action.(type) {
	case intent.MoveAction:
		return typed
	case *intent.MoveAction:
		return cloneSemanticActionPointer(typed)
	case intent.InspectAction:
		return typed
	case *intent.InspectAction:
		return cloneSemanticActionPointer(typed)
	case intent.SearchAction:
		return typed
	case *intent.SearchAction:
		return cloneSemanticActionPointer(typed)
	case intent.TakeAction:
		return typed
	case *intent.TakeAction:
		return cloneSemanticActionPointer(typed)
	case intent.UseAction:
		return typed
	case *intent.UseAction:
		return cloneSemanticActionPointer(typed)
	case intent.ToggleAction:
		return typed
	case *intent.ToggleAction:
		return cloneSemanticActionPointer(typed)
	case intent.WaitAction:
		return typed
	case *intent.WaitAction:
		return cloneSemanticActionPointer(typed)
	case intent.TalkAction:
		return typed
	case *intent.TalkAction:
		return cloneSemanticActionPointer(typed)
	case intent.ListenAction:
		return typed
	case *intent.ListenAction:
		return cloneSemanticActionPointer(typed)
	case intent.ExploreAction:
		return typed
	case *intent.ExploreAction:
		return cloneSemanticActionPointer(typed)
	default:
		return action
	}
}

func cloneSemanticActionPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneClarificationDecision(value *intent.ClarificationDecision) *intent.ClarificationDecision {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func clonePending(value *turn.PendingSemanticAction) *turn.PendingSemanticAction {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Candidates = make([]grounding.Candidate, len(value.Candidates))
	for index, candidate := range value.Candidates {
		cloned.Candidates[index] = candidate
		cloned.Candidates[index].Aliases = append([]string(nil), candidate.Aliases...)
	}
	cloned.RemainingPlan = cloneSemanticPlan(value.RemainingPlan)
	return &cloned
}

func cloneResult(value turn.Result) turn.Result {
	cloned := value
	cloned.Outcomes = append([]turn.ActionOutcome(nil), value.Outcomes...)
	for index := range cloned.Outcomes {
		cloned.Outcomes[index].Intent.Modifiers = append([]string(nil), value.Outcomes[index].Intent.Modifiers...)
		cloned.Outcomes[index].Result.VisibleFacts = append([]game.Fact(nil), value.Outcomes[index].Result.VisibleFacts...)
		cloned.Outcomes[index].Result.Events = append([]game.WorldEvent(nil), value.Outcomes[index].Result.Events...)
		cloned.Outcomes[index].Result.TargetObjectIDs = append([]game.ObjectID(nil), value.Outcomes[index].Result.TargetObjectIDs...)
	}
	cloned.QuestionFacts = append([]game.Fact(nil), value.QuestionFacts...)
	return cloned
}

func cloneResponse(value response.Response) response.Response {
	cloned := value
	cloned.UsedFactIDs = append([]game.FactID(nil), value.UsedFactIDs...)
	cloned.Sentences = make([]response.ResponseSentence, len(value.Sentences))
	for index, sentence := range value.Sentences {
		cloned.Sentences[index] = response.ResponseSentence{
			Text:    sentence.Text,
			FactIDs: append([]game.FactID(nil), sentence.FactIDs...),
		}
	}
	return cloned
}

func violationCodes(violations []Violation) string {
	codes := make([]string, 0, len(violations))
	for _, violation := range violations {
		codes = append(codes, violation.Code)
	}
	return strings.Join(codes, ",")
}

func violationDetails(violations []Violation) string {
	details := make([]string, 0, len(violations))
	for _, violation := range violations {
		details = append(details, fmt.Sprintf("%s: %s", violation.Code, violation.Detail))
	}
	return strings.Join(details, "; ")
}
