package webconsole

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"kaya/internal/response"
	"kaya/internal/session"
)

func TestNewRejectsMissingPassword(t *testing.T) {
	_, err := New(Config{NewGame: newTestGame})
	if err == nil || !strings.Contains(err.Error(), "KAYA_WEB_PASSWORD") {
		t.Fatalf("New error = %v, want missing password error", err)
	}
}

func TestLoginIssuesSecureCookie(t *testing.T) {
	response := postForm(t, newTestServer(t).Handler(), "/login", url.Values{"password": {"test-password"}})
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v, want one session cookie", cookies)
	}
	cookie := cookies[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %#v", cookie)
	}
}

func TestSessionStartsWithConnectionAndGreeting(t *testing.T) {
	server, _ := newFakeServer(t)
	browser := login(t, server.Handler())

	response := browser.request(t, server.Handler(), http.MethodGet, "/api/session", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("session status = %d, want %d", response.Code, http.StatusOK)
	}
	state := decodeState(t, response)
	want := []transcriptEntry{
		{Role: "system", Text: "Connection established."},
		{Role: "kaya", Text: "I can read you. I am in reception. The ceiling is cracked, but I can move."},
	}
	if fmt.Sprint(state.Entries) != fmt.Sprint(want) {
		t.Fatalf("entries = %#v, want %#v", state.Entries, want)
	}
	if state.Complete {
		t.Fatal("new session is complete")
	}
}

