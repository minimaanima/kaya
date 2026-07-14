package session

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"kaya/internal/game"
	"kaya/internal/grounding"
	"kaya/internal/intent"
	"kaya/internal/turn"
	"kaya/internal/world"
)

type SemanticParser interface {
	ParseSemanticWithProvenance(context.Context, string, game.PerceptionSnapshot) (intent.SemanticPlan, intent.SemanticProvenance, error)
	ParseClarification(context.Context, string, []intent.CandidateView) (intent.ClarificationDecision, error)
}

type Session struct {
	state    *world.State
	parser   SemanticParser
	composer Composer
	pending  *pendingClarification
}

type pendingClarification struct {
	plan         intent.SemanticPlan
	actionIndex  int
	role         grounding.Role
	candidateIDs []string
	candidates   []grounding.Candidate
}

func New(state *world.State, parser SemanticParser, composer Composer) *Session {
	return &Session{state: state, parser: parser, composer: composer}
}

func (s *Session) ProcessTurn(ctx context.Context, message string) (ProcessedTurn, error) {
	if s == nil || s.state == nil || s.parser == nil || s.composer == nil {
		return ProcessedTurn{}, fmt.Errorf("session dependencies must not be nil")
	}
	if s.pending != nil {
		return s.processClarification(ctx, message)
	}
	return s.processCommand(ctx, message)
}

func (s *Session) processCommand(ctx context.Context, message string) (ProcessedTurn, error) {
	snapshot, err := s.state.PerceptionSnapshot()
	if err != nil {
		return ProcessedTurn{}, fmt.Errorf("snapshot world: %w", err)
	}
	parseCtx, cancelParse := context.WithTimeout(ctx, 60*time.Second)
	plan, provenance, err := s.parser.ParseSemanticWithProvenance(parseCtx, message, snapshot)
	cancelParse()
	if err != nil {
		return ProcessedTurn{}, err
	}

	execution := turn.NewExecutor(s.state).ExecuteSemantic(plan, 0, nil)
	s.rememberPending(plan, execution.Pending)
	return s.finishSemanticTurn(ctx, message, plan, provenance, execution.Result, nil), nil
}

func (s *Session) processClarification(ctx context.Context, message string) (ProcessedTurn, error) {
	pending := s.pending
	views := candidateViews(pending.candidates)
	parseCtx, cancelParse := context.WithTimeout(ctx, 60*time.Second)
	decision, err := s.parser.ParseClarification(parseCtx, message, views)
	cancelParse()
	if err != nil {
		return ProcessedTurn{}, err
	}

	switch decision.Kind {
	case intent.ClarificationCancel:
		s.pending = nil
		result := turn.Result{Outcomes: []turn.ActionOutcome{}, QuestionFacts: []game.Fact{}, StopReason: "cancelled"}
		return s.finishSemanticTurn(ctx, message, pending.plan, intent.SemanticProvenance{}, result, &decision), nil
	case intent.ClarificationNewCommand:
		s.pending = nil
		processed, err := s.processCommand(ctx, message)
		if err != nil {
			return ProcessedTurn{}, err
		}
		processed.ClarificationDecision = &decision
		return processed, nil
	}

	selectedIDs, ok := pending.selectedCandidateIDs(decision)
	if !ok {
		result := turn.Result{
			Outcomes:              []turn.ActionOutcome{},
			QuestionFacts:         []game.Fact{},
			StopReason:            "clarification",
			ClarificationQuestion: pending.question(),
		}
		return s.finishSemanticTurn(ctx, message, pending.plan, intent.SemanticProvenance{}, result, &decision), nil
	}

	execution := turn.NewExecutor(s.state).ExecuteSemantic(pending.plan, pending.actionIndex, &grounding.Binding{
		Role:         pending.role,
		CandidateIDs: selectedIDs,
	})
	s.rememberPending(pending.plan, execution.Pending)
	return s.finishSemanticTurn(ctx, message, pending.plan, intent.SemanticProvenance{}, execution.Result, &decision), nil
}

func (s *Session) finishSemanticTurn(
	ctx context.Context,
	message string,
	plan intent.SemanticPlan,
	provenance intent.SemanticProvenance,
	result turn.Result,
	decision *intent.ClarificationDecision,
) ProcessedTurn {
	responseCtx, cancelResponse := context.WithTimeout(ctx, 60*time.Second)
	composed := s.composer.Compose(responseCtx, result.FactBundle(message))
	cancelResponse()
	return ProcessedTurn{
		SemanticPlan:          cloneSemanticPlan(plan),
		SemanticProvenance:    provenance,
		ClarificationDecision: cloneDecision(decision),
		Pending:               s.pendingView(),
		Result:                result,
		Response:              composed,
		DurationSeconds:       ResultDuration(result),
	}
}

