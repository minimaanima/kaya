package playtest

import (
	"fmt"
	"sort"
	"strings"

	"kaya/internal/game"
	"kaya/internal/intent"
	"kaya/internal/kaya"
	"kaya/internal/response"
	"kaya/internal/rungen"
	"kaya/internal/turn"
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
		fmt.Fprintf(&b, "Processed: `%t`\n\n", step.Processed)
		writeSnapshot(&b, "Before", step.Before)
		if step.Processed {
			writePlan(&b, "Raw actions", step.Turn.Provenance.RawPlan, step.Turn.Provenance.HasRawPlan)
			writePlan(&b, "Resolved actions", step.Turn.Plan, true)
			writeProvenance(&b, step.Turn.Provenance)
			fmt.Fprintf(&b, "Processed turn duration: `%d`\n\n", step.Turn.DurationSeconds)
			writeResult(&b, step)
			writeResponse(&b, step)
		} else {
			writeUnprocessedTurnEvidence(&b)
		}
		writeSnapshot(&b, "After", step.After)
		writeStateDiff(&b, step.Before, step.After)
		writeViolations(&b, step.Violations)
		fmt.Fprintf(&b, "Objective emitted: `%t`\n\n", step.Processed && step.ObjectiveEmitted)
		if step.Error != "" {
			writeFencedSection(&b, "Error", step.Error)
		}
	}
	fmt.Fprintf(&b, "## Objective emissions\n\nObjective emissions: `%d`\n", value.ObjectiveEmissions)
	return b.String()
}

func writeUnprocessedTurnEvidence(b *strings.Builder) {
	writePlan(b, "Raw actions", intent.TurnPlan{}, false)
	writePlan(b, "Resolved actions", intent.TurnPlan{}, false)
	b.WriteString("Result evidence:\n- unavailable\n\n")
	b.WriteString("Response evidence:\n- unavailable\n\n")
}

func writePlan(b *strings.Builder, title string, plan intent.TurnPlan, present bool) {
	b.WriteString(title)
	b.WriteString(":\n")
	if !present {
		b.WriteString("- unavailable\n\n")
		return
	}

	var details strings.Builder
	fmt.Fprintf(&details, "confidence=%.2f needs_clarification=%t\nraw_text=%q\nclarification_question=%q\n", plan.Confidence, plan.NeedsClarification, plan.RawText, plan.ClarificationQuestion)
	for index, action := range plan.Actions {
		fmt.Fprintf(&details, "action %d: target_mode=%q %s\n", index+1, action.TargetMode, formatIntent(action.Intent))
	}
	for index, question := range plan.Questions {
		fmt.Fprintf(&details, "question %d: kind=%q target=%q targetMode=%q\n", index+1, question.Kind, question.Target, question.TargetMode)
	}
	if len(plan.Actions) == 0 && len(plan.Questions) == 0 {
		details.WriteString("none\n")
	}
	writeFenced(details.String(), b)
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
	b.WriteString("Result metadata:\n")
	writeFenced(fmt.Sprintf("stop_reason=%q\nclarification_question=%q\nemotion=%q\n", result.StopReason, result.ClarificationQuestion, result.Emotion), b)
	b.WriteString("Outcomes:\n")
	events := make([]game.WorldEvent, 0)
	if len(result.Outcomes) == 0 {
		b.WriteString("- none\n")
	} else {
		for index, outcome := range result.Outcomes {
			fmt.Fprintf(b, "Outcome %d:\n", index+1)
			writeFenced(formatActionOutcome(outcome), b)
			events = append(events, outcome.Result.Events...)
		}
	}
	writeEvents(b, "Events", events)
	writeFacts(b, "Question facts", result.QuestionFacts)
	b.WriteByte('\n')
}

func formatActionOutcome(outcome turn.ActionOutcome) string {
	result := outcome.Result
	return fmt.Sprintf(
		"intent: %s\ntarget_object_id=%q\nstatus=%q\ntarget_object_ids=[%s]\nstarted_at_seconds=%d\nduration_seconds=%d\noutcome=%q\nstress_delta=%d trust_delta=%d fear_delta=%d pain_delta=%d exhaustion_delta=%d\ndanger=%q\nneeds_clarification=%t\nclarification_question=%q\nvisible_facts:\n%s\nevents:\n%s\n",
		formatIntent(outcome.Intent),
		outcome.TargetObjectID,
		result.Status,
		joinObjectIDsPreserved(result.TargetObjectIDs),
		result.StartedAtSeconds,
		result.DurationSeconds,
		result.Outcome,
		result.StressDelta,
		result.TrustDelta,
		result.FearDelta,
		result.PainDelta,
		result.ExhaustionDelta,
		result.Danger,
		result.NeedsClarification,
		result.ClarificationQuestion,
		formatFacts(result.VisibleFacts),
		formatEvents(result.Events),
	)
}

