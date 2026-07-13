package response

import (
	"context"
	"encoding/json"
	"fmt"
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
	fallback := responseFromSentences(c.fallback.Sentences(bundle))
	fallback.UsedFallback = true
	if c.generator == nil {
		fallback.FallbackReason = "generator_unavailable"
		return fallback
	}
	payload, err := json.Marshal(responseInput(bundle))
	if err != nil {
		fallback.FallbackReason = "encode_input"
		return fallback
	}
	raw, err := c.generator.GenerateJSON(ctx, SystemPrompt, string(payload), ResponseSchema)
	if err != nil {
		fallback.FallbackReason = "generate_failed"
		return fallback
	}
	draft, reason := validateDraft(raw, bundle)
	if reason != "" {
		return c.repairInvalidDraft(ctx, bundle, fallback, raw, reason)
	}
	return responseFromDraft(draft)
}

func (c Composer) repairInvalidDraft(ctx context.Context, bundle turn.FactBundle, fallback Response, originalDraft, initialReason string) Response {
	response := fallback
	response.FallbackReason = initialReason
	response.RepairAttempted = true
	response.InitialValidationReason = initialReason
	payload, err := json.Marshal(responseRepairInput(bundle, originalDraft, initialReason))
	if err != nil {
		response.RepairGenerationError = fmt.Sprintf("encode repaired response input: %v", err)
		return response
	}
	repairedRaw, err := c.generator.GenerateJSON(ctx, RepairSystemPrompt, string(payload), ResponseSchema)
	if err != nil {
		response.RepairGenerationError = fmt.Sprintf("generate repaired response: %v", err)
		return response
	}
	repaired, reason := validateDraft(repairedRaw, bundle)
	if reason != "" {
		response.RepairValidationReason = reason
		return response
	}
	response = responseFromDraft(repaired)
	response.RepairAttempted = true
	response.RepairSucceeded = true
	response.InitialValidationReason = initialReason
	return response
}

func responseFromDraft(draft ResponseDraft) Response {
	sentences := make([]ResponseSentence, len(draft.Sentences))
	for index, sentence := range draft.Sentences {
		sentences[index] = ResponseSentence{
			Text:    strings.TrimSpace(sentence.Text),
			FactIDs: append([]game.FactID(nil), sentence.FactIDs...),
		}
	}
	return responseFromSentences(sentences)
}

func responseFromSentences(sentences []ResponseSentence) Response {
	cloned := make([]ResponseSentence, len(sentences))
	parts := make([]string, len(sentences))
	used := make([]game.FactID, 0)
	seen := make(map[game.FactID]bool)
	for index, sentence := range sentences {
		cloned[index] = ResponseSentence{Text: strings.TrimSpace(sentence.Text), FactIDs: append([]game.FactID(nil), sentence.FactIDs...)}
		parts[index] = cloned[index].Text
		for _, id := range sentence.FactIDs {
			if !seen[id] {
				seen[id] = true
				used = append(used, id)
			}
		}
	}
	return Response{Text: strings.Join(parts, " "), Sentences: cloned, UsedFactIDs: used}
}