func TestTurnUsesOnlyTheCallersGameAndStoresTranscript(t *testing.T) {
	server, games := newFakeServer(t)
	clientA := login(t, server.Handler())
	clientB := login(t, server.Handler())
	games.runtimes[0].duration = 5

	response := clientA.request(t, server.Handler(), http.MethodPost, "/api/turn", "application/json", strings.NewReader(`{"message":" look around "}`))
	if response.Code != http.StatusOK {
		t.Fatalf("turn status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := games.runtimes[0].messages; fmt.Sprint(got) != "[look around]" {
		t.Fatalf("first game messages = %#v, want [look around]", got)
	}
	if got := games.runtimes[1].messages; len(got) != 0 {
		t.Fatalf("second game messages = %#v, want none", got)
	}
	otherState := decodeState(t, clientB.request(t, server.Handler(), http.MethodGet, "/api/session", "", nil))
	if len(otherState.Entries) != 2 {
		t.Fatalf("second game entries = %#v, want initial transcript only", otherState.Entries)
	}

	state := decodeState(t, response)
	want := []transcriptEntry{
		{Role: "system", Text: "Connection established."},
		{Role: "kaya", Text: "I can read you. I am in reception. The ceiling is cracked, but I can move."},
		{Role: "player", Text: "look around"},
		{Role: "system", Text: "[time +5s]"},
		{Role: "kaya", Text: "reply: look around"},
	}
	if fmt.Sprint(state.Entries) != fmt.Sprint(want) {
		t.Fatalf("entries = %#v, want %#v", state.Entries, want)
	}
}

func TestProtectedEndpointsRejectUnauthenticatedAndInvalidCookies(t *testing.T) {
	server, _ := newFakeServer(t)
	for _, test := range []struct {
		name, method, path string
	}{
		{name: "session", method: http.MethodGet, path: "/api/session"},
		{name: "turn", method: http.MethodPost, path: "/api/turn"},
		{name: "new run", method: http.MethodPost, path: "/api/new-run"},
		{name: "logout", method: http.MethodPost, path: "/logout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if test.name == "turn" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-session"})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid-cookie status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestTurnRejectsInvalidJSONRequests(t *testing.T) {
	server, games := newFakeServer(t)
	browser := login(t, server.Handler())
	for _, test := range []struct {
		name, contentType, body string
	}{
		{name: "wrong content type", contentType: "text/plain", body: `{"message":"look around"}`},
		{name: "blank message", contentType: "application/json", body: `{"message":"  \n\t "}`},
		{name: "malformed json", contentType: "application/json", body: `{"message":`},
		{name: "unknown field", contentType: "application/json", body: `{"message":"look around","extra":true}`},
		{name: "trailing data", contentType: "application/json", body: `{"message":"look around"} {}`},
		{name: "message too long", contentType: "application/json", body: `{"message":"` + strings.Repeat("x", 2001) + `"}`},
		{name: "body too large", contentType: "application/json", body: `{"message":"` + strings.Repeat("x", 5000) + `"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := browser.request(t, server.Handler(), http.MethodPost, "/api/turn", test.contentType, strings.NewReader(test.body))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
			}
		})
	}
	if got := games.runtimes[0].messages; len(got) != 0 {
		t.Fatalf("runtime messages = %#v, want none", got)
	}
}

func TestTurnHidesRuntimeErrorsAndKeepsSessionUsable(t *testing.T) {
	server, games := newFakeServer(t)
	browser := login(t, server.Handler())
	games.runtimes[0].err = errors.New("parser network address leaked")

	response := browser.request(t, server.Handler(), http.MethodPost, "/api/turn", "application/json", strings.NewReader(`{"message":"look around"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("turn status = %d, want %d", response.Code, http.StatusOK)
	}
	if strings.Contains(response.Body.String(), "parser network address leaked") {
		t.Fatalf("response exposed runtime error: %s", response.Body.String())
	}
	state := decodeState(t, response)
	got := state.Entries[len(state.Entries)-1]
	want := transcriptEntry{Role: "kaya", Text: "The signal broke up. I did not understand that."}
	if got != want {
		t.Fatalf("last entry = %#v, want %#v", got, want)
	}
}

func TestSessionExpiresAtTwelveHoursOfInactivity(t *testing.T) {
	server, games := newFakeServer(t)
	browser := login(t, server.Handler())
	games.now = games.now.Add(12 * time.Hour)

	response := browser.request(t, server.Handler(), http.MethodGet, "/api/session", "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expired session status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.sessions) != 0 {
		t.Fatalf("sessions = %#v, want expired session pruned", server.sessions)
	}
}

func TestNewRunReplacesOnlyTheCallersGameAndTranscript(t *testing.T) {
	server, games := newFakeServer(t)
	clientA := login(t, server.Handler())
	clientB := login(t, server.Handler())
	turn := clientA.request(t, server.Handler(), http.MethodPost, "/api/turn", "application/json", strings.NewReader(`{"message":"look around"}`))
	if turn.Code != http.StatusOK {
		t.Fatalf("turn status = %d", turn.Code)
	}

	response := clientA.request(t, server.Handler(), http.MethodPost, "/api/new-run", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("new-run status = %d, want %d", response.Code, http.StatusOK)
	}
	if len(games.runtimes) != 3 {
		t.Fatalf("games created = %d, want 3", len(games.runtimes))
	}
	stateA := decodeState(t, response)
	if len(stateA.Entries) != 2 {
		t.Fatalf("new game entries = %#v, want initial transcript only", stateA.Entries)
	}
	stateB := decodeState(t, clientB.request(t, server.Handler(), http.MethodGet, "/api/session", "", nil))
	if len(stateB.Entries) != 2 {
		t.Fatalf("other game entries = %#v, want untouched initial transcript", stateB.Entries)
	}
	if got := games.runtimes[0].messages; fmt.Sprint(got) != "[look around]" {
		t.Fatalf("first game messages = %#v", got)
	}
	if got := games.runtimes[2].messages; len(got) != 0 {
		t.Fatalf("replacement game messages = %#v, want none", got)
	}
}

func TestCompletedRunRejectsTurnsUntilNewRun(t *testing.T) {
	server, games := newFakeServer(t)
	browser := login(t, server.Handler())

	completed := browser.request(t, server.Handler(), http.MethodPost, "/api/turn", "application/json", strings.NewReader(`{"message":"finish"}`))
	if completed.Code != http.StatusOK {
		t.Fatalf("completion status = %d, want %d", completed.Code, http.StatusOK)
	}
	state := decodeState(t, completed)
	if !state.Complete {
		t.Fatal("completed state = false, want true")
	}
	if got, want := state.Entries[len(state.Entries)-2:], []transcriptEntry{
		{Role: "kaya", Text: "I am in the stairwell. This part is clear."},
		{Role: "system", Text: "Prototype objective complete."},
	}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("completion entries = %#v, want %#v", got, want)
	}

	blocked := browser.request(t, server.Handler(), http.MethodPost, "/api/turn", "application/json", strings.NewReader(`{"message":"look around"}`))
	if blocked.Code != http.StatusConflict {
		t.Fatalf("blocked-turn status = %d, want %d", blocked.Code, http.StatusConflict)
	}
	if got := games.runtimes[0].messages; fmt.Sprint(got) != "[finish]" {
		t.Fatalf("completed runtime messages = %#v, want [finish]", got)
	}

	reset := browser.request(t, server.Handler(), http.MethodPost, "/api/new-run", "", nil)
	if reset.Code != http.StatusOK || decodeState(t, reset).Complete {
		t.Fatalf("reset = %d %#v, want incomplete success", reset.Code, decodeState(t, reset))
	}
	afterReset := browser.request(t, server.Handler(), http.MethodPost, "/api/turn", "application/json", strings.NewReader(`{"message":"look around"}`))
	if afterReset.Code != http.StatusOK {
		t.Fatalf("turn after reset status = %d, want %d", afterReset.Code, http.StatusOK)
	}
}

func TestLogoutRemovesTheSessionAndExpiresCookie(t *testing.T) {
	server, _ := newFakeServer(t)
	browser := login(t, server.Handler())

	response := browser.request(t, server.Handler(), http.MethodPost, "/logout", "", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", response.Code, http.StatusNoContent)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("logout cookie = %#v, want expired session cookie", cookies)
	}
	response = browser.request(t, server.Handler(), http.MethodGet, "/api/session", "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("session status after logout = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := New(Config{Password: "test-password", NewGame: newTestGame})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

type fakeGames struct {
	now      time.Time
	runtimes []*fakeRuntime
}

func newFakeServer(t *testing.T) (*Server, *fakeGames) {
	t.Helper()
	games := &fakeGames{now: time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)}
	server, err := New(Config{
		Password: "test-password",
		Now:      func() time.Time { return games.now },
		NewGame: func() (Game, error) {
			runtime := &fakeRuntime{}
			games.runtimes = append(games.runtimes, runtime)
			return Game{
				Runtime: runtime,
				Complete: func() bool {
					return runtime.complete
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return server, games
}

func newTestGame() (Game, error) {
	return Game{Runtime: testRuntime{}, Complete: func() bool { return false }}, nil
}

func postForm(t *testing.T, handler http.Handler, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type testRuntime struct{}

func (testRuntime) ProcessTurn(context.Context, string) (session.ProcessedTurn, error) {
	return session.ProcessedTurn{}, nil
}

type fakeRuntime struct {
	messages []string
	duration int
	err      error
	complete bool
}

func (r *fakeRuntime) ProcessTurn(_ context.Context, message string) (session.ProcessedTurn, error) {
	r.messages = append(r.messages, message)
	if r.err != nil {
		return session.ProcessedTurn{}, r.err
	}
	if message == "finish" {
		r.complete = true
	}
	return session.ProcessedTurn{
		DurationSeconds: r.duration,
		Response:        response.Response{Text: "reply: " + message},
	}, nil
}

type browser struct {
	cookie *http.Cookie
}

func login(t *testing.T, handler http.Handler) browser {
	t.Helper()
	response := postForm(t, handler, "/login", url.Values{"password": {"test-password"}})
	if response.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", response.Code, http.StatusNoContent)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies = %#v, want one", cookies)
	}
	return browser{cookie: cookies[0]}
}

func (b browser) request(t *testing.T, handler http.Handler, method, path, contentType string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, body)
	request.AddCookie(b.cookie)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeState(t *testing.T, response *httptest.ResponseRecorder) struct {
	Entries  []transcriptEntry `json:"entries"`
	Complete bool              `json:"complete"`
} {
	t.Helper()
	var state struct {
		Entries  []transcriptEntry `json:"entries"`
		Complete bool              `json:"complete"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state response %q: %v", response.Body.String(), err)
	}
	return state
}

type transcriptEntry struct {
	Role string `json:"role"`
	Text string `json:"text"`
}
