package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaClientGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("path = %q, want /api/generate", r.URL.Path)
		}

		var request ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "mistral:latest" {
			t.Fatalf("model = %q, want mistral:latest", request.Model)
		}
		if request.Format != "json" {
			t.Fatalf("format = %q, want json", request.Format)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"{\"action\":\"wait\"}"}`))
	}))
	defer server.Close()

	client, err := NewOllamaClient("mistral:latest", WithOllamaBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewOllamaClient returned error: %v", err)
	}

	got, err := client.Generate(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if got != `{"action":"wait"}` {
		t.Fatalf("response = %q, want json response", got)
	}
}

func TestNewOllamaClientRequiresModel(t *testing.T) {
	_, err := NewOllamaClient("")
	if err == nil {
		t.Fatal("NewOllamaClient returned nil error for empty model")
	}
}
