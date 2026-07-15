# Remote Web Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a password-protected, mobile-friendly browser console for remote Kaya playtesting through Ngrok.

**Architecture:** An `internal/webconsole` package owns HTTP routing, authentication, cookies, in-memory browser sessions, transcripts, and the embedded terminal page. `cmd/kaya` provides a factory that creates real generated runs and `session.Session` instances, so HTTP never duplicates game rules.

**Tech Stack:** Go standard library (`net/http`, `crypto/*`), existing Kaya session/runtime packages, embedded HTML/CSS/JavaScript.

## Global Constraints

- `kaya web` defaults to `127.0.0.1:8080`; expose it with `ngrok http 8080`.
- `KAYA_WEB_PASSWORD` is required and must never be logged or returned by any endpoint.
- Cookies are cryptographically random, Secure, HTTP-only, and SameSite=Strict.
- Each authenticated browser owns an isolated, serialized game session; sessions and transcripts are in memory only.
- HTTP turn processing calls `session.Session.ProcessTurn`; it does not implement parser or resolver behavior.
- Every change follows red-green-refactor and the full Go suite passes before handoff.

---

### Task 1: Secure web-session foundation

**Files:**

- Create: `internal/webconsole/server.go`
- Create: `internal/webconsole/server_test.go`

**Interfaces:**

- Produces `type Runtime interface { ProcessTurn(context.Context, string) (session.ProcessedTurn, error) }`.
- Produces `type Game struct { Runtime Runtime; Complete func() bool }` and `type GameFactory func() (Game, error)`.
- Produces `New(Config) (*Server, error)` and `Handler() http.Handler`.

- [ ] **Step 1: Write failing constructor/login tests.**

```go
func TestNewRejectsMissingPassword(t *testing.T) {
	_, err := New(Config{NewGame: newTestGame})
	if err == nil || !strings.Contains(err.Error(), "KAYA_WEB_PASSWORD") {
		t.Fatalf("New error = %v, want missing password error", err)
	}
}

func TestLoginIssuesSecureCookie(t *testing.T) {
	response := postForm(t, newTestServer(t).Handler(), "/login", url.Values{"password": {"test-password"}})
	cookie := response.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %#v", cookie)
	}
}
```

- [ ] **Step 2: Run `go test ./internal/webconsole -run 'Test(NewRejectsMissingPassword|LoginIssuesSecureCookie)'`; expect a package-not-found failure.**

- [ ] **Step 3: Implement the narrow server contract and authentication.**

```go
type Config struct { Password string; NewGame GameFactory; Now func() time.Time; Random io.Reader }
type Server struct { passwordHash [sha256.Size]byte; newGame GameFactory; now func() time.Time; random io.Reader; sessions map[string]*webSession; mu sync.Mutex }

func New(config Config) (*Server, error) {
	if strings.TrimSpace(config.Password) == "" { return nil, errors.New("KAYA_WEB_PASSWORD must be set") }
	if config.NewGame == nil { return nil, errors.New("web game factory must not be nil") }
	// default clock/random source, hash password, initialise map, and register routes
}
```

Use SHA-256 digests plus `subtle.ConstantTimeCompare`. Generate a 32-byte ID with `io.ReadFull`, encode with `base64.RawURLEncoding`, and issue it as a secure cookie.

- [ ] **Step 4: Re-run the focused tests; expect PASS.**
- [ ] **Step 5: Commit with `git add internal/webconsole && git commit -m "feat: add authenticated web console sessions"`.**

### Task 2: Protected game and transcript API

**Files:**

- Modify: `internal/webconsole/server.go`
- Modify: `internal/webconsole/server_test.go`

**Interfaces:**

- Consumes `GameFactory` from Task 1.
- Produces `GET /api/session`, `POST /api/turn`, `POST /api/new-run`, and `POST /logout`.
- Produces `Entry{Role, Text string}` and JSON `{entries, complete}` responses.

- [ ] **Step 1: Write failing handler tests using a fake runtime.**

```go
func TestTurnUsesOnlyTheCallersGameAndStoresTranscript(t *testing.T) {
	clientA, clientB := loggedInClients(t, server.Handler())
	response := clientA.Post("/api/turn", "application/json", strings.NewReader(`{"message":"look around"}`))
	if response.Code != http.StatusOK { t.Fatalf("turn status = %d", response.Code) }
	if got := testGameFor(clientA).messages; !reflect.DeepEqual(got, []string{"look around"}) { t.Fatalf("messages = %#v", got) }
	if got := testGameFor(clientB).messages; len(got) != 0 { t.Fatalf("other browser messages = %#v", got) }
}
```

Add tests for unauthenticated requests, blank/malformed/oversized requests, parser failure, session expiry, logout, new-run isolation, and completed-run rejection.

- [ ] **Step 2: Run `go test ./internal/webconsole -run TestTurnUsesOnlyTheCallersGameAndStoresTranscript`; expect a route failure.**

- [ ] **Step 3: Implement endpoint behavior.**

