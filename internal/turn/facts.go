package turn

import (
	"fmt"
	"strconv"

	"kaya/internal/game"
)

func (r Result) FactBundle(playerMessage string) FactBundle {
	bundle := FactBundle{
		PlayerMessage: playerMessage,
		Outcomes:      append([]ActionOutcome(nil), r.Outcomes...),
		Emotion:       r.Emotion,
	}
	for _, outcome := range r.Outcomes {
		hasFailure, hasClarification := false, false
		for _, fact := range outcome.Result.VisibleFacts {
			bundle.Facts = append(bundle.Facts, fact)
			hasFailure = hasFailure || fact.Kind == game.FactFailure
			hasClarification = hasClarification || fact.Kind == game.FactClarification
		}
		if (outcome.Result.Status == game.ActionFailed || outcome.Result.Status == game.ActionRefused) && !hasFailure {
			bundle.Facts = append(bundle.Facts, game.Fact{Kind: game.FactFailure, Subject: "action", Value: string(outcome.Result.Status), Text: outcome.Result.Outcome, Required: true})
		}
		if (outcome.Result.Status == game.ActionClarification || outcome.Result.NeedsClarification) && !hasClarification {
			bundle.Facts = append(bundle.Facts, game.Fact{Kind: game.FactClarification, Subject: "action", Value: "needs_clarification", Text: outcome.Result.ClarificationQuestion, Required: true})
		}
		if outcome.Result.DurationSeconds > 0 {
			seconds := outcome.Result.DurationSeconds
			bundle.Facts = append(bundle.Facts, game.Fact{Kind: game.FactElapsedTime, Subject: "time", Value: strconv.Itoa(seconds), Text: elapsedText(seconds), Required: true})
		}
		for _, event := range outcome.Result.Events {
			bundle.Facts = append(bundle.Facts, game.Fact{Kind: game.FactEvent, Subject: string(event.Type), Value: event.Description, Text: event.Description, Required: true})
		}
	}
	for _, fact := range r.QuestionFacts {
		fact.Required = true
		bundle.Facts = append(bundle.Facts, fact)
	}
	for i := range bundle.Facts {
		bundle.Facts[i].ID = game.FactID(fmt.Sprintf("f%03d", i+1))
	}
	return bundle
}

func elapsedText(seconds int) string {
	if seconds == 1 {
		return "1 second passes."
	}
	return fmt.Sprintf("%d seconds pass.", seconds)
}
