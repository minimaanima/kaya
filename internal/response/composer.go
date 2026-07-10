package response

import (
	"context"
	"encoding/json"
	"strings"

	"kaya/internal/game"
	"kaya/internal/turn"
)

type StructuredGenerator interface {
	GenerateJSON(context.Context, string, string, any) (string, error)
}

type Composer struct {
	generator StructuredGenerator
	fallback  Fallback
}

func NewComposer(generator StructuredGenerator) Composer {
	return Composer{generator: generator}
}

func (c Composer) Compose(ctx context.Context, bundle turn.FactBundle) Response {
	fallback := c.fallback.Render(bundle)
	if c.generator == nil {
		return Response{Text: fallback, UsedFallback: true, FallbackReason: "generator_unavailable"}
	}
	payload, err := json.Marshal(responseInput(bundle))
	if err != nil {
		return Response{Text: fallback, UsedFallback: true, FallbackReason: "encode_input"}
	}
	raw, err := c.generator.GenerateJSON(ctx, SystemPrompt, string(payload), ResponseSchema)
	if err != nil {
		return Response{Text: fallback, UsedFallback: true, FallbackReason: "generate_failed"}
	}
	draft, reason := validateDraft(raw, bundle)
	if reason != "" {
		return Response{Text: fallback, UsedFallback: true, FallbackReason: reason}
	}
	parts := make([]string, len(draft.Sentences))
	used := make([]game.FactID, 0)
	seen := make(map[game.FactID]bool)
	for i, sentence := range draft.Sentences {
		parts[i] = strings.TrimSpace(sentence.Text)
		for _, id := range sentence.FactIDs {
			if !seen[id] {
				seen[id] = true
				used = append(used, id)
			}
		}
	}
	return Response{Text: strings.Join(parts, " "), UsedFactIDs: used}
}