func formatIntent(value intent.Intent) string {
	return fmt.Sprintf(
		"action=%q target=%q item=%q direction=%q modifiers=[%s] confidence=%.2f raw_text=%q needs_clarification=%t clarification_question=%q",
		value.Action,
		value.Target,
		value.Item,
		value.Direction,
		joinPreserved(value.Modifiers),
		value.Confidence,
		value.RawText,
		value.NeedsClarification,
		value.ClarificationQuestion,
	)
}

func writeFacts(b *strings.Builder, title string, facts []game.Fact) {
	b.WriteString(title)
	b.WriteString(":\n")
	writeFenced(formatFacts(facts), b)
}

func writeEvents(b *strings.Builder, title string, events []game.WorldEvent) {
	b.WriteString(title)
	b.WriteString(":\n")
	writeFenced(formatEvents(events), b)
}

func formatFacts(facts []game.Fact) string {
	if len(facts) == 0 {
		return "none\n"
	}
	var b strings.Builder
	for index, fact := range facts {
		fmt.Fprintf(&b, "fact %d: id=%q kind=%q subject=%q value=%q text=%q required=%t\n", index+1, fact.ID, fact.Kind, fact.Subject, fact.Value, fact.Text, fact.Required)
	}
	return b.String()
}

func formatEvents(events []game.WorldEvent) string {
	if len(events) == 0 {
		return "none\n"
	}
	var b strings.Builder
	for index, event := range events {
		fmt.Fprintf(&b, "event %d: type=%q description=%q danger=%q\n", index+1, event.Type, event.Description, event.Danger)
	}
	return b.String()
}

func writeResponse(b *strings.Builder, step Step) {
	response := step.Turn.Response
	writeFencedSection(b, "Response text", response.Text)
	b.WriteString("Response sentence evidence:\n")
	writeFenced(formatResponseSentences(response.Sentences), b)
	b.WriteString("Response metadata:\n")
	fmt.Fprintf(b, "- Fallback flag: `%t`\n", response.UsedFallback)
	fmt.Fprintf(b, "- Repair attempted: `%t`\n", response.RepairAttempted)
	fmt.Fprintf(b, "- Repair succeeded: `%t`\n", response.RepairSucceeded)
	ids := append([]game.FactID(nil), response.UsedFactIDs...)
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
	if strings.TrimSpace(response.InitialValidationReason) != "" {
		writeFencedSection(b, "Initial validation reason", response.InitialValidationReason)
	}
	if strings.TrimSpace(response.RepairValidationReason) != "" {
		writeFencedSection(b, "Repair validation reason", response.RepairValidationReason)
	}
	if strings.TrimSpace(response.RepairGenerationError) != "" {
		writeFencedSection(b, "Repair generation error", response.RepairGenerationError)
	}
}

func formatResponseSentences(sentences []response.ResponseSentence) string {
	if len(sentences) == 0 {
		return "none\n"
	}
	var b strings.Builder
	for index, sentence := range sentences {
		fmt.Fprintf(&b, "sentence %d: text=%q fact_ids=[%s]\n", index+1, sentence.Text, joinFactIDsPreserved(sentence.FactIDs))
	}
	return b.String()
}

