package world

import "kaya/internal/game"

type ScheduledEvent struct {
	TriggerAtSeconds int
	Event            game.WorldEvent
}

func (s *State) ScheduleEvent(afterSeconds int, event game.WorldEvent) {
	if s == nil {
		return
	}
	if afterSeconds < 0 {
		afterSeconds = 0
	}
	s.ScheduledEvents = append(s.ScheduledEvents, ScheduledEvent{
		TriggerAtSeconds: s.NowSeconds + afterSeconds,
		Event:            event,
	})
}

func (s *State) Advance(seconds int) []game.WorldEvent {
	if s == nil || seconds <= 0 {
		return nil
	}

	s.NowSeconds += seconds

	var fired []game.WorldEvent
	remaining := s.ScheduledEvents[:0]
	for _, scheduled := range s.ScheduledEvents {
		if scheduled.TriggerAtSeconds <= s.NowSeconds {
			fired = append(fired, scheduled.Event)
			continue
		}
		remaining = append(remaining, scheduled)
	}
	s.ScheduledEvents = remaining

	return fired
}
