// Package webconsole provides the HTTP foundation for remote Kaya playtesting.
package webconsole

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kaya/internal/session"
)

const (
	sessionCookieName      = "kaya_web_session"
	sessionInactivityLimit = 12 * time.Hour
	maxTurnRequestBytes    = 4096
	maxTurnMessageRunes    = 2000
)

// Runtime processes one player command for an active game.
type Runtime interface {
	ProcessTurn(context.Context, string) (session.ProcessedTurn, error)
}

// Game contains an isolated game runtime and reports whether its run is complete.
type Game struct {
	Runtime  Runtime
	Complete func() bool
}

// GameFactory creates an isolated game for an authenticated browser session.
type GameFactory func() (Game, error)

// Entry is one line in a browser game's transcript.
type Entry struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// State is the JSON response returned for an authenticated game session.
type State struct {
	Entries  []Entry `json:"entries"`
	Complete bool    `json:"complete"`
}

// Config configures the web console server.
type Config struct {
	Password string
	NewGame  GameFactory
	Now      func() time.Time
	Random   io.Reader
}

// Server owns web-console authentication and in-memory browser sessions.
type Server struct {
	passwordHash [sha256.Size]byte
	newGame      GameFactory
	now          func() time.Time
	random       io.Reader
	sessions     map[string]*webSession
	mu           sync.Mutex
	handler      http.Handler
}

type webSession struct {
	mu         sync.Mutex
	game       Game
	entries    []Entry
	complete   bool
	lastActive time.Time
}

// New creates a password-protected web console server.
func New(config Config) (*Server, error) {
	if strings.TrimSpace(config.Password) == "" {
		return nil, errors.New("KAYA_WEB_PASSWORD must be set")
	}
	if config.NewGame == nil {
		return nil, errors.New("web game factory must not be nil")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}

	server := &Server{
		passwordHash: sha256.Sum256([]byte(config.Password)),
		newGame:      config.NewGame,
		now:          config.Now,
		random:       config.Random,
		sessions:     make(map[string]*webSession),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", server.handleRoot)
	mux.HandleFunc("POST /login", server.handleLogin)
	mux.HandleFunc("GET /api/session", server.requireSession(server.handleSession))
	mux.HandleFunc("POST /api/turn", server.requireSession(server.handleTurn))
	mux.HandleFunc("POST /api/new-run", server.requireSession(server.handleNewRun))
	mux.HandleFunc("POST /logout", server.requireSession(server.handleLogout))
	server.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.pruneExpiredSessions()
		mux.ServeHTTP(w, r)
	})
	return server, nil
}

// Handler returns the web console HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.activeSession(r); ok {
		writeHTML(w, terminalDocument)
		return
	}
	writeHTML(w, loginDocument)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid login request", http.StatusBadRequest)
		return
	}
	attemptHash := sha256.Sum256([]byte(r.Form.Get("password")))
	if subtle.ConstantTimeCompare(s.passwordHash[:], attemptHash[:]) != 1 {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}

	game, err := s.newGame()
	if err != nil {
		http.Error(w, "unable to start game", http.StatusInternalServerError)
		return
	}

	sessionID, err := s.newSessionID()
	if err != nil {
		http.Error(w, "unable to start session", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.sessions[sessionID] = newWebSession(game, s.now())
	s.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireSession(next func(http.ResponseWriter, *http.Request, *webSession)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		active, ok := s.activeSession(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r, active)
	}
}

func (s *Server) activeSession(r *http.Request) (*webSession, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.sessions[cookie.Value]
	if !ok {
		return nil, false
	}
	if sessionExpired(active.lastActive, now) {
		delete(s.sessions, cookie.Value)
		return nil, false
	}
	active.lastActive = now
	return active, true
}

func (s *Server) pruneExpiredSessions() {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, active := range s.sessions {
		if sessionExpired(active.lastActive, now) {
			delete(s.sessions, id)
		}
	}
}

func sessionExpired(lastActive, now time.Time) bool {
	return !now.Before(lastActive.Add(sessionInactivityLimit))
}

func (s *Server) handleSession(w http.ResponseWriter, _ *http.Request, active *webSession) {
	active.mu.Lock()
	defer active.mu.Unlock()
	writeState(w, active)
}

func (s *Server) handleTurn(w http.ResponseWriter, r *http.Request, active *webSession) {
	message, ok := decodeTurnMessage(w, r)
	if !ok {
		return
	}

	active.mu.Lock()
	defer active.mu.Unlock()
	if active.complete {
		writeError(w, http.StatusConflict, "start a new run to continue")
		return
	}

	active.entries = append(active.entries, Entry{Role: "player", Text: message})
	processed, err := active.game.Runtime.ProcessTurn(r.Context(), message)
	if err != nil {
		active.entries = append(active.entries, Entry{Role: "kaya", Text: "Kaya: The signal broke up. I did not understand that."})
		writeState(w, active)
		return
	}
	if processed.DurationSeconds > 0 {
		active.entries = append(active.entries, Entry{Role: "system", Text: fmt.Sprintf("[time +%ds]", processed.DurationSeconds)})
	}
	active.entries = append(active.entries, Entry{Role: "kaya", Text: processed.Response.Text})
	if !active.complete && active.game.Complete != nil && active.game.Complete() {
		active.complete = true
		active.entries = append(active.entries,
			Entry{Role: "kaya", Text: "I am in the stairwell. This part is clear."},
			Entry{Role: "system", Text: "Prototype objective complete."},
		)
	}
	writeState(w, active)
}

func (s *Server) handleNewRun(w http.ResponseWriter, _ *http.Request, active *webSession) {
	active.mu.Lock()
	defer active.mu.Unlock()
	game, err := s.newGame()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to start game")
		return
	}
	active.game = game
	active.entries = initialEntries()
	active.complete = gameComplete(game)
	writeState(w, active)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, _ *webSession) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	s.mu.Lock()
	delete(s.sessions, cookie.Value)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func newWebSession(game Game, now time.Time) *webSession {
	return &webSession{
		game:       game,
		entries:    initialEntries(),
		complete:   gameComplete(game),
		lastActive: now,
	}
}

func gameComplete(game Game) bool {
	return game.Complete != nil && game.Complete()
}

func initialEntries() []Entry {
	return []Entry{
		{Role: "system", Text: "Connection established."},
		{Role: "kaya", Text: "I can read you. I am in reception. The ceiling is cracked, but I can move."},
	}
}

func decodeTurnMessage(w http.ResponseWriter, r *http.Request) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusBadRequest, "turn requests must use application/json")
		return "", false
	}

	var request struct {
		Message string `json:"message"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid turn request")
		return "", false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid turn request")
		return "", false
	}
	message := strings.TrimSpace(request.Message)
	if message == "" {
		writeError(w, http.StatusBadRequest, "enter a command")
		return "", false
	}
	if utf8.RuneCountInString(message) > maxTurnMessageRunes {
		writeError(w, http.StatusBadRequest, "command is too long")
		return "", false
	}
	return message, true
}

func writeState(w http.ResponseWriter, active *webSession) {
	entries := append([]Entry(nil), active.entries...)
	writeJSON(w, http.StatusOK, State{Entries: entries, Complete: active.complete})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeHTML(w http.ResponseWriter, document string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, document)
}

func (s *Server) newSessionID() (string, error) {
	var bytes [32]byte
	if _, err := io.ReadFull(s.random, bytes[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}
