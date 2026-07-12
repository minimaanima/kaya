package response

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"kaya/internal/game"
	"kaya/internal/kaya"
	"kaya/internal/turn"
)

func TestComposerAcceptsDraftCoveringRequiredFacts(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"I searched Doctor Near Cabinet. The doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if got.UsedFallback || got.Text != "I searched Doctor Near Cabinet. The doctor is dead." {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerRejectsUnknownFactID(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["secret"],"text":"There is a monster here."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "unknown_fact_id" {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerRejectsMissingRequiredFact(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001"],"text":"I checked the first doctor."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "missing_required_fact" {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerRejectsUnknownNamedEntity(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"I checked the doctor beside the Basement Door. The doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "unknown_entity" {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerAcceptsApprovedEntitiesAdjacentToPunctuation(t *testing.T) {
	bundle := turn.FactBundle{Facts: []game.Fact{{
		ID:       "f001",
		Kind:     game.FactVisibleObjects,
		Subject:  "reception",
		Value:    "Reception Desk, Reception Floor",
		Text:     "I can see: Reception Desk, Reception Floor.",
		Required: true,
	}}}
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001"],"text":"I can see: Reception Desk, Reception Floor."}]}`}
	got := NewComposer(gen).Compose(context.Background(), bundle)
	if got.UsedFallback {
		t.Fatalf("response = %#v, want accepted approved entities", got)
	}
}

func TestComposerAcceptsFactCitedNeutralParaphrase(t *testing.T) {
	bundle := turn.FactBundle{Facts: []game.Fact{
		{ID: "f001", Kind: game.FactRoomDescription, Subject: "reception", Value: "A damaged reception area. The ceiling has split above the front desk.", Text: "A damaged reception area. The ceiling has split above the front desk.", Required: true},
		{ID: "f002", Kind: game.FactVisibleObjects, Subject: "reception", Value: "Reception Desk, Reception Floor, Collapsed Chair", Text: "I can see: Reception Desk, Reception Floor, Collapsed Chair.", Required: true},
		{ID: "f003", Kind: game.FactKnownExits, Subject: "reception", Value: "east", Text: "I can go: east.", Required: true},
		{ID: "f004", Kind: game.FactElapsedTime, Subject: "time", Value: "5", Text: "5 seconds pass.", Required: true},
	}}
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001"],"text":"I see a damaged reception area with the ceiling split above the front desk."},{"factIds":["f002"],"text":"My view includes the Reception Desk, Reception Floor, and Collapsed Chair."},{"factIds":["f003"],"text":"The only exit I can access is east."},{"factIds":["f004"],"text":"Five seconds have passed since my observation began."}]}`}
	got := NewComposer(gen).Compose(context.Background(), bundle)
	if got.UsedFallback {
		t.Fatalf("response = %#v, want accepted fact-cited neutral paraphrase", got)
	}
}

func TestComposerRejectsUnsupportedLowercaseClaim(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"A monster is here. The doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "unsupported_claim" {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerRejectsUnsupportedOneWordClaim(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"North is open. The doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "unsupported_claim" {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerRejectsUnapprovedPredicate(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"The cabinet is open. The doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "unsupported_claim" {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerRejectsUnapprovedMovementClaim(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"The doctor went to the cabinet. The doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "unsupported_claim" {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerAcceptsNaturalGrammarAroundApprovedFacts(t *testing.T) {
	gen := &fakeGenerator{raw: `{"sentences":[{"factIds":["f001","f002"],"text":"I am near the doctor and the doctor is dead."}]}`}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if got.UsedFallback {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerFallsBackOnGeneratorError(t *testing.T) {
	gen := &fakeGenerator{err: errors.New("offline")}
	got := NewComposer(gen).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.Text == "" {
		t.Fatalf("response = %#v", got)
	}
}

func TestComposerRejectsStrictDraftViolations(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		reason string
	}{
		{name: "unknown field", raw: `{"sentences":[{"factIds":["f001","f002"],"text":"ok"}],"extra":true}`, reason: "invalid_draft"},
		{name: "trailing object", raw: `{"sentences":[{"factIds":["f001","f002"],"text":"ok"}]} {}`, reason: "invalid_draft"},
		{name: "too many sentences", raw: `{"sentences":[{"factIds":["f001","f002"],"text":"1"},{"factIds":["f001"],"text":"2"},{"factIds":["f001"],"text":"3"},{"factIds":["f001"],"text":"4"},{"factIds":["f001"],"text":"5"},{"factIds":["f001"],"text":"6"},{"factIds":["f001"],"text":"7"}]}`, reason: "invalid_draft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewComposer(&fakeGenerator{raw: tt.raw}).Compose(context.Background(), doctorBundle())
			if !got.UsedFallback || got.FallbackReason != tt.reason {
				t.Fatalf("response = %#v", got)
			}
		})
	}
}

func TestComposerRejectsOverlongDraft(t *testing.T) {
	text := strings.Repeat("x", 301)
	raw := fmt.Sprintf(`{"sentences":[{"factIds":["f001","f002"],"text":%q}]}`, text)
	got := NewComposer(&fakeGenerator{raw: raw}).Compose(context.Background(), doctorBundle())
	if !got.UsedFallback || got.FallbackReason != "invalid_draft" {
		t.Fatalf("response = %#v", got)
	}
}

type fakeGenerator struct {
	raw string
	err error
}

func (f *fakeGenerator) GenerateJSON(context.Context, string, string, any) (string, error) {
	return f.raw, f.err
}

func doctorBundle() turn.FactBundle {
	return turn.FactBundle{
		PlayerMessage: "search the doctors are they dead",
		Emotion:       kaya.EmotionUneasy,
		Facts: []game.Fact{
			{ID: "f001", Kind: game.FactAction, Subject: "Doctor Near Cabinet", Value: "searched", Text: "I searched Doctor Near Cabinet.", Required: true},
			{ID: "f002", Kind: game.FactLifeStatus, Subject: "Doctor Near Cabinet", Value: "dead", Text: "Doctor Near Cabinet is dead.", Required: true},
		},
	}
}
