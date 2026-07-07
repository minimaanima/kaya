package items

import "kaya/internal/game"

type Flashlight struct {
	ItemID         game.ItemID
	DisplayName    string
	Details        string
	BatterySeconds int
	On             bool
}

func (f Flashlight) ID() game.ItemID {
	return f.ItemID
}

func (f Flashlight) Name() string {
	return f.DisplayName
}

func (f Flashlight) Description() string {
	return f.Details
}

func (f Flashlight) Kind() ItemKind {
	return KindFlashlight
}

func (f Flashlight) CanUse(ctx game.ActionContext) bool {
	return f.BatterySeconds > 0
}

func (f Flashlight) Use(ctx game.ActionContext) game.ActionResult {
	if !f.CanUse(ctx) {
		return game.ActionResult{
			StartedAtSeconds: ctx.NowSeconds,
			DurationSeconds:  2,
			Outcome:          "flashlight_dead",
			VisibleFacts: []game.Fact{{
				Text: "The flashlight does not turn on.",
			}},
			StressDelta: 3,
			Danger:      game.DangerLow,
		}
	}

	return game.ActionResult{
		StartedAtSeconds: ctx.NowSeconds,
		DurationSeconds:  3,
		Outcome:          "flashlight_used",
		VisibleFacts: []game.Fact{{
			Text: "The flashlight cuts through the darkness and reveals nearby details.",
		}},
		Danger: game.DangerLow,
	}
}
