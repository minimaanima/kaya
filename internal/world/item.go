package world

import "kaya/internal/game"

type Item struct {
	ID          game.ItemID
	Name        string
	Aliases     []string
	Description string
	Portable    bool
}
