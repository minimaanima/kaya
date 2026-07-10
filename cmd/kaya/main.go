package main

import (
	"bufio"
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kaya/internal/actions"
	"kaya/internal/game"
	"kaya/internal/intent"
	kayastate "kaya/internal/kaya"
	"kaya/internal/llm"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/world"
)

const defaultOllamaModel = "qwen3.5:4b"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: kaya <intent|play|playtest>")
		return
	}

	switch os.Args[1] {
	case "intent":
		if err := runIntent(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "play":
		if err := runPlay(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "playtest":
		if err := runPlaytest(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}

func runIntent(args []string) error {
	message := strings.TrimSpace(strings.Join(args, " "))
	if message == "" {
		return fmt.Errorf("usage: kaya intent <player message>")
	}

	model := envOrDefault("KAYA_OLLAMA_MODEL", defaultOllamaModel)
	baseURL := envOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)

	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		return err
	}

	parser := intent.NewParser(client)
	parsed, err := parser.Parse(context.Background(), message)
	if err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return fmt.Errorf("encode intent: %w", err)
	}

	fmt.Println(string(encoded))
	return nil
}

func runPlay(args []string) error {
	options, err := parsePlayOptions(args, newRunSeed)
	if err != nil {
		return err
	}
	run, err := rungen.Generate(
		rungen.RunConfig{
			Seed:             options.Seed,
			GeneratorVersion: rungen.CurrentGeneratorVersion,
		},
		runscenario.PrototypeDefinition(),
	)
	if err != nil {
		return fmt.Errorf("generate run: %w", err)
	}

	model := envOrDefault("KAYA_OLLAMA_MODEL", defaultOllamaModel)
	baseURL := envOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)

	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		return err
	}

	parser := intent.NewParser(client)
	state := run.State
	resolver := actions.NewResolver(state)
	scanner := bufio.NewScanner(os.Stdin)

	printRunDebug(os.Stdout, run)
	fmt.Println("Connection established.")
	fmt.Println("Kaya: I can read you. I am in reception. The ceiling is cracked, but I can move.")
	fmt.Println("Type naturally. Use 'quit' or 'exit' to stop.")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		message := strings.TrimSpace(scanner.Text())
		if message == "" {
			continue
		}
		if message == "quit" || message == "exit" {
			fmt.Println("Connection closed.")
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		parsed, err := parser.Parse(ctx, message)
		cancel()
		if err != nil {
			fmt.Println("Kaya: The signal broke up. I did not understand that.")
			fmt.Println("debug:", err)
			continue
		}

		result := resolver.Resolve(parsed)
		printResult(result)
		if state.CurrentRoomID == scenario.RoomStairwell {
			fmt.Println("Kaya: I am in the stairwell. This part is clear.")
			fmt.Println("Prototype objective complete.")
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	return nil
}

func runPlaytest() error {
	model := envOrDefault("KAYA_OLLAMA_MODEL", defaultOllamaModel)
	baseURL := envOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)

	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		return err
	}

	parser := intent.NewParser(client)
	run := playtestRun{
		Model:     model,
		BaseURL:   baseURL,
		StartedAt: time.Now(),
	}

	for _, script := range defaultPlaytestScripts() {
		runScript, err := runPlaytestScript(parser, script)
		if err != nil {
			return err
		}
		run.Scripts = append(run.Scripts, runScript)
	}

	logPath, err := writePlaytestLog(run)
	if err != nil {
		return err
	}

	fmt.Println("Playtest log:", logPath)
	for _, script := range run.Scripts {
		fmt.Printf("- %s: %d steps, %d suspicious\n", script.Name, len(script.Steps), len(script.Suspicious))
		for _, note := range script.Suspicious {
			fmt.Println("  -", note)
		}
	}
	return nil
}

type playOptions struct {
	Seed int64
}

