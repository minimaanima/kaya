package items

import "kaya/internal/game"

type Key struct {
	ItemID      game.ItemID
	DisplayName string
	Details     string
	OpensDoor   game.DoorID
}

func (k Key) ID() game.ItemID {
	return k.ItemID
}

func (k Key) Name() string {
	return k.DisplayName
}

func (k Key) Description() string {
	return k.Details
}

func (k Key) Kind() ItemKind {
	return KindKey
}

func (k Key) CanUse(ctx game.ActionContext) bool {
	return ctx.TargetDoor != "" && ctx.TargetDoor == k.OpensDoor
}

func (k Key) Use(ctx game.ActionContext) game.ActionResult {
	if !k.CanUse(ctx) {
		return game.ActionResult{
			StartedAtSeconds: ctx.NowSeconds,
			DurationSeconds:  5,
			Outcome:          "key_does_not_fit",
			VisibleFacts: []game.Fact{{
				Text: "The key does not fit anything nearby.",
			}},
			StressDelta: 1,
			Danger:      game.DangerNone,
		}
	}

	return game.ActionResult{
		StartedAtSeconds: ctx.NowSeconds,
		DurationSeconds:  8,
		Outcome:          "door_unlocked",
		VisibleFacts: []game.Fact{{
			Text: "The key turns in the lock.",
		}},
		Events: []game.WorldEvent{{
			Type:        game.EventDoorChanged,
			Description: "A locked door was unlocked.",
			Danger:      game.DangerNone,
		}},
		Danger: game.DangerNone,
	}
}
