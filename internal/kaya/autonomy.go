package kaya

import "kaya/internal/game"

type AutonomyDecision struct {
	Allowed           bool
	NeedsConfirmation bool
	Reason            string
}

func DefaultState() State {
	return State{
		Trust: 50,
	}
}

func (s State) CanAttempt(danger game.DangerLevel) AutonomyDecision {
	switch danger {
	case game.DangerNone, game.DangerLow:
		return AutonomyDecision{Allowed: true}
	}

	willingness := s.Willingness()
	refusalThreshold, confirmationThreshold := thresholdsForDanger(danger)
	if willingness <= refusalThreshold {
		return AutonomyDecision{
			Allowed: false,
			Reason:  "I cannot make myself do that right now.",
		}
	}
	if willingness <= confirmationThreshold {
		return AutonomyDecision{
			Allowed:           true,
			NeedsConfirmation: true,
			Reason:            "That feels dangerous. Are you sure?",
		}
	}
	return AutonomyDecision{Allowed: true}
}

func (s State) Apply(result game.ActionResult) State {
	s.Stress = clampScore(s.Stress + result.StressDelta)
	s.Trust = clampScore(s.Trust + result.TrustDelta)
	s.Fear = clampScore(s.Fear + result.FearDelta)
	s.Pain = clampScore(s.Pain + result.PainDelta)
	s.Exhaustion = clampScore(s.Exhaustion + result.ExhaustionDelta)

	s = s.applyDanger(result.Danger)
	for _, event := range result.Events {
		s = s.applyDanger(event.Danger)
	}

	if s.Pain > 0 {
		s.Injured = true
	}
	s.HasDoubt = s.NeedsReassurance()
	return s
}

func (s State) NeedsReassurance() bool {
	decision := s.CanAttempt(game.DangerHigh)
	return decision.NeedsConfirmation || !decision.Allowed
}

func (s State) applyDanger(danger game.DangerLevel) State {
	stress, fear := dangerDeltas(danger)
	s.Stress = clampScore(s.Stress + stress)
	s.Fear = clampScore(s.Fear + fear)
	return s
}

func thresholdsForDanger(danger game.DangerLevel) (int, int) {
	switch danger {
	case game.DangerModerate:
		return -70, -20
	case game.DangerHigh:
		return -50, 20
	case game.DangerLethal:
		return -10, 50
	default:
		return -1000, -1000
	}
}

func dangerDeltas(danger game.DangerLevel) (int, int) {
	switch danger {
	case game.DangerLow:
		return 3, 2
	case game.DangerModerate:
		return 10, 8
	case game.DangerHigh:
		return 20, 15
	case game.DangerLethal:
		return 35, 25
	default:
		return 0, 0
	}
}

func clampScore(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
