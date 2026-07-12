package response

import "kaya/internal/game"

// Response is a rendered Kaya reply and its fallback metadata.
type Response struct {
	Text                    string
	UsedFallback            bool
	FallbackReason          string
	UsedFactIDs             []game.FactID
	RepairAttempted         bool
	RepairSucceeded         bool
	InitialValidationReason string
	RepairValidationReason  string
	RepairGenerationError   string
}
