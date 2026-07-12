package playtest

import (
	"fmt"
	"sort"
	"strings"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/kaya"
	"kaya/internal/rungen"
	"kaya/internal/world"
)

// RenderMarkdown produces a reproducible, complete transcript for a stateful playtest session.
func RenderMarkdown(value Session) string {
	var b strings.Builder
	b.WriteString("# Kaya Stateful Playtest\n\n")
	fmt.Fprintf(&b, "Scenario: `%s`\n", value.ScenarioID)
	fmt.Fprintf(&b, "Scenario version: `%d`\n", value.ScenarioVersion)
	fmt.Fprintf(&b, "Generator version: `%d`\n", value.GeneratorVersion)
	fmt.Fprintf(&b, "Seed: `%d`\n\n", value.Seed)

	b.WriteString("## Placements\n\n")
	placements := append([]rungen.Placement(nil), value.Placements...)
	sort.Slice(placements, func(i, j int) bool {
		if placements[i].ItemID != placements[j].ItemID {
			return placements[i].ItemID < placements[j].ItemID
		}
		return placements[i].ObjectID < placements[j].ObjectID
	})
	if len(placements) == 0 {
		b.WriteString("- none\n\n")
	} else {
		for _, placement := range placements {
			fmt.Fprintf(&b, "- %s (`%s`) -> `%s`\n", displayID(string(placement.ItemID)), placement.ItemID, placement.ObjectID)
		}
		b.WriteByte('\n')
	}

	for _, step := range value.Steps {
		fmt.Fprintf(&b, "## Step %d\n\n", step.Number)
		writeFencedSection(&b, "Player", step.Player)
		writeSnapshot(&b, "Before", step.Before)
		writePlan(&b, "Raw actions", step.Turn.Provenance.RawPlan, step.Turn.Provenance.HasRawPlan)
		writePlan(&b, "Resolved actions", step.Turn.Plan, true)
		writeProvenance(&b, step.Turn.Provenance)
		writeResult(&b, step)
		writeResponse(&b, step)
		writeSnapshot(&b, "After", step.After)
		writeStateDiff(&b, step.Before, step.After)
		writeViolations(&b, step.Violations)
		fmt.Fprintf(&b, "Objective emitted: `%t`\n\n", step.ObjectiveEmitted)
		if step.Error != "" {
			writeFencedSection(&b, "Error", step.Error)
		}
	}
	fmt.Fprintf(&b, "## Objective emissions\n\nObjective emissions: `%d`\n", value.ObjectiveEmissions)
	return b.String()
}

func writePlan(b *strings.Builder, title string, plan intent.TurnPlan, present bool) {
	b.WriteString(title)
	b.WriteString(":\n")
	if !present {
		b.WriteString("- unavailable\n\n")
		return
	}

	var details strings.Builder
	fmt.Fprintf(&details, "confidence=%.2f needsClarification=%t\n", plan.Confidence, plan.NeedsClarification)
	for index, action := range plan.Actions {
		fmt.Fprintf(&details, "action %d: action=%q target=%q item=%q direction=%q targetMode=%q confidence=%.2f modifiers=%s\n", index+1, action.Intent.Action, action.Intent.Target, action.Intent.Item, action.Intent.Direction, action.TargetMode, action.Intent.Confidence, joinSorted(action.Intent.Modifiers))
	}
	for index, question := range plan.Questions {
		fmt.Fprintf(&details, "question %d: kind=%q target=%q targetMode=%q\n", index+1, question.Kind, question.Target, question.TargetMode)
	}
	if len(plan.Actions) == 0 && len(plan.Questions) == 0 {
		details.WriteString("none\n")
	}
	writeFenced(details.String(), b)
	if plan.NeedsClarification || strings.TrimSpace(plan.ClarificationQuestion) != "" {
		writeFencedSection(b, title+" clarification question", plan.ClarificationQuestion)
	}
}