func joinFactIDsPreserved(ids []game.FactID) string {
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = string(id)
	}
	return strings.Join(values, ",")
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
		beforeValue, beforePresent := left[key]
		afterValue, afterPresent := right[key]
		if beforePresent && afterPresent && beforeValue == afterValue {
			continue
		}
		found = true
		switch {
		case !beforePresent:
			fmt.Fprintf(b, "- %s: added=%q\n", key, afterValue)
		case !afterPresent:
			fmt.Fprintf(b, "- %s: removed=%q\n", key, beforeValue)
		default:
			fmt.Fprintf(b, "- %s: before=%q after=%q\n", key, beforeValue, afterValue)
		}
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
		writeFencedSection(b, "Violation code", violation.Code)
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
	for _, roomID := range sortedRoomVisibilityKeys(snapshot.RoomVisibility) {
		entries["room_visibility."+string(roomID)] = string(snapshot.RoomVisibility[roomID])
	}
	for _, roomID := range sortedRoomObjectKeys(snapshot.RoomObjects) {
		entries["room_objects."+string(roomID)] = joinObjectIDsPreserved(snapshot.RoomObjects[roomID])
	}
	for _, key := range sortedDoorKeys(snapshot.DoorStates) {
		entries["door_state."+string(key)] = string(snapshot.DoorStates[key])
	}
	for _, doorID := range sortedDoorNameKeys(snapshot.DoorNames) {
		entries["door_name."+string(doorID)] = snapshot.DoorNames[doorID]
	}
	for _, doorID := range sortedDoorAliasKeys(snapshot.DoorAliases) {
		entries["door_aliases."+string(doorID)] = joinSorted(snapshot.DoorAliases[doorID])
	}
	for _, roomID := range sortedKnownExitRoomKeys(snapshot.KnownExitDirections) {
		prefix := "known_exit_directions." + string(roomID)
		entries[prefix] = "present"
		for _, direction := range sortedDirectionKeys(snapshot.KnownExitDirections[roomID]) {
			entries[prefix+"."+direction] = fmt.Sprintf("%t", snapshot.KnownExitDirections[roomID][direction])
		}
	}
	for index, group := range snapshot.RecentReferents {
		prefix := fmt.Sprintf("recent_referents.%d", index)
		entries[prefix] = "present"
		entries[prefix+".object_ids"] = joinObjectIDsPreserved(group.ObjectIDs)
		entries[prefix+".item_ids"] = joinItemIDsPreserved(group.ItemIDs)
	}
	entries["last_mentioned_item_id"] = string(snapshot.LastMentionedItemID)
	entries["last_mentioned_item_ids"] = joinItemIDsPreserved(snapshot.LastMentionedItemIDs)
	for _, objectID := range sortedObservedFactObjectKeys(snapshot.ObservedObjectFacts) {
		prefix := "observed_object_facts." + string(objectID)
		entries[prefix] = "present"
		for _, kind := range sortedFactKindKeys(snapshot.ObservedObjectFacts[objectID]) {
			fact := snapshot.ObservedObjectFacts[objectID][kind]
			entries[prefix+"."+string(kind)] = formatFact(fact)
		}
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
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = fmt.Sprintf("%d:%s:%s:%s", value.TriggerAtSeconds, value.Event.Type, value.Event.Danger, value.Event.Description)
	}
	return strings.Join(parts, ",")
}

func sortedRoomVisibilityKeys(values map[game.RoomID]world.Visibility) []game.RoomID {
	keys := make([]game.RoomID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedRoomObjectKeys(values map[game.RoomID][]game.ObjectID) []game.RoomID {
	keys := make([]game.RoomID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedDoorNameKeys(values map[game.DoorID]string) []game.DoorID {
	keys := make([]game.DoorID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedDoorAliasKeys(values map[game.DoorID][]string) []game.DoorID {
	keys := make([]game.DoorID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func joinPreserved(values []string) string {
	return strings.Join(values, ",")
}

func joinObjectIDsPreserved(values []game.ObjectID) string {
	formatted := make([]string, len(values))
	for index, value := range values {
		formatted[index] = string(value)
	}
	return strings.Join(formatted, ",")
}

func joinItemIDsPreserved(values []game.ItemID) string {
	formatted := make([]string, len(values))
	for index, value := range values {
		formatted[index] = string(value)
	}
	return strings.Join(formatted, ",")
}

func formatFact(fact game.Fact) string {
	return fmt.Sprintf("id=%q kind=%q subject=%q value=%q text=%q required=%t", fact.ID, fact.Kind, fact.Subject, fact.Value, fact.Text, fact.Required)
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

func sortedKnownExitRoomKeys(values map[game.RoomID]map[string]bool) []game.RoomID {
	keys := make([]game.RoomID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedDirectionKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedObservedFactObjectKeys(values map[game.ObjectID]map[game.FactKind]game.Fact) []game.ObjectID {
	keys := make([]game.ObjectID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedFactKindKeys(values map[game.FactKind]game.Fact) []game.FactKind {
	keys := make([]game.FactKind, 0, len(values))
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
