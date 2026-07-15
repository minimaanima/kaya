package main

import (
	"bufio"
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"kaya/internal/game"
	"kaya/internal/intent"
	kayastate "kaya/internal/kaya"
	"kaya/internal/llm"
	"kaya/internal/playtest"
	"kaya/internal/response"
	"kaya/internal/rungen"
	"kaya/internal/runscenario"
	"kaya/internal/scenario"
	"kaya/internal/session"
	"kaya/internal/turn"
	"kaya/internal/webconsole"
	"kaya/internal/world"
)

const (
	defaultOllamaModel   = "qwen3.5:4b"
	defaultWebAddress    = "127.0.0.1:8080"
	webUsage             = "usage: kaya web [--addr <host:port>]"
	webReadHeaderTimeout = 10 * time.Second
	webReadTimeout       = 15 * time.Second
	webIdleTimeout       = 60 * time.Second
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: kaya <intent|play|playtest|web>")
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
		if err := runPlaytest(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "web":
		if err := runWeb(os.Args[2:]); err != nil {
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
	state := scenario.NewPrototypeWorld()
	snapshot, err := state.PerceptionSnapshot()
	if err != nil {
		return fmt.Errorf("snapshot world: %w", err)
	}
	parsed, err := parser.Parse(context.Background(), message, snapshot)
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
	composer := response.NewComposer(client)

	printRunDebug(os.Stdout, run)
	fmt.Println("Connection established.")
	fmt.Println("Kaya: I can read you. I am in reception. The ceiling is cracked, but I can move.")
	fmt.Println("Type naturally. Use 'quit' or 'exit' to stop.")
	return runPlayConsole(context.Background(), os.Stdin, os.Stdout, state, parser, composer, options.ParseLog || parseLogEnabled())
}

func runWeb(args []string) error {
	return runWebWithEnvironment(args, os.Getenv)
}

func runWebWithEnvironment(args []string, lookupEnvironment func(string) string) error {
	options, err := parseWebOptions(args)
	if err != nil {
		return err
	}

	password := lookupEnvironment("KAYA_WEB_PASSWORD")
	if strings.TrimSpace(password) == "" {
		return errors.New("KAYA_WEB_PASSWORD must be set")
	}

	model := envOrDefaultWithEnvironment(lookupEnvironment, "KAYA_OLLAMA_MODEL", defaultOllamaModel)
	baseURL := envOrDefaultWithEnvironment(lookupEnvironment, "KAYA_OLLAMA_URL", llm.DefaultOllamaURL)
	client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
	if err != nil {
		return err
	}

	console, err := webconsole.New(webconsole.Config{
		Password: password,
		NewGame:  newWebGameFactory(intent.NewParser(client), response.NewComposer(client), newRunSeed),
	})
	if err != nil {
		return err
	}

	httpServer := newWebHTTPServer(options, console.Handler())
	writeWebStartup(os.Stdout, options)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newWebHTTPServer(options webOptions, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              options.Address,
		Handler:           handler,
		ReadHeaderTimeout: webReadHeaderTimeout,
		ReadTimeout:       webReadTimeout,
		IdleTimeout:       webIdleTimeout,
	}
}

func writeWebStartup(output io.Writer, options webOptions) {
	fmt.Fprintf(output, "Kaya web console is listening locally at http://%s (Ngrok backend only).\n", options.Address)
	fmt.Fprintf(output, "Expose it with: ngrok http %s\n", options.Port)
	fmt.Fprintln(output, "On your phone, open the HTTPS Ngrok URL that command provides.")
}

func runPlayConsole(ctx context.Context, input io.Reader, output io.Writer, state *world.State, parser session.SemanticParser, composer session.Composer, parseLog bool) error {
	runtime := session.New(state, parser, composer)
	scanner := bufio.NewScanner(input)

	for {
		fmt.Fprint(output, "> ")
		if !scanner.Scan() {
			break
		}

		message := strings.TrimSpace(scanner.Text())
		if message == "" {
			continue
		}
		if message == "quit" || message == "exit" {
			fmt.Fprintln(output, "Connection closed.")
			return nil
		}

		processed, err := runtime.ProcessTurn(ctx, message)
		if err != nil {
			fmt.Fprintln(output, "Kaya: The signal broke up. I did not understand that.")
			fmt.Fprintln(output, "debug:", err)
			continue
		}
		if parseLog {
			fmt.Fprintln(output, formatParseLog(processed))
		}
		if elapsed := resultDuration(processed.Result); elapsed > 0 {
			fmt.Fprintf(output, "[time +%ds]\n", elapsed)
		}
		fmt.Fprintln(output, "Kaya:", processed.Response.Text)
		if processed.Response.UsedFallback && debugOutputEnabled() {
			fmt.Fprintln(output, "debug:", processed.Response.FallbackReason)
		}
		if state.CurrentRoomID == scenario.RoomStairwell {
			fmt.Fprintln(output, "Kaya: I am in the stairwell. This part is clear.")
			fmt.Fprintln(output, "Prototype objective complete.")
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	return nil
}

type turnParser interface {
	Parse(context.Context, string, game.PerceptionSnapshot) (intent.TurnPlan, error)
}

type responseComposer interface {
	Compose(context.Context, turn.FactBundle) response.Response
}

type processedTurn = session.ProcessedTurn

func processPlayerTurn(ctx context.Context, message string, state *world.State, parser turnParser, _ turn.Executor, composer responseComposer) (processedTurn, error) {
	adapter := provenanceParser{parser: parser}
	return session.ProcessTurn(ctx, message, state, adapter, composer)
}

type provenanceParser struct{ parser turnParser }

func (p provenanceParser) ParseWithProvenance(ctx context.Context, message string, snapshot game.PerceptionSnapshot) (intent.TurnPlan, intent.ParseProvenance, error) {
	if parser, ok := p.parser.(interface {
		ParseWithProvenance(context.Context, string, game.PerceptionSnapshot) (intent.TurnPlan, intent.ParseProvenance, error)
	}); ok {
		return parser.ParseWithProvenance(ctx, message, snapshot)
	}
	plan, err := p.parser.Parse(ctx, message, snapshot)
	return plan, intent.ParseProvenance{}, err
}

func resultDuration(result turn.Result) int {
	return session.ResultDuration(result)
}

func runPlaytest(args []string) error {
	return runPlaytestWith(args, newRunSeed, newPrototypePlaytestExecutor, writePlaytestLog, os.Stdout)
}

type playtestExecutor interface {
	Run(context.Context) error
	Session() playtest.Session
}

type prototypePlaytestExecutor struct {
	runner    *playtest.Runner
	generated rungen.GeneratedRun
	seed      int64
}

func (e prototypePlaytestExecutor) Run(ctx context.Context) error {
	return playtest.RunPrototypeSession(ctx, e.runner, e.generated, e.seed)
}

func (e prototypePlaytestExecutor) Session() playtest.Session {
	return e.runner.Session()
}

func runPlaytestWith(
	args []string,
	generateSeed func() (int64, error),
	start func(playOptions) (playtestExecutor, error),
	write func(playtest.Session) (string, error),
	output io.Writer,
) error {
	options, err := parsePlaytestOptions(args, generateSeed)
	if err != nil {
		return err
	}
	executor, err := start(options)
	if err != nil {
		return err
	}
	runErr := executor.Run(context.Background())
	logPath, logErr := write(executor.Session())
	if logErr != nil {
		return logErr
	}
	fmt.Fprintln(output, "Playtest log:", logPath)
	return runErr
}

func newPrototypePlaytestExecutor(options playOptions) (playtestExecutor, error) {
	definition := runscenario.PrototypeDefinition()
	generated, err := rungen.Generate(
		rungen.RunConfig{Seed: options.Seed, GeneratorVersion: rungen.CurrentGeneratorVersion},
		definition,
	)
	if err != nil {
		return nil, fmt.Errorf("generate playtest run: %w", err)
	}

	var runner *playtest.Runner
	if envBool("KAYA_PLAYTEST_OLLAMA") {
		model := envOrDefault("KAYA_OLLAMA_MODEL", defaultOllamaModel)
		baseURL := envOrDefault("KAYA_OLLAMA_URL", llm.DefaultOllamaURL)
		client, err := llm.NewOllamaClient(model, llm.WithOllamaBaseURL(baseURL))
		if err != nil {
			return nil, err
		}
		runner = playtest.NewRunner(definition, generated, intent.NewParser(client), response.NewComposer(client))
	} else {
		runner = playtest.NewOfflineRunner(definition, generated)
	}
	return prototypePlaytestExecutor{runner: runner, generated: generated, seed: options.Seed}, nil
}

type playOptions struct {
	Seed     int64
	ParseLog bool
}

type webOptions struct {
	Address string
	Port    string
}

func parseWebOptions(args []string) (webOptions, error) {
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	address := flags.String("addr", defaultWebAddress, "web console address")
	if err := flags.Parse(args); err != nil {
		return webOptions{}, fmt.Errorf("%s: %w", webUsage, err)
	}
	if flags.NArg() != 0 {
		return webOptions{}, fmt.Errorf("%s", webUsage)
	}
	_, port, err := net.SplitHostPort(*address)
	if err != nil || port == "" {
		return webOptions{}, fmt.Errorf("%s: invalid --addr %q", webUsage, *address)
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return webOptions{}, fmt.Errorf("%s: invalid --addr %q", webUsage, *address)
	}
	if portNumber == 0 {
		return webOptions{}, fmt.Errorf("%s: invalid --addr %q: port must not be 0", webUsage, *address)
	}
	return webOptions{Address: *address, Port: port}, nil
}

func newWebGameFactory(
	parser session.SemanticParser,
	composer session.Composer,
	generateSeed func() (int64, error),
) webconsole.GameFactory {
	return func() (webconsole.Game, error) {
		seed, err := generateSeed()
		if err != nil {
			return webconsole.Game{}, fmt.Errorf("generate web run seed: %w", err)
		}
		generated, err := rungen.Generate(
			rungen.RunConfig{
				Seed:             seed,
				GeneratorVersion: rungen.CurrentGeneratorVersion,
			},
			runscenario.PrototypeDefinition(),
		)
		if err != nil {
			return webconsole.Game{}, fmt.Errorf("generate web run: %w", err)
		}

		state := generated.State
		return webconsole.Game{
			Runtime: session.New(state, parser, composer),
			Complete: func() bool {
				return state.CurrentRoomID == scenario.RoomStairwell
			},
		}, nil
	}
}

func parsePlayOptions(args []string, generateSeed func() (int64, error)) (playOptions, error) {
	flags := flag.NewFlagSet("play", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	seed := flags.Int64("seed", 0, "reproducible run seed")
	parseLog := flags.Bool("parse-log", false, "print parsed turn plans during play")
	if err := flags.Parse(args); err != nil {
		return playOptions{}, err
	}
	if flags.NArg() != 0 {
		return playOptions{}, fmt.Errorf("usage: kaya play [--seed <int64>] [--parse-log]")
	}

	provided := false
	flags.Visit(func(current *flag.Flag) {
		if current.Name == "seed" {
			provided = true
		}
	})
	if provided {
		return playOptions{Seed: *seed, ParseLog: *parseLog}, nil
	}

	generated, err := generateSeed()
	if err != nil {
		return playOptions{}, err
	}
	return playOptions{Seed: generated, ParseLog: *parseLog}, nil
}

func parsePlaytestOptions(args []string, generateSeed func() (int64, error)) (playOptions, error) {
	return parsePlayOptions(args, generateSeed)
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

func runPlaytestScript(parser turnParser, script playtestScript, composers ...responseComposer) (playtestScriptLog, error) {
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
	if err := state.ObserveRoom(state.CurrentRoomID, state.PreviousRoomID); err != nil {
		return playtestScriptLog{}, fmt.Errorf("observe initial room %q: %w", state.CurrentRoomID, err)
	}

	executor := turn.NewExecutor(state)
	var composer responseComposer = response.NewComposer(nil)
	if len(composers) > 0 && composers[0] != nil {
		composer = composers[0]
	}
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

		processed, err := processPlayerTurn(context.Background(), message, state, parser, executor, composer)
		if err != nil {
			step.ParseError = err.Error()
			step.Suspicious = true
			step.Suspicion = "parse_error"
			log.Steps = append(log.Steps, step)
			log.Suspicious = append(log.Suspicious, fmt.Sprintf("step %d parse error for %q: %v", step.Number, message, err))
			continue
		}

		step.Plan = processed.Plan
		step.Result = processed.Result
		step.Response = processed.Response
		step.After = playtestWorldState{
			Room:       string(state.CurrentRoomID),
			Time:       state.NowSeconds,
			Inventory:  inventoryNames(state),
			Discovered: discoveredItemNames(state),
			LightOn:    state.ActiveLight,
			Kaya:       state.Kaya,
		}

		if reason, ok := suspiciousOutcome(processed.Plan, processed.Result, processed.Response); ok {
			addSuspicion(&log, &step, reason)
		}
		for _, reason := range expectationMismatches(planned.Expect, processed.Plan, processed.Result) {
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
	FirstAction     intent.Action
	FirstOutcome    string
	OutcomeCount    int
	QuestionKind    game.FactKind
	QuestionCount   int
	RequireFactText string
	ForbidFactText  string
	// Action and Outcome are kept for compatibility with older local scripts.
	Action  intent.Action
	Outcome string
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
	Plan       intent.TurnPlan
	Result     turn.Result
	Response   response.Response
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
			Name: "user regression fixed seed",
			Steps: []playtestMessage{
				{Player: "what is around you", Expect: playtestExpectation{FirstAction: intent.ActionInspect, FirstOutcome: "inspected_room", OutcomeCount: 1}},
				{Player: "what is on the desk", Expect: playtestExpectation{FirstAction: intent.ActionInspect, FirstOutcome: "inspected_object", OutcomeCount: 1}},
				{Player: "look inside the drawers", Expect: playtestExpectation{FirstAction: intent.ActionSearch, FirstOutcome: "searched_found_items", OutcomeCount: 1}},
				{Player: "take the flashlight", Expect: playtestExpectation{FirstAction: intent.ActionTakeItem, FirstOutcome: "item_taken", OutcomeCount: 1}},
				{Player: "go east", Expect: playtestExpectation{FirstAction: intent.ActionMove, FirstOutcome: "moved", OutcomeCount: 1}},
				{Player: "whats around you", Expect: playtestExpectation{FirstAction: intent.ActionInspect, FirstOutcome: "inspected_room", OutcomeCount: 1, ForbidFactText: "north"}},
				{Player: "turn on the flashlight", Expect: playtestExpectation{FirstAction: intent.ActionTurnOn, FirstOutcome: "flashlight_on", OutcomeCount: 1}},
				{Player: "look around", Expect: playtestExpectation{FirstAction: intent.ActionInspect, FirstOutcome: "inspected_room", OutcomeCount: 1, RequireFactText: "north"}},
				{Player: "search the doctors are they dead", Expect: playtestExpectation{FirstAction: intent.ActionSearch, FirstOutcome: "searched_found_items", OutcomeCount: 2, QuestionKind: game.FactLifeStatus, QuestionCount: 2}},
			},
		},
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

func suspiciousOutcome(plan intent.TurnPlan, result turn.Result, composed response.Response) (string, bool) {
	if plan.NeedsClarification || result.StopReason == "clarification" {
		return "parser_clarification", true
	}
	for _, outcome := range result.Outcomes {
		if outcome.Result.Status == game.ActionFailed || outcome.Result.Status == game.ActionRefused {
			return outcome.Result.Outcome, true
		}
	}
	if composed.UsedFallback {
		return "response_fallback: " + composed.FallbackReason, true
	}
	if len(plan.Actions) > 0 && plan.Actions[0].Intent.Confidence < 0.5 {
		return fmt.Sprintf("low_confidence %.2f", plan.Actions[0].Intent.Confidence), true
	}
	return "", false
}

func expectationMismatches(expect playtestExpectation, plan intent.TurnPlan, result turn.Result) []string {
	var mismatches []string
	action := expect.FirstAction
	if action == "" {
		action = expect.Action
	}
	if action != "" {
		if len(plan.Actions) == 0 {
			mismatches = append(mismatches, fmt.Sprintf("expected action %s, got none", action))
		} else if got := plan.Actions[0].Intent.Action; got != action {
			mismatches = append(mismatches, fmt.Sprintf("expected action %s, got %s", action, got))
		}
	}
	outcome := expect.FirstOutcome
	if outcome == "" {
		outcome = expect.Outcome
	}
	if outcome != "" {
		if len(result.Outcomes) == 0 {
			mismatches = append(mismatches, fmt.Sprintf("expected outcome %s, got none", outcome))
		} else if got := result.Outcomes[0].Result.Outcome; got != outcome {
			mismatches = append(mismatches, fmt.Sprintf("expected outcome %s, got %s", outcome, got))
		}
	}
	if expect.OutcomeCount > 0 && len(result.Outcomes) != expect.OutcomeCount {
		mismatches = append(mismatches, fmt.Sprintf("expected %d outcomes, got %d", expect.OutcomeCount, len(result.Outcomes)))
	}
	if expect.QuestionKind != "" {
		count := 0
		for _, fact := range result.QuestionFacts {
			if fact.Kind == expect.QuestionKind {
				count++
			}
		}
		if count != expect.QuestionCount {
			mismatches = append(mismatches, fmt.Sprintf("expected %d %s facts, got %d", expect.QuestionCount, expect.QuestionKind, count))
		}
	}
	bundle := result.FactBundle("")
	factText := make([]string, 0, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		factText = append(factText, fact.Text)
	}
	joined := strings.ToLower(strings.Join(factText, " "))
	if required := strings.TrimSpace(expect.RequireFactText); required != "" && !strings.Contains(joined, strings.ToLower(required)) {
		mismatches = append(mismatches, fmt.Sprintf("required fact text %q missing", required))
	}
	if forbidden := strings.TrimSpace(expect.ForbidFactText); forbidden != "" && strings.Contains(joined, strings.ToLower(forbidden)) {
		mismatches = append(mismatches, fmt.Sprintf("forbidden fact text %q present", forbidden))
	}
	return mismatches
}

func playtestLogPath(seed int64) string {
	return filepath.Join("playtest_logs", fmt.Sprintf("seed-%d.md", seed))
}

// writePlaytestLog remains the CLI compatibility wrapper for canonical transcripts.
func writePlaytestLog(session playtest.Session) (string, error) {
	if err := os.MkdirAll("playtest_logs", 0o755); err != nil {
		return "", fmt.Errorf("create playtest_logs: %w", err)
	}

	path := playtestLogPath(session.Seed)
	if err := os.WriteFile(path, []byte(playtest.RenderMarkdown(session)), 0o644); err != nil {
		return "", fmt.Errorf("write playtest log: %w", err)
	}
	return path, nil
}

func (e playtestExpectation) empty() bool {
	return e.FirstAction == "" && e.FirstOutcome == "" && e.Action == "" && e.Outcome == "" && e.OutcomeCount == 0 && e.QuestionKind == "" && e.QuestionCount == 0 && e.RequireFactText == "" && e.ForbidFactText == ""
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
	return envOrDefaultWithEnvironment(os.Getenv, name, fallback)
}

func envOrDefaultWithEnvironment(lookupEnvironment func(string) string, name string, fallback string) string {
	value := strings.TrimSpace(lookupEnvironment(name))
	if value == "" {
		return fallback
	}
	return value
}

func debugOutputEnabled() bool {
	return envBool("KAYA_DEBUG")
}

func parseLogEnabled() bool {
	return envBool("KAYA_PARSE_LOG") || debugOutputEnabled()
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func formatParseLog(processed session.ProcessedTurn) string {
	plan := processed.SemanticPlan
	provenance := processed.SemanticProvenance
	var b strings.Builder
	b.WriteString(fmt.Sprintf("parse: source=%s", provenance.Source))
	if plan.NeedsClarification {
		b.WriteString(" clarification=true")
		if strings.TrimSpace(plan.ClarificationQuestion) != "" {
			b.WriteString(" question=")
			b.WriteString(strconvQuote(plan.ClarificationQuestion))
		}
	}
	if len(plan.Actions) == 0 {
		b.WriteString(" actions=0")
	} else {
		b.WriteString(fmt.Sprintf(" actions=%d", len(plan.Actions)))
		for i, action := range plan.Actions {
			b.WriteString(fmt.Sprintf(" | %d:%s%s evidence=%s", i+1, action.ActionKind(), formatSemanticActionFields(action), strconvQuote(action.SourceEvidence())))
		}
	}
	if len(plan.Questions) > 0 {
		b.WriteString(fmt.Sprintf(" questions=%d", len(plan.Questions)))
	}
	for i, outcome := range processed.Result.Outcomes {
		b.WriteString(fmt.Sprintf(" | grounded %d: action=%s grounded_item_id=%s grounded_target_id=%s grounded_object=%s grounded_ids=[%s]", i+1, outcome.Intent.Action, strconvQuote(outcome.Intent.Item), strconvQuote(outcome.Intent.Target), strconvQuote(string(outcome.TargetObjectID)), joinObjectIDs(outcome.Result.TargetObjectIDs)))
	}
	if processed.Pending != nil {
		ids := make([]string, len(processed.Pending.Candidates))
		for i, candidate := range processed.Pending.Candidates {
			ids[i] = candidate.ID
		}
		b.WriteString(fmt.Sprintf(" | pending: action=%d role=%s candidates=[%s]", processed.Pending.ActionIndex, processed.Pending.Role, strings.Join(ids, ",")))
	}
	appendParseJSON(&b, "raw_dto", provenance.RawPlan, provenance.HasRawPlan)
	appendParseJSON(&b, "initial_raw_dto", provenance.InitialRawPlan, provenance.HasInitialRawPlan)
	appendParseJSON(&b, "validation_errors", provenance.ValidationErrors, len(provenance.ValidationErrors) > 0)
	appendParseJSON(&b, "initial_validation_errors", provenance.InitialValidationErrors, len(provenance.InitialValidationErrors) > 0)
	if provenance.RepairReason != nil {
		b.WriteString(" | repair_reason=")
		b.WriteString(strconvQuote(provenance.RepairReason.Error()))
	}
	if provenance.FallbackError != nil {
		b.WriteString(" | fallback_error=")
		b.WriteString(strconvQuote(provenance.FallbackError.Error()))
	}
	return b.String()
}

func appendParseJSON(b *strings.Builder, name string, value any, present bool) {
	if !present {
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	b.WriteString(" | ")
	b.WriteString(name)
	b.WriteByte('=')
	b.Write(encoded)
}

func formatSemanticActionFields(action intent.SemanticAction) string {
	switch typed := action.(type) {
	case intent.MoveAction:
		return " direction=" + strconvQuote(typed.Direction)
	case intent.InspectAction:
		return formatSemanticReference(typed.Target)
	case intent.SearchAction:
		return formatSemanticReference(typed.Target)
	case intent.TakeAction:
		return formatSemanticReference(typed.Target)
	case intent.UseAction:
		return " item_" + strings.TrimPrefix(formatSemanticReference(typed.Item), " ") + formatSemanticReference(typed.Target)
	case intent.ToggleAction:
		return " item_" + strings.TrimPrefix(formatSemanticReference(typed.Item), " ") + " state=" + strconvQuote(typed.State)
	case intent.TalkAction:
		return formatSemanticReference(typed.Target)
	case intent.ListenAction:
		return formatSemanticReference(typed.Target)
	case intent.ExploreAction:
		return formatSemanticReference(typed.Target)
	default:
		return ""
	}
}

func formatSemanticReference(reference intent.Reference) string {
	return " mention=" + strconvQuote(reference.Mention) + " quantity=" + strconvQuote(string(reference.Quantity))
}

func joinObjectIDs(ids []game.ObjectID) string {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = string(id)
	}
	return strings.Join(values, ",")
}

func strconvQuote(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return string(encoded)
}