func (s *Session) rememberPending(plan intent.SemanticPlan, pending *turn.PendingSemanticAction) {
	if pending == nil {
		s.pending = nil
		return
	}
	storedPlan := cloneSemanticPlan(plan)
	if pending.ActionIndex >= 0 && pending.ActionIndex <= len(storedPlan.Actions) {
		remaining := cloneSemanticPlan(pending.RemainingPlan)
		storedPlan.Actions = append(storedPlan.Actions[:pending.ActionIndex], remaining.Actions...)
		storedPlan.Questions = remaining.Questions
	}
	candidates := cloneGroundingCandidates(pending.Candidates)
	ids := make([]string, len(candidates))
	for index, candidate := range candidates {
		ids[index] = candidate.ID
	}
	s.pending = &pendingClarification{
		plan:         storedPlan,
		actionIndex:  pending.ActionIndex,
		role:         pending.Role,
		candidateIDs: ids,
		candidates:   candidates,
	}
}

func (s *Session) pendingView() *turn.PendingSemanticAction {
	if s == nil || s.pending == nil {
		return nil
	}
	pending := s.pending
	remaining := cloneSemanticPlan(pending.plan)
	if pending.actionIndex >= 0 && pending.actionIndex <= len(remaining.Actions) {
		remaining.Actions = append([]intent.SemanticAction(nil), remaining.Actions[pending.actionIndex:]...)
	}
	return &turn.PendingSemanticAction{
		ActionIndex:   pending.actionIndex,
		Role:          pending.role,
		Candidates:    cloneGroundingCandidates(pending.candidates),
		RemainingPlan: remaining,
	}
}

func (p *pendingClarification) selectedCandidateIDs(decision intent.ClarificationDecision) ([]string, bool) {
	if p == nil {
		return nil, false
	}
	if decision.Kind == intent.ClarificationAll {
		if len(p.candidateIDs) == 0 {
			return nil, false
		}
		return append([]string(nil), p.candidateIDs...), true
	}
	if decision.Kind != intent.ClarificationSelect && decision.Kind != intent.ClarificationConfirm {
		return nil, false
	}

	var selectedByOrdinal, selectedByMention string
	if decision.Ordinal > 0 {
		index := decision.Ordinal - 1
		if index < 0 || index >= len(p.candidates) {
			return nil, false
		}
		selectedByOrdinal = p.candidates[index].ID
	}
	if decision.Mention != "" {
		selectedByMention = exactCandidateID(decision.Mention, p.candidates)
		if selectedByMention == "" {
			return nil, false
		}
	}
	if selectedByOrdinal != "" && selectedByMention != "" && selectedByOrdinal != selectedByMention {
		return nil, false
	}
	selected := selectedByOrdinal
	if selected == "" {
		selected = selectedByMention
	}
	if selected == "" && decision.Kind == intent.ClarificationConfirm && len(p.candidates) == 1 {
		selected = p.candidates[0].ID
	}
	if selected == "" || !containsCandidateID(p.candidateIDs, selected) {
		return nil, false
	}
	return []string{selected}, true
}

func exactCandidateID(mention string, candidates []grounding.Candidate) string {
	target := normalizeCandidateText(mention)
	if target == "" {
		return ""
	}
	nameMatches := make([]string, 0, 1)
	for _, candidate := range candidates {
		if normalizeCandidateText(candidate.Name) == target {
			nameMatches = append(nameMatches, candidate.ID)
		}
	}
	if len(nameMatches) == 1 {
		return nameMatches[0]
	}
	if len(nameMatches) > 1 {
		return ""
	}
	aliasMatches := make([]string, 0, 1)
	for _, candidate := range candidates {
		for _, alias := range candidate.Aliases {
			if normalizeCandidateText(alias) == target {
				aliasMatches = append(aliasMatches, candidate.ID)
				break
			}
		}
	}
	if len(aliasMatches) == 1 {
		return aliasMatches[0]
	}
	return ""
}