func writeProvenance(b *strings.Builder, provenance intent.ParseProvenance) {
	fmt.Fprintf(b, "Parse source: `%s`\n", provenance.Source)
	fmt.Fprintf(b, "Generator provenance: `%s`\n", generatorProvenance(provenance.Source))
	fmt.Fprintf(b, "Repair provenance: `%s`\n", provenanceSource(provenance.Source, intent.ParseSourceRepair))
	fmt.Fprintf(b, "Fallback provenance: `%s`\n", provenanceSource(provenance.Source, intent.ParseSourceFallback))
	fmt.Fprintf(b, "Raw plan captured: `%t`\n", provenance.HasRawPlan)
	fmt.Fprintf(b, "Canonicalized: `%t`\n", provenance.Canonicalized)

	errors := []namedError{
		{name: "Fallback", err: provenance.FallbackError},
		{name: "Repair", err: provenance.RepairReason},
	}
	sort.Slice(errors, func(i, j int) bool { return errors[i].name < errors[j].name })
	b.WriteString("Provenance errors:\n")
	found := false
	for _, entry := range errors {
		if entry.err == nil {
			continue
		}
		found = true
		writeFencedSection(b, entry.name, entry.err.Error())
	}
	if !found {
		b.WriteString("- none\n\n")
	}
}

type namedError struct {
	name string
	err  error
}

func provenanceSource(source, expected intent.ParseSource) string {
	if source == expected {
		return "used"
	}
	return "not used"
}

func generatorProvenance(source intent.ParseSource) string {
	if source == intent.ParseSourceModel || source == intent.ParseSourceRepair {
		return "used"
	}
	return "not used"
}

func writeResult(b *strings.Builder, step Step) {
	result := step.Turn.Result
	b.WriteString("Outcomes:\n")
	events := make([]game.WorldEvent, 0)
	if len(result.Outcomes) == 0 {
		b.WriteString("- none\n")
	} else {
		for index, outcome := range result.Outcomes {
			fmt.Fprintf(b, "- %d: action=`%s` targetObject=`%s` status=`%s` outcome=`%s` start=`%d` duration=`%d` danger=`%s`\n", index+1, outcome.Intent.Action, outcome.TargetObjectID, outcome.Result.Status, outcome.Result.Outcome, outcome.Result.StartedAtSeconds, outcome.Result.DurationSeconds, outcome.Result.Danger)
			writeFacts(b, "  Facts", outcome.Result.VisibleFacts)
			writeEvents(b, "  Outcome events", outcome.Result.Events)
			events = append(events, outcome.Result.Events...)
		}
	}
	writeEvents(b, "Events", events)
	fmt.Fprintf(b, "Stop reason: `%s`\n", result.StopReason)
	if strings.TrimSpace(result.ClarificationQuestion) != "" {
		writeFencedSection(b, "Result clarification question", result.ClarificationQuestion)
	}
	writeFacts(b, "Question facts", result.QuestionFacts)
	b.WriteByte('\n')
}

func writeFacts(b *strings.Builder, title string, facts []game.Fact) {
	b.WriteString(title)
	b.WriteString(":\n")
	cloned := append([]game.Fact(nil), facts...)
	sort.Slice(cloned, func(i, j int) bool {
		left, right := cloned[i], cloned[j]
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Subject != right.Subject {
			return left.Subject < right.Subject
		}
		if left.Value != right.Value {
			return left.Value < right.Value
		}
		return left.Text < right.Text
	})
	if len(cloned) == 0 {
		b.WriteString("- none\n")
		return
	}
	for _, fact := range cloned {
		fmt.Fprintf(b, "- id=`%s` kind=`%s` subject=%q value=%q required=`%t` text=%q\n", fact.ID, fact.Kind, fact.Subject, fact.Value, fact.Required, fact.Text)
	}
}

