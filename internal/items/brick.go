package items

import "kaya/internal/game"

type Brick struct {
	ItemID      game.ItemID
	DisplayName string
	Details     string
}

func (b Brick) ID() game.ItemID {
	return b.ItemID
}

func (b Brick) Name() string {
	return b.DisplayName
}

func (b Brick) Description() string {
	return b.Details
}

func (b Brick) Kind() ItemKind {
	return KindThrowable
}

func (b Brick) CanUse(ctx game.ActionContext) bool {
	return ctx.TargetObject != "" || ctx.TargetDoor != ""
}

func (b Brick) Use(ctx game.ActionContext) game.ActionResult {
	if !b.CanUse(ctx) {
		return game.ActionResult{
			StartedAtSeconds: ctx.NowSeconds,
			DurationSeconds:  4,
			Outcome:          "nothing_to_throw_at",
			VisibleFacts: []game.Fact{{
				Text: "There is no useful target for the brick.",
			}},
			Danger: game.DangerNone,
		}
	}

	return game.ActionResult{
		StartedAtSeconds: ctx.NowSeconds,
		DurationSeconds:  6,
		Outcome:          "brick_thrown",
		VisibleFacts: []game.Fact{{
			Text: "The brick hits hard and makes a sharp sound.",
		}},
		Events: []game.WorldEvent{{
			Type:        game.EventSound,
			Description: "The impact echoes through nearby rooms.",
			Danger:      game.DangerModerate,
		}},
		StressDelta: 2,
		Danger:      game.DangerModerate,
	}
}
