package items

import "kaya/internal/game"

type ItemKind string

const (
	KindKey        ItemKind = "key"
	KindFlashlight ItemKind = "flashlight"
	KindThrowable  ItemKind = "throwable"
	KindMedical    ItemKind = "medical"
	KindDocument   ItemKind = "document"
	KindTool       ItemKind = "tool"
)

type Item interface {
	ID() game.ItemID
	Name() string
	Description() string
	Kind() ItemKind
	CanUse(ctx game.ActionContext) bool
	Use(ctx game.ActionContext) game.ActionResult
}