func parsePlayOptions(args []string, generateSeed func() (int64, error)) (playOptions, error) {
	flags := flag.NewFlagSet("play", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	seed := flags.Int64("seed", 0, "reproducible run seed")
	if err := flags.Parse(args); err != nil {
		return playOptions{}, err
	}
	if flags.NArg() != 0 {
		return playOptions{}, fmt.Errorf("usage: kaya play [--seed <int64>]")
	}

	provided := false
	flags.Visit(func(current *flag.Flag) {
		if current.Name == "seed" {
			provided = true
		}
	})
	if provided {
		return playOptions{Seed: *seed}, nil
	}

	generated, err := generateSeed()
	if err != nil {
		return playOptions{}, err
	}
	return playOptions{Seed: generated}, nil
}

func newRunSeed() (int64, error) {
	return readRunSeed(cryptorand.Reader)
}

func readRunSeed(reader io.Reader) (int64, error) {
	const positiveMask = ^uint64(0) >> 1
	for {
		var value [8]byte
		if _, err := io.ReadFull(reader, value[:]); err != nil {
			return 0, fmt.Errorf("generate run seed: %w", err)
		}
		seed := int64(binary.LittleEndian.Uint64(value[:]) & positiveMask)
		if seed != 0 {
			return seed, nil
		}
	}
}

func printRunDebug(writer io.Writer, run rungen.GeneratedRun) {
	fmt.Fprintf(writer, "Run seed: %d\n", run.Seed)
	fmt.Fprintf(writer, "Generator: %d\n", run.GeneratorVersion)

	placements := append([]rungen.Placement(nil), run.Placements...)
	sort.Slice(placements, func(i, j int) bool {
		return placements[i].ItemID < placements[j].ItemID
	})
	for _, placement := range placements {
		itemName := string(placement.ItemID)
		if item, ok := run.State.Items[placement.ItemID]; ok {
			itemName = item.Name
		}
		objectName := string(placement.ObjectID)
		if object, ok := run.State.Objects[placement.ObjectID]; ok {
			objectName = object.Name
		}
		fmt.Fprintf(writer, "%s: %s\n", itemName, objectName)
	}
	fmt.Fprintf(
		writer,
		"Validation: playable (%d witness steps, %d states)\n",
		len(run.Validation.Witness),
		run.Validation.VisitedStates,
	)
}

func runPlaytestScript(parser intent.Parser, script playtestScript) (playtestScriptLog, error) {
	state := scenario.NewPrototypeWorld()
	if script.UseInitialKaya {
		state.Kaya = script.InitialKaya
	}
	if script.InitialRoom != "" {
		state.CurrentRoomID = script.InitialRoom
	}
	for _, itemID := range script.InitialInventory {
		state.AddInventory(itemID)
	}
	state.ActiveLight = script.InitialLight

	resolver := actions.NewResolver(state)
	log := playtestScriptLog{Name: script.Name}

	for i, planned := range script.playtestMessages() {
		message := planned.Player
		step := playtestStep{
			Number:   i + 1,
			Player:   message,
			Expected: planned.Expect,
			Before: playtestWorldState{
				Room:       string(state.CurrentRoomID),
				Time:       state.NowSeconds,
				Inventory:  inventoryNames(state),
				Discovered: discoveredItemNames(state),
				LightOn:    state.ActiveLight,
				Kaya:       state.Kaya,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		parsed, err := parser.Parse(ctx, message)
		cancel()
		if err != nil {
			step.ParseError = err.Error()
			step.Suspicious = true
			step.Suspicion = "parse_error"
			log.Steps = append(log.Steps, step)
			log.Suspicious = append(log.Suspicious, fmt.Sprintf("step %d parse error for %q: %v", step.Number, message, err))
			continue
		}

		result := resolver.Resolve(parsed)
		step.Intent = parsed
		step.Result = result
		step.After = playtestWorldState{
			Room:       string(state.CurrentRoomID),
			Time:       state.NowSeconds,
			Inventory:  inventoryNames(state),
			Discovered: discoveredItemNames(state),
			LightOn:    state.ActiveLight,
			Kaya:       state.Kaya,
		}

		if reason, ok := suspiciousOutcome(parsed, result); ok {
			addSuspicion(&log, &step, reason)
		}
		for _, reason := range expectationMismatches(planned.Expect, parsed, result) {
			addSuspicion(&log, &step, reason)
		}

		log.Steps = append(log.Steps, step)
		if state.CurrentRoomID == scenario.RoomStairwell {
			break
		}
	}

	return log, nil
}

func printResult(result game.ActionResult) {
	if result.NeedsClarification && result.ClarificationQuestion != "" {
		fmt.Println("Kaya:", result.ClarificationQuestion)
		return
	}
	if result.DurationSeconds > 0 {
		fmt.Printf("[time +%ds]\n", result.DurationSeconds)
	}
	for _, fact := range result.VisibleFacts {
		fmt.Println("Kaya:", fact.Text)
	}
	for _, event := range result.Events {
		fmt.Println("Kaya:", event.Description)
	}
	if len(result.VisibleFacts) == 0 && !result.NeedsClarification {
		fmt.Println("Kaya: Done.")
	}
}

type playtestScript struct {
	Name             string
	Messages         []string
	Steps            []playtestMessage
	UseInitialKaya   bool
	InitialKaya      kayastate.State
	InitialRoom      game.RoomID
	InitialInventory []game.ItemID
	InitialLight     bool
}

func (s playtestScript) playtestMessages() []playtestMessage {
	if len(s.Steps) > 0 {
		return s.Steps
	}
	messages := make([]playtestMessage, 0, len(s.Messages))
	for _, message := range s.Messages {
		messages = append(messages, playtestMessage{Player: message})
	}
	return messages
}

type playtestMessage struct {
	Player string
	Expect playtestExpectation
}

type playtestExpectation struct {
	Action  intent.Action
	Outcome string
}

type playtestRun struct {
	Model     string
	BaseURL   string
	StartedAt time.Time
	Scripts   []playtestScriptLog
}

type playtestScriptLog struct {
	Name       string
	Steps      []playtestStep
	Suspicious []string
}

type playtestStep struct {
	Number     int
	Player     string
	Expected   playtestExpectation
	Intent     intent.Intent
	Result     game.ActionResult
	Before     playtestWorldState
	After      playtestWorldState
	ParseError string
	Suspicious bool
	Suspicion  string
}

type playtestWorldState struct {
	Room       string
	Time       int
	Inventory  []string
	Discovered []string
	LightOn    bool
	Kaya       kayastate.State
}

func defaultPlaytestScripts() []playtestScript {
	return []playtestScript{
		{
			Name: "reception chaos",
			Messages: []string{
				"hey",
				"whats around you",
				"what can you see",
				"what is on the desk",
				"is there soemthjing in the drawers",
				"search drawers",
				"where is the flashlight",
				"do you have anything",
				"pick up the flashlight",
				"where is the flashlight",
				"do ypou have flashlight",
				"turn it on",
				"turn on the flashlight",
			},
		},
		{
			Name: "dark room without light",
			Messages: []string{
				"go east",
				"what is around you",
				"can you see the body",
				"use your flashlight",
				"go back",
				"search drawers",
				"grab the flashlight from the drawers",
				"go east",
				"use your flashlight",
				"what is around you now",
			},
		},
		{
			Name: "doctor and key variations",
			Messages: []string{
				"search the desk",
				"take flashlight",
				"go east",
				"turn on flashlight",
				"check the doctor",
				"search the doctor near cabinet",
				"take the key",
				"do you have the key",
				"use the key",
				"go north",
			},
		},
		{
			Name: "nonsense and clarification",
			Messages: []string{
				"do it",
				"that thing over there",
				"no wait",
				"hide behind something",
				"throw the moon",
				"where are you",
				"inventory",
			},
		},
		{
			Name: "intent collision phrases",
			Steps: []playtestMessage{
				{
					Player: "what's in the room",
					Expect: playtestExpectation{
						Action:  intent.ActionInspect,
						Outcome: "inspected_room",
					},
				},
				{
					Player: "what's in your bag",
					Expect: playtestExpectation{
						Action:  intent.ActionTalk,
						Outcome: "inventory_empty",
					},
				},
				{
					Player: "what do you have in your inventory",
					Expect: playtestExpectation{
						Action:  intent.ActionTalk,
						Outcome: "inventory_empty",
					},
				},
				{
					Player: "anything useful on you",
					Expect: playtestExpectation{
						Action:  intent.ActionTalk,
						Outcome: "inventory_empty",
					},
				},
				{
					Player: "anything useful around you",
					Expect: playtestExpectation{
						Action:  intent.ActionInspect,
						Outcome: "inspected_room",
					},
				},
				{
					Player: "what do you have in mind",
					Expect: playtestExpectation{
						Action:  intent.ActionTalk,
						Outcome: "talked",
					},
				},
				{
					Player: "do you have anything on you",
					Expect: playtestExpectation{
						Action:  intent.ActionTalk,
						Outcome: "inventory_empty",
					},
				},
				{
					Player: "what's on the desk",
					Expect: playtestExpectation{
						Action:  intent.ActionInspect,
						Outcome: "inspected_object",
					},
				},
			},
		},
		{
			Name:           "autonomy refusal under panic",
			UseInitialKaya: true,
			InitialKaya: kayastate.State{
				Stress: 85,
				Trust:  5,
				Fear:   80,
			},
			Steps: []playtestMessage{
				{
					Player: "go east",
					Expect: playtestExpectation{
						Action:  intent.ActionMove,
						Outcome: "kaya_refused",
					},
				},
			},
		},
		{
			Name:           "autonomy trust asks confirmation",
			UseInitialKaya: true,
			InitialKaya: kayastate.State{
				Stress: 55,
				Trust:  90,
				Fear:   55,
			},
			Steps: []playtestMessage{
				{
					Player: "go east",
					Expect: playtestExpectation{
						Action:  intent.ActionMove,
						Outcome: "kaya_needs_confirmation",
					},
				},
			},
		},
		{
			Name:           "autonomy body search refusal",
			UseInitialKaya: true,
			InitialKaya: kayastate.State{
				Stress: 80,
				Trust:  5,
				Fear:   80,
			},
			InitialRoom:      scenario.RoomStorage,
			InitialInventory: []game.ItemID{scenario.ItemFlashlight},
			InitialLight:     true,
			Steps: []playtestMessage{
				{
					Player: "search the doctor near cabinet",
					Expect: playtestExpectation{
						Action:  intent.ActionSearch,
						Outcome: "kaya_refused",
					},
				},
			},
		},
		{
			Name: "autonomy stress rises after danger",
			Steps: []playtestMessage{
				{
					Player: "go east",
					Expect: playtestExpectation{
						Action:  intent.ActionMove,
						Outcome: "moved",
					},
				},
				{
					Player: "wait",
					Expect: playtestExpectation{
						Action:  intent.ActionWait,
						Outcome: "waited",
					},
				},
			},
		},
	}
}

func addSuspicion(log *playtestScriptLog, step *playtestStep, reason string) {
	step.Suspicious = true
	if step.Suspicion == "" {
		step.Suspicion = reason
	} else {
		step.Suspicion += "; " + reason
	}
	log.Suspicious = append(log.Suspicious, fmt.Sprintf("step %d %q -> %s", step.Number, step.Player, reason))
}

func suspiciousOutcome(parsed intent.Intent, result game.ActionResult) (string, bool) {
	switch result.Outcome {
	case "object_not_found", "object_not_visible", "item_not_found", "item_not_discovered", "item_not_portable", "item_already_taken", "unsupported_action", "unsupported_item", "exit_not_found", "cannot_turn_on", "cannot_turn_off", "missing_item", "needs_clarification":
		return result.Outcome, true
	}
	if parsed.Confidence < 0.5 {
		return fmt.Sprintf("low_confidence %.2f", parsed.Confidence), true
	}
	return "", false
}

func expectationMismatches(expect playtestExpectation, parsed intent.Intent, result game.ActionResult) []string {
	var mismatches []string
	if expect.Action != "" && parsed.Action != expect.Action {
		mismatches = append(mismatches, fmt.Sprintf("expected action %s, got %s", expect.Action, parsed.Action))
	}
	if expect.Outcome != "" && result.Outcome != expect.Outcome {
		mismatches = append(mismatches, fmt.Sprintf("expected outcome %s, got %s", expect.Outcome, result.Outcome))
	}
	return mismatches
}

func writePlaytestLog(run playtestRun) (string, error) {
	if err := os.MkdirAll("playtest_logs", 0o755); err != nil {
		return "", fmt.Errorf("create playtest_logs: %w", err)
	}

	path := filepath.Join("playtest_logs", run.StartedAt.Format("20060102-150405")+".md")
	var b strings.Builder
	b.WriteString("# Kaya Playtest Log\n\n")
	b.WriteString(fmt.Sprintf("- Started: %s\n", run.StartedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Model: `%s`\n", run.Model))
	b.WriteString(fmt.Sprintf("- Ollama URL: `%s`\n\n", run.BaseURL))

	for _, script := range run.Scripts {
		b.WriteString(fmt.Sprintf("## %s\n\n", script.Name))
		if len(script.Suspicious) == 0 {
			b.WriteString("Suspicious outcomes: none\n\n")
		} else {
			b.WriteString("Suspicious outcomes:\n\n")
			for _, note := range script.Suspicious {
				b.WriteString("- ")
				b.WriteString(note)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		for _, step := range script.Steps {
			b.WriteString(fmt.Sprintf("### Step %d\n\n", step.Number))
			b.WriteString(fmt.Sprintf("Player: `%s`\n\n", step.Player))
			b.WriteString(fmt.Sprintf("Before: room=`%s`, time=`%d`, light=`%t`, inventory=`%s`, discovered=`%s`, kaya=`%s`\n\n", step.Before.Room, step.Before.Time, step.Before.LightOn, strings.Join(step.Before.Inventory, ", "), strings.Join(step.Before.Discovered, ", "), formatKayaState(step.Before.Kaya)))
			if !step.Expected.empty() {
				b.WriteString(fmt.Sprintf("Expected: action=`%s`, outcome=`%s`\n\n", step.Expected.Action, step.Expected.Outcome))
			}
			if step.ParseError != "" {
				b.WriteString("Parse error: `")
				b.WriteString(step.ParseError)
				b.WriteString("`\n\n")
				continue
			}
			b.WriteString(fmt.Sprintf("Intent: action=`%s`, target=`%s`, item=`%s`, direction=`%s`, confidence=`%.2f`\n\n", step.Intent.Action, step.Intent.Target, step.Intent.Item, step.Intent.Direction, step.Intent.Confidence))
			b.WriteString(fmt.Sprintf("Outcome: `%s`, duration=`%d`, clarification=`%t`\n\n", step.Result.Outcome, step.Result.DurationSeconds, step.Result.NeedsClarification))
			for _, fact := range step.Result.VisibleFacts {
				b.WriteString("- Kaya: ")
				b.WriteString(fact.Text)
				b.WriteString("\n")
			}
			for _, event := range step.Result.Events {
				b.WriteString("- Event: ")
				b.WriteString(event.Description)
				b.WriteString("\n")
			}
			if step.Suspicious {
				b.WriteString("- Suspicious: `")
				b.WriteString(step.Suspicion)
				b.WriteString("`\n")
			}
			b.WriteString(fmt.Sprintf("\nAfter: room=`%s`, time=`%d`, light=`%t`, inventory=`%s`, discovered=`%s`, kaya=`%s`\n\n", step.After.Room, step.After.Time, step.After.LightOn, strings.Join(step.After.Inventory, ", "), strings.Join(step.After.Discovered, ", "), formatKayaState(step.After.Kaya)))
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write playtest log: %w", err)
	}
	return path, nil
}

func (e playtestExpectation) empty() bool {
	return e.Action == "" && e.Outcome == ""
}

func inventoryNames(state *world.State) []string {
	names := make([]string, 0, len(state.Inventory))
	for itemID := range state.Inventory {
		item, ok := state.Items[itemID]
		if !ok {
			continue
		}
		names = append(names, item.Name)
	}
	sort.Strings(names)
	return names
}

func discoveredItemNames(state *world.State) []string {
	names := make([]string, 0, len(state.DiscoveredItems))
	for itemID := range state.DiscoveredItems {
		item, ok := state.Items[itemID]
		if !ok {
			continue
		}
		names = append(names, item.Name)
	}
	sort.Strings(names)
	return names
}

func formatKayaState(state kayastate.State) string {
	return fmt.Sprintf("stress:%d trust:%d fear:%d pain:%d exhaustion:%d emotion:%s", state.Stress, state.Trust, state.Fear, state.Pain, state.Exhaustion, state.DominantEmotion())
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
