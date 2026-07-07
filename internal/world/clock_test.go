package world

import (
	"testing"

	"kaya/internal/game"
)

func TestAdvanceMovesClock(t *testing.T) {
	state := NewState("room")

	state.Advance(12)

	if state.NowSeconds != 12 {
		t.Fatalf("NowSeconds = %d, want 12", state.NowSeconds)
	}
}

func TestAdvanceFiresScheduledEvents(t *testing.T) {
	state := NewState("room")
	state.ScheduleEvent(10, game.WorldEvent{
		Type:        game.EventSound,
		Description: "A pipe knocks behind the wall.",
		Danger:      game.DangerLow,
	})

	got := state.Advance(9)
	if len(got) != 0 {
		t.Fatalf("events after 9 seconds = %d, want 0", len(got))
	}

	got = state.Advance(1)
	if len(got) != 1 {
		t.Fatalf("events after 10 seconds = %d, want 1", len(got))
	}
	if got[0].Description != "A pipe knocks behind the wall." {
		t.Fatalf("event description = %q", got[0].Description)
	}
	if len(state.ScheduledEvents) != 0 {
		t.Fatalf("scheduled events remaining = %d, want 0", len(state.ScheduledEvents))
	}
}
