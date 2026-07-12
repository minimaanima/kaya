package playtest

import (
	"context"
	"fmt"
	"strings"

	"kaya/internal/game"
	"kaya/internal/rungen"
	"kaya/internal/scenario"
)

func PrototypeWinningMessages(run rungen.GeneratedRun, seed int64) ([]string, error) {
	flashlightPlacement, keyPlacement, err := prototypePlacements(run.Placements)
	if err != nil {
		return nil, err
	}
	if run.State == nil {
		return nil, fmt.Errorf("generated run has no world state")
	}
	flashlightObject, ok := run.State.Objects[flashlightPlacement.ObjectID]
	if !ok {
		return nil, fmt.Errorf("flashlight placement object %q is missing", flashlightPlacement.ObjectID)
	}
	keyObject, ok := run.State.Objects[keyPlacement.ObjectID]
	if !ok {
		return nil, fmt.Errorf("key placement object %q is missing", keyPlacement.ObjectID)
	}

	selector := newSplitMix64(seed)
	unlock, err := fallbackExecutableUnlockPhrase(PrototypePhrases.Unlock)
	if err != nil {
		return nil, err
	}
	messages := []string{
		selector.phrase(PrototypePhrases.Awareness),
		fmt.Sprintf(selector.phrase(PrototypePhrases.Search), flashlightObject.Name),
		selector.phrase(PrototypePhrases.TakeFlashlight),
		selector.phrase(PrototypePhrases.MoveEast),
		selector.phrase(PrototypePhrases.LightOn),
		selector.phrase(PrototypePhrases.Awareness),
		fmt.Sprintf(selector.phrase(PrototypePhrases.Search), keyObject.Name),
		selector.phrase(PrototypePhrases.TakeKey),
		unlock,
		selector.phrase(PrototypePhrases.MoveNorth),
	}

	switch uint64(seed) % 4 {
	case 0:
		messages[2] = messages[2] + " then " + messages[3]
		messages = append(messages[:3], messages[4:]...)
	case 1:
		messages[4] = messages[4] + " then " + messages[5]
		messages = append(messages[:5], messages[6:]...)
	}
	return messages, nil
}

func RunPrototypeSession(ctx context.Context, runner *Runner, run rungen.GeneratedRun, seed int64) error {
	if runner == nil {
		return fmt.Errorf("prototype runner is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	messages, err := PrototypeWinningMessages(run, seed)
	if err != nil {
		return fmt.Errorf("seed %d placements=%#v: build winning messages: %w", seed, run.Placements, err)
	}
	for _, message := range messages {
		if _, err := runner.Step(ctx, message); err != nil {
			return fmt.Errorf("seed %d placements=%#v message %q: %w\nsession=%#v", seed, run.Placements, message, err, runner.Session())
		}
	}
	return nil
}

func fallbackExecutableUnlockPhrase(phrases []string) (string, error) {
	for _, phrase := range phrases {
		if strings.HasPrefix(phrase, "use ") {
			return phrase, nil
		}
	}
	return "", fmt.Errorf("phrase bank has no fallback-executable unlock phrase")
}

func prototypePlacements(placements []rungen.Placement) (rungen.Placement, rungen.Placement, error) {
	var flashlightPlacement, keyPlacement rungen.Placement
	for _, placement := range placements {
		switch placement.ItemID {
		case game.ItemID(scenario.ItemFlashlight):
			flashlightPlacement = placement
		case game.ItemID(scenario.ItemBrassKey):
			keyPlacement = placement
		}
	}
	if flashlightPlacement.ObjectID == "" {
		return rungen.Placement{}, rungen.Placement{}, fmt.Errorf("generated run has no flashlight placement")
	}
	if keyPlacement.ObjectID == "" {
		return rungen.Placement{}, rungen.Placement{}, fmt.Errorf("generated run has no brass key placement")
	}
	return flashlightPlacement, keyPlacement, nil
}
