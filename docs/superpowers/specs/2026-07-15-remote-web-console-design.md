# Remote Web Console Design

## Goal

Provide a small, phone-friendly web terminal for remote Kaya playtesting. The production player experience remains a desktop UI; this is a development and testing console only.

## Scope

Add a `kaya web` command that runs the existing game engine behind a local HTTP server. The server is intended to be exposed securely through Ngrok, while Ollama and all game processing remain on the home computer.

The first version includes:

- a password-gated, dark terminal-style page;
- a transcript of player commands and Kaya responses;
- a fixed mobile-friendly command input and Send button;
- a New Run action;
- one isolated game session per authenticated browser;
- predictable errors for bad input, expired sessions, and unavailable game services.

It deliberately excludes account management, persistent saves, multiple-player sessions, a public API, or changes to the desktop game UI.

## Deployment Model

The developer starts the server locally:

```text
kaya web --addr 127.0.0.1:8080
ngrok http 8080
```

`kaya web` binds to loopback by default, so the server is not directly reachable on the local network. Ngrok supplies the public HTTPS address used from a phone.

The command requires a password value supplied through an environment variable. It must refuse to start if the value is missing. The password is never written to source code, logs, or the browser after login.

## Authentication And Sessions

The web server presents a password form before any game endpoint is available.

On a successful password check, it issues a cryptographically random, secure, HTTP-only, SameSite cookie. The server stores the associated web session in memory, including its game state and the session's last activity time.

Sessions expire after a fixed idle period and are removed by the server. An expired or invalid session receives an authentication response and the browser returns to the password form. Each authenticated browser gets a separate game run; reconnecting with its valid cookie resumes that run.

The password comparison uses constant-time comparison. Authentication failures return a generic response and do not disclose whether configuration is missing or which value was incorrect.

## HTTP Interface

The server serves a single self-contained HTML page and small JSON endpoints:

- `GET /`: login page or terminal page, depending on authentication state.
- `POST /login`: checks the configured password and creates a browser session.
- `POST /logout`: clears the session cookie and removes its in-memory game session.
- `GET /api/session`: returns the authenticated browser's transcript and whether its run is complete.
- `POST /api/turn`: accepts one non-empty player message and returns the command, game response, elapsed time, and completion state.
- `POST /api/new-run`: discards the current run and creates a fresh generated run for the authenticated browser.

Only authenticated requests may use game endpoints. The server validates request sizes and content type before parsing JSON. Game processing remains synchronous per web session so turns cannot race and corrupt state.

## User Interface

The terminal page is intentionally minimal:

- a header identifying the Kaya remote console plus New Run and Logout controls;
- a vertically scrollable transcript that distinguishes player input, Kaya output, game time, errors, and completed runs;
- a command field anchored at the bottom with a Send button; Enter submits a command;
- responsive layout that works on a narrow phone viewport without horizontal scrolling.

The client escapes all transcript content before displaying it and disables the form while a turn request is in flight. New Run asks for confirmation before discarding an active run. On page load, the client retrieves the saved transcript for the current browser session, so a phone refresh or brief connection loss resumes both the run and its visible history.

## Game Integration

The web command reuses the same run generation, semantic parser, response composer, and `session` processing flow used by `kaya play`. It does not duplicate game rules or call the deterministic resolver directly.

A new web game session owns its generated state and runtime session. Server construction receives dependencies for generation, parsing, response composition, time, random bytes, and configuration where practical, keeping HTTP behavior testable without Ollama.

The initial transcript contains the current connection and Kaya greeting used by the CLI. Reaching the stairwell records the completion response and prevents additional turn requests until the user chooses New Run.

## Errors And Limits

- Missing or invalid password configuration prevents startup with a clear local error.
- Invalid login returns a generic authentication error.
- Missing, malformed, oversized, or blank turn input returns a client error without modifying the game state.
- A parser or composer failure returns the same player-safe signal-break message as the CLI; the detailed error stays in local server logs only.
- Expired sessions return to login and never expose another browser's game state.
- Game requests for the same session are serialized; requests from separate sessions may run independently.

## Test Strategy

Unit and HTTP handler tests will verify:

- the web command requires a password configuration and defaults to loopback;
- unauthenticated game endpoints are rejected;
- valid login creates a secure session and invalid login does not;
- one browser cannot access another browser's run;
- a turn uses the existing session processing path and appends the response;
- new run replaces only the caller's game state;
- invalid input and game failures return safe, useful responses;
- completed games reject further turns until reset;
- the rendered UI includes the accessibility and mobile layout hooks needed by the client code.

The full Go test suite will run after implementation. Manual verification will run the server on loopback, expose it through Ngrok, and complete a test game from a phone browser.