func writeEvents(b *strings.Builder, title string, events []game.WorldEvent) {
	b.WriteString(title)
	b.WriteString(":\n")
	cloned := append([]game.WorldEvent(nil), events...)
	sort.Slice(cloned, func(i, j int) bool {
		if cloned[i].Type != cloned[j].Type {
			return cloned[i].Type < cloned[j].Type
		}
		if cloned[i].Description != cloned[j].Description {
			return cloned[i].Description < cloned[j].Description
		}
		return cloned[i].Danger < cloned[j].Danger
	})
	if len(cloned) == 0 {
		b.WriteString("- none\n")
		return
	}
	for _, event := range cloned {
		fmt.Fprintf(b, "- type=`%s` danger=`%s` description=%q\n", event.Type, event.Danger, event.Description)
	}
}

func writeResponse(b *strings.Builder, step Step) {
	response := step.Turn.Response
	writeFencedSection(b, "Response text", response.Text)
	b.WriteString("Response metadata:\n")
	fmt.Fprintf(b, "- Fallback flag: `%t`\n", response.UsedFallback)
	ids := append([]game.FactID(nil), response.UsedFactIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 0 {
		b.WriteString("- Used fact IDs: none\n\n")
	} else {
		formatted := make([]string, len(ids))
		for index, id := range ids {
			formatted[index] = fmt.Sprintf("`%s`", id)
		}
		fmt.Fprintf(b, "- Used fact IDs: %s\n\n", strings.Join(formatted, ", "))
	}
	if strings.TrimSpace(response.FallbackReason) != "" {
		writeFencedSection(b, "Fallback reason", response.FallbackReason)
	}
}

func writeSnapshot(b *strings.Builder, title string, snapshot Snapshot) {
	b.WriteString(title)
	b.WriteString(":\n")
	writeFenced(strings.Join(snapshotLines(snapshot), "\n")+"\n", b)
}

func writeStateDiff(b *strings.Builder, before, after Snapshot) {
	b.WriteString("State diff:\n")
	left, right := snapshotEntryMap(before), snapshotEntryMap(after)
	keys := make([]string, 0, len(left)+len(right))
	seen := make(map[string]bool, len(left)+len(right))
	for key := range left {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range right {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	found := false
	for _, key := range keys {
		if left[key] == right[key] {
			continue
		}
		found = true
		fmt.Fprintf(b, "- %s: before=%q after=%q\n", key, left[key], right[key])
	}
	if !found {
		b.WriteString("- none\n")
	}
	b.WriteByte('\n')
}

func writeViolations(b *strings.Builder, violations []Violation) {
	b.WriteString("Violations:\n")
	cloned := append([]Violation(nil), violations...)
	sort.Slice(cloned, func(i, j int) bool {
		if cloned[i].Code != cloned[j].Code {
			return cloned[i].Code < cloned[j].Code
		}
		return cloned[i].Detail < cloned[j].Detail
	})
	if len(cloned) == 0 {
		b.WriteString("- none\n\n")
		return
	}
	for _, violation := range cloned {
		fmt.Fprintf(b, "- code=`%s`\n", violation.Code)
		writeFencedSection(b, "Violation detail", violation.Detail)
	}
}

func snapshotLines(snapshot Snapshot) []string {
	entries := snapshotEntryMap(snapshot)
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", key, entries[key]))
	}
	return lines
}

func snapshotEntryMap(snapshot Snapshot) map[string]string {
	entries := map[string]string{
		"active_light":     fmt.Sprintf("%t", snapshot.ActiveLight),
		"current_room":     string(snapshot.CurrentRoom),
		"previous_room":    string(snapshot.PreviousRoom),
		"time":             fmt.Sprintf("%d", snapshot.Time),
		"kaya":             formatKaya(snapshot.Kaya),
		"inventory":        joinItemIDs(snapshot.Inventory),
		"discovered":       joinItemIDs(snapshot.Discovered),
		"event_times":      joinInts(snapshot.RemainingEventTimes),
		"scheduled_events": joinScheduledEvents(snapshot.RemainingEvents),
	}
	for _, key := range sortedItemNameKeys(snapshot.ItemNames) {
		entries["item_name."+string(key)] = snapshot.ItemNames[key]
	}
	for _, key := range sortedItemAliasKeys(snapshot.ItemAliases) {
		entries["item_aliases."+string(key)] = joinSorted(snapshot.ItemAliases[key])
	}
	for _, key := range sortedObjectItemKeys(snapshot.ObjectItems) {
		entries["object_items."+string(key)] = joinItemIDs(snapshot.ObjectItems[key])
	}
	for _, key := range sortedObjectItemKeys(snapshot.ObjectRevealedItems) {
		entries["object_revealed_items."+string(key)] = joinItemIDs(snapshot.ObjectRevealedItems[key])
	}
	for _, key := range sortedDoorKeys(snapshot.DoorStates) {
		entries["door_state."+string(key)] = string(snapshot.DoorStates[key])
	}
	return entries
}

func joinItemIDs(values []game.ItemID) string {
	cloned := append([]game.ItemID(nil), values...)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i] < cloned[j] })
	valuesAsStrings := make([]string, len(cloned))
	for index, value := range cloned {
		valuesAsStrings[index] = string(value)
	}
	return strings.Join(valuesAsStrings, ",")
}