func normalizeCandidateText(value string) string {
	words := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
	kept := words[:0]
	for _, word := range words {
		switch word {
		case "a", "an", "the":
		default:
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
}

func (p *pendingClarification) question() string {
	names := make([]string, 0, len(p.candidates))
	for _, candidate := range p.candidates {
		names = append(names, candidate.Name)
	}
	if len(names) == 0 {
		return "Which one do you mean?"
	}
	return "Which one do you mean: " + strings.Join(names, ", ") + "?"
}

func candidateViews(candidates []grounding.Candidate) []intent.CandidateView {
	views := make([]intent.CandidateView, len(candidates))
	for index, candidate := range candidates {
		views[index] = intent.CandidateView{
			Ordinal: index + 1,
			Name:    candidate.Name,
			Aliases: append([]string(nil), candidate.Aliases...),
		}
	}
	return views
}

func cloneSemanticPlan(plan intent.SemanticPlan) intent.SemanticPlan {
	actions := make([]intent.SemanticAction, len(plan.Actions))
	for index, action := range plan.Actions {
		actions[index] = cloneSemanticAction(action)
	}
	questions := make([]intent.FactQuestion, len(plan.Questions))
	copy(questions, plan.Questions)
	plan.Actions = actions
	plan.Questions = questions
	return plan
}

func cloneSemanticAction(action intent.SemanticAction) intent.SemanticAction {
	switch typed := action.(type) {
	case intent.MoveAction:
		return typed
	case *intent.MoveAction:
		if typed == nil {
			return (*intent.MoveAction)(nil)
		}
		cloned := *typed
		return &cloned
	case intent.InspectAction:
		typed.Target = cloneReference(typed.Target)
		return typed
	case *intent.InspectAction:
		if typed == nil {
			return (*intent.InspectAction)(nil)
		}
		cloned := *typed
		cloned.Target = cloneReference(typed.Target)
		return &cloned
	case intent.SearchAction:
		typed.Target = cloneReference(typed.Target)
		return typed
	case *intent.SearchAction:
		if typed == nil {
			return (*intent.SearchAction)(nil)
		}
		cloned := *typed
		cloned.Target = cloneReference(typed.Target)
		return &cloned
	case intent.TakeAction:
		typed.Target = cloneReference(typed.Target)
		return typed
	case *intent.TakeAction:
		if typed == nil {
			return (*intent.TakeAction)(nil)
		}
		cloned := *typed
		cloned.Target = cloneReference(typed.Target)
		return &cloned
	case intent.UseAction:
		typed.Item = cloneReference(typed.Item)
		typed.Target = cloneReference(typed.Target)
		return typed
	case *intent.UseAction:
		if typed == nil {
			return (*intent.UseAction)(nil)
		}
		cloned := *typed
		cloned.Item = cloneReference(typed.Item)
		cloned.Target = cloneReference(typed.Target)
		return &cloned
	case intent.ToggleAction:
		typed.Item = cloneReference(typed.Item)
		return typed
	case *intent.ToggleAction:
		if typed == nil {
			return (*intent.ToggleAction)(nil)
		}
		cloned := *typed
		cloned.Item = cloneReference(typed.Item)
		return &cloned
	case intent.WaitAction:
		return typed
	case *intent.WaitAction:
		if typed == nil {
			return (*intent.WaitAction)(nil)
		}
		cloned := *typed
		return &cloned
	case intent.TalkAction:
		typed.Target = cloneReference(typed.Target)
		return typed
	case *intent.TalkAction:
		if typed == nil {
			return (*intent.TalkAction)(nil)
		}
		cloned := *typed
		cloned.Target = cloneReference(typed.Target)
		return &cloned
	case intent.ListenAction:
		typed.Target = cloneReference(typed.Target)
		return typed
	case *intent.ListenAction:
		if typed == nil {
			return (*intent.ListenAction)(nil)
		}
		cloned := *typed
		cloned.Target = cloneReference(typed.Target)
		return &cloned
	case intent.ExploreAction:
		typed.Target = cloneReference(typed.Target)
		return typed
	case *intent.ExploreAction:
		if typed == nil {
			return (*intent.ExploreAction)(nil)
		}
		cloned := *typed
		cloned.Target = cloneReference(typed.Target)
		return &cloned
	default:
		return action
	}
}

func cloneReference(reference intent.Reference) intent.Reference {
	return intent.Reference{Mention: reference.Mention, Quantity: reference.Quantity}
}

func cloneGroundingCandidates(candidates []grounding.Candidate) []grounding.Candidate {
	cloned := make([]grounding.Candidate, len(candidates))
	for index, candidate := range candidates {
		candidate.Aliases = append([]string(nil), candidate.Aliases...)
		cloned[index] = candidate
	}
	return cloned
}

func cloneDecision(decision *intent.ClarificationDecision) *intent.ClarificationDecision {
	if decision == nil {
		return nil
	}
	cloned := *decision
	return &cloned
}

func containsCandidateID(ids []string, wanted string) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}
