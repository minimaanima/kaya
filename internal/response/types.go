package response

import "kaya/internal/game"

// ResponseSentence preserves rendered sentence order and its cited engine facts.
type ResponseSentence struct {
	Text    string
	FactIDs []game.FactID
}

// Response is a rendered Kaya reply and its fallback metadata.
type Response struct {
	Text                    string
	Sentences               []ResponseSentence
	UsedFallback            bool
	FallbackReason          string
	UsedFactIDs             []game.FactID
	RepairAttempted         bool
	RepairSucceeded         bool
	InitialValidationReason string
	RepairValidationReason  string
	RepairGenerationError   string
}