func joinInts(values []int) string {
	cloned := append([]int(nil), values...)
	sort.Ints(cloned)
	formatted := make([]string, len(cloned))
	for index, value := range cloned {
		formatted[index] = fmt.Sprintf("%d", value)
	}
	return strings.Join(formatted, ",")
}

func joinScheduledEvents(values []world.ScheduledEvent) string {
	cloned := append([]world.ScheduledEvent(nil), values...)
	sort.Slice(cloned, func(i, j int) bool {
		left, right := cloned[i], cloned[j]
		if left.TriggerAtSeconds != right.TriggerAtSeconds {
			return left.TriggerAtSeconds < right.TriggerAtSeconds
		}
		if left.Event.Type != right.Event.Type {
			return left.Event.Type < right.Event.Type
		}
		if left.Event.Description != right.Event.Description {
			return left.Event.Description < right.Event.Description
		}
		return left.Event.Danger < right.Event.Danger
	})
	parts := make([]string, len(cloned))
	for index, value := range cloned {
		parts[index] = fmt.Sprintf("%d:%s:%s:%s", value.TriggerAtSeconds, value.Event.Type, value.Event.Danger, value.Event.Description)
	}
	return strings.Join(parts, ",")
}

func joinSorted(values []string) string {
	cloned := append([]string(nil), values...)
	sort.Strings(cloned)
	return strings.Join(cloned, ",")
}

func sortedItemNameKeys(values map[game.ItemID]string) []game.ItemID {
	keys := make([]game.ItemID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedItemAliasKeys(values map[game.ItemID][]string) []game.ItemID {
	keys := make([]game.ItemID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedObjectItemKeys(values map[game.ObjectID][]game.ItemID) []game.ObjectID {
	keys := make([]game.ObjectID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedDoorKeys(values map[game.DoorID]world.DoorState) []game.DoorID {
	keys := make([]game.DoorID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func formatKaya(value kaya.State) string {
	return fmt.Sprintf("stress=%d trust=%d fear=%d pain=%d exhaustion=%d emotion=%s", value.Stress, value.Trust, value.Fear, value.Pain, value.Exhaustion, value.DominantEmotion())
}

func displayID(value string) string {
	words := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for index, word := range words {
		words[index] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func writeFencedSection(b *strings.Builder, title, value string) {
	b.WriteString(title)
	b.WriteString(":\n")
	writeFenced(value, b)
}

func writeFenced(value string, b *strings.Builder) {
	delimiter := strings.Repeat("`", longestBacktickRun(value)+1)
	if len(delimiter) < 3 {
		delimiter = "```"
	}
	b.WriteString(delimiter)
	b.WriteByte('\n')
	b.WriteString(value)
	if !strings.HasSuffix(value, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(delimiter)
	b.WriteString("\n\n")
}

func longestBacktickRun(value string) int {
	longest, current := 0, 0
	for _, character := range value {
		if character == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	return longest
}