```go
func (s *Server) handleTurn(w http.ResponseWriter, r *http.Request, active *webSession) {
	var request struct { Message string `json:"message"` }
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.Message) == "" { writeError(w, http.StatusBadRequest, "enter a command"); return }
	active.mu.Lock(); defer active.mu.Unlock()
	if active.complete { writeError(w, http.StatusConflict, "start a new run to continue"); return }
	// append player entry, call Runtime.ProcessTurn, append response or safe signal-break text
}
```

Store the CLI connection/greeting entries when a game starts. Purge sessions idle for 12 hours, lock each session during turn/reset work, and return only the caller's transcript.

- [ ] **Step 4: Run `go test ./internal/webconsole`; expect PASS.**
- [ ] **Step 5: Commit with `git add internal/webconsole && git commit -m "feat: add web console game endpoints"`.**

### Task 3: Responsive terminal document

**Files:**

- Create: `internal/webconsole/page.go`
- Modify: `internal/webconsole/server.go`
- Modify: `internal/webconsole/server_test.go`

**Interfaces:**

- Consumes the API from Task 2.
- Produces login and terminal HTML at `GET /` and browser calls to each documented endpoint.

- [ ] **Step 1: Write a failing page-content test.**

```go
func TestTerminalPageProvidesMobileConsoleControls(t *testing.T) {
	response := authenticatedRequest(t, server.Handler(), http.MethodGet, "/", nil)
	for _, want := range []string{`name="viewport"`, `id="transcript"`, `id="message"`, `id="send"`, `New Run`, `Logout`} {
		if !strings.Contains(response.Body.String(), want) { t.Fatalf("page missing %q", want) }
	}
}
```

- [ ] **Step 2: Run `go test ./internal/webconsole -run TestTerminalPageProvidesMobileConsoleControls`; expect FAIL.**
- [ ] **Step 3: Add self-contained login and terminal documents.**

```html
<meta name="viewport" content="width=device-width, initial-scale=1">
<main class="console"><header>…</header><section id="transcript" aria-live="polite"></section>
<form id="command-form"><input id="message" maxlength="2000" autocomplete="off"><button id="send">Send</button></form></main>
```

Render transcript entries with `document.createElement` and `.textContent`, never `innerHTML`; disable the form in-flight, scroll to new output, confirm before New Run, and return to login on 401.

- [ ] **Step 4: Run `go test ./internal/webconsole`; expect PASS.**
- [ ] **Step 5: Commit with `git add internal/webconsole && git commit -m "feat: add mobile web terminal interface"`.**

### Task 4: Integrate the real `kaya web` command

**Files:**

- Modify: `cmd/kaya/main.go`
- Modify: `cmd/kaya/main_test.go`

**Interfaces:**

- Consumes `webconsole.New(webconsole.Config{Password, NewGame})`.
- Produces `runWeb(args []string) error`, `parseWebOptions(args []string) (webOptions, error)`, and a real-game factory.

- [ ] **Step 1: Write failing option tests.**

```go
func TestParseWebOptionsDefaultsToLoopback(t *testing.T) {
	options, err := parseWebOptions(nil)
	if err != nil || options.Address != "127.0.0.1:8080" { t.Fatalf("options/error = %#v / %v", options, err) }
}

func TestParseWebOptionsRejectsPositionals(t *testing.T) {
	if _, err := parseWebOptions([]string{"extra"}); err == nil { t.Fatal("expected usage error") }
}
```

- [ ] **Step 2: Run `go test ./cmd/kaya -run TestParseWebOptions`; expect FAIL.**
- [ ] **Step 3: Dispatch `web`, parse `--addr`, check `KAYA_WEB_PASSWORD`, build one Ollama client, and give webconsole a factory.**

```go
return webconsole.Game{
	Runtime: session.New(generated.State, parser, composer),
	Complete: func() bool { return generated.State.CurrentRoomID == scenario.RoomStairwell },
}, nil
```

Generate a fresh random seed for every factory call. Start an `http.Server`, update usage text, and print the equivalent Ngrok command without ever printing the password.

- [ ] **Step 4: Run `go test ./cmd/kaya -run TestParseWebOptions` then `go test ./...`; expect PASS.**
- [ ] **Step 5: Commit integration and plan with `git add cmd/kaya/main.go cmd/kaya/main_test.go docs/superpowers/plans/2026-07-15-remote-web-console.md && git commit -m "feat: expose Kaya through web console"`.**

### Task 5: Format and verify

**Files:**

- Modify only to correct verification findings.

- [ ] **Step 1: Run `gofmt -w cmd/kaya/main.go cmd/kaya/main_test.go internal/webconsole/*.go`.**
- [ ] **Step 2: Run `go test ./...`, `go test -race ./...`, and `go vet ./...`; expect all PASS.**
- [ ] **Step 3: Start `KAYA_WEB_PASSWORD=temporary-local-test-password go run ./cmd/kaya web --addr 127.0.0.1:8080`, then verify login, turn, reset, and logout locally before sharing it with Ngrok.**
- [ ] **Step 4: In the final handoff, give the exact Windows startup commands and identify any manual validation blocked by unavailable Ollama or Ngrok.**
