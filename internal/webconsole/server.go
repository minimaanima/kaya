// Package webconsole provides the HTTP foundation for remote Kaya playtesting.
package webconsole

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"kaya/internal/session"
)

const sessionCookieName = "kaya_web_session"

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
	game       Game
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
	mux.HandleFunc("POST /login", server.handleLogin)
	server.handler = mux
	return server, nil
}

// Handler returns the web console HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.handler
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
	s.sessions[sessionID] = &webSession{game: game, lastActive: s.now()}
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

func (s *Server) newSessionID() (string, error) {
	var bytes [32]byte
	if _, err := io.ReadFull(s.random, bytes[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}
