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
		if request.Think == nil || *request.Think {
			t.Fatalf("think = %v, want explicit false", request.Think)
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

func TestOllamaClientGenerateJSONForwardsSchema(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []string{"actions"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		format, ok := request.Format.(map[string]any)
		if !ok || format["type"] != "object" {
			t.Fatalf("format = %#v", request.Format)
		}
		if request.Think == nil || *request.Think {
			t.Fatalf("think = %#v", request.Think)
		}
		_, _ = w.Write([]byte(`{"response":"{\"actions\":[]}"}`))
	}))
	defer server.Close()

	client, err := NewOllamaClient("qwen3.5:4b", WithOllamaBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GenerateJSON(context.Background(), "system", "user", schema); err != nil {
		t.Fatal(err)
	}
}

func TestOllamaClientGenerateJSONRequiresSchema(t *testing.T) {
	client, err := NewOllamaClient("qwen3.5:4b")
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GenerateJSON(context.Background(), "system", "user", nil)
	if err == nil || err.Error() != "json schema is required" {
		t.Fatalf("GenerateJSON error = %v, want json schema is required", err)
	}
}

func TestNewOllamaClientRequiresModel(t *testing.T) {
	_, err := NewOllamaClient("")
	if err == nil {
		t.Fatal("NewOllamaClient returned nil error for empty model")
	}
}
