package webconsole

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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

func newTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := New(Config{Password: "test-password", NewGame: newTestGame})
	if err != nil {
		t.Fatal(err)
	}
	return server
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
