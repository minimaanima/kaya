package playtest

import (
	"context"
	"fmt"
	"strings"

	"kaya/internal/game"
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
	parser             session.Parser
	composer           session.Composer
	session            Session
	objectiveCompleted bool
}

func NewRunner(def rungen.Definition, run rungen.GeneratedRun, parser session.Parser, composer session.Composer) *Runner {
	return &Runner{
		definition: def,
		run:        run,
		parser:     parser,
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
}

func (r *Runner) Step(ctx context.Context, message string) (Step, error) {
	if r == nil || r.run.State == nil {
		return Step{}, fmt.Errorf("playtest runner has no world state")
	}

	step := Step{
		Number: len(r.session.Steps) + 1,
		Player: message,
		Before: Capture(r.run.State),
	}
	processed, err := session.ProcessTurn(ctx, message, r.run.State, r.parser, r.composer)
	if err != nil {
		return Step{}, err
	}
	step.Turn = cloneProcessedTurn(processed)
	step.After = Capture(r.run.State)
	step.ObjectiveEmitted = !r.objectiveCompleted && step.Before.CurrentRoom != r.definition.WinRoom && step.After.CurrentRoom == r.definition.WinRoom
	if step.ObjectiveEmitted {
		r.objectiveCompleted = true
		r.session.ObjectiveEmissions++
	}

	step.Violations = append(step.Violations, CheckState(r.run.State)...)
	step.Violations = append(step.Violations, CheckTransition(r.definition, step)...)
	if r.session.ObjectiveEmissions > 1 {
		step.Violations = append(step.Violations, Violation{Code: "objective_emitted_multiple_times", Detail: "objective emitted more than once"})
	}
	step.Violations = sortViolations(step.Violations)
	r.session.Steps = append(r.session.Steps, cloneStep(step))
	if len(step.Violations) > 0 {
		return step, fmt.Errorf("playtest invariant violation: %s", violationCodes(step.Violations))
	}
	return step, nil
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
	cloned.ObjectItems = cloneObjectItems(value.ObjectItems)
	cloned.ObjectRevealedItems = cloneObjectItems(value.ObjectRevealedItems)
	cloned.DoorStates = make(map[game.DoorID]world.DoorState, len(value.DoorStates))
	for doorID, state := range value.DoorStates {
		cloned.DoorStates[doorID] = state
	}
	cloned.RemainingEventTimes = append([]int(nil), value.RemainingEventTimes...)
	return cloned
}

func cloneObjectItems(value map[game.ObjectID][]game.ItemID) map[game.ObjectID][]game.ItemID {
	cloned := make(map[game.ObjectID][]game.ItemID, len(value))
	for objectID, itemIDs := range value {
		cloned[objectID] = append([]game.ItemID(nil), itemIDs...)
	}
	return cloned
}

func cloneProcessedTurn(value session.ProcessedTurn) session.ProcessedTurn {
	cloned := value
	cloned.Plan = clonePlan(value.Plan)
	cloned.Provenance.RawPlan = clonePlan(value.Provenance.RawPlan)
	cloned.Result = cloneResult(value.Result)
	cloned.Response = cloneResponse(value.Response)
	return cloned
}

func clonePlan(value intent.TurnPlan) intent.TurnPlan {
	cloned := value
	cloned.Actions = append([]intent.PlannedAction(nil), value.Actions...)
	for index := range cloned.Actions {
		cloned.Actions[index].Intent.Modifiers = append([]string(nil), value.Actions[index].Intent.Modifiers...)
	}
	cloned.Questions = append([]intent.FactQuestion(nil), value.Questions...)
	return cloned
}

func cloneResult(value turn.Result) turn.Result {
	cloned := value
	cloned.Outcomes = append([]turn.ActionOutcome(nil), value.Outcomes...)
	for index := range cloned.Outcomes {
		cloned.Outcomes[index].Intent.Modifiers = append([]string(nil), value.Outcomes[index].Intent.Modifiers...)
		cloned.Outcomes[index].Result.VisibleFacts = append([]game.Fact(nil), value.Outcomes[index].Result.VisibleFacts...)
		cloned.Outcomes[index].Result.Events = append([]game.WorldEvent(nil), value.Outcomes[index].Result.Events...)
	}
	cloned.QuestionFacts = append([]game.Fact(nil), value.QuestionFacts...)
	return cloned
}

func cloneResponse(value response.Response) response.Response {
	cloned := value
	cloned.UsedFactIDs = append([]game.FactID(nil), value.UsedFactIDs...)
	return cloned
}

func violationCodes(violations []Violation) string {
	codes := make([]string, 0, len(violations))
	for _, violation := range violations {
		codes = append(codes, violation.Code)
	}
	return strings.Join(codes, ",")
}
