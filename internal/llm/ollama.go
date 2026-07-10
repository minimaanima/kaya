package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultOllamaURL = "http://localhost:11434"

type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

type OllamaOption func(*OllamaClient)

func NewOllamaClient(model string, options ...OllamaOption) (*OllamaClient, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("ollama model is required")
	}

	client := &OllamaClient{
		baseURL: DefaultOllamaURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
	for _, option := range options {
		option(client)
	}

	client.baseURL = strings.TrimRight(client.baseURL, "/")
	if client.baseURL == "" {
		return nil, errors.New("ollama base url is required")
	}

	return client, nil
}

func WithOllamaBaseURL(baseURL string) OllamaOption {
	return func(c *OllamaClient) {
		c.baseURL = baseURL
	}
}

func WithHTTPClient(httpClient *http.Client) OllamaOption {
	return func(c *OllamaClient) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func (c *OllamaClient) Generate(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	think := false
	body := ollamaGenerateRequest{
		Model:  c.model,
		System: systemPrompt,
		Prompt: userPrompt,
		Stream: false,
		Format: "json",
		Think:  &think,
		Options: map[string]any{
			"temperature": 0,
		},
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama returned %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}

	var generated ollamaGenerateResponse
	if err := json.Unmarshal(responseBody, &generated); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	if generated.Response == "" {
		return "", errors.New("ollama returned empty response")
	}

	return generated.Response, nil
}

type ollamaGenerateRequest struct {
	Model   string         `json:"model"`
	System  string         `json:"system"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Format  string         `json:"format,omitempty"`
	Think   *bool          `json:"think"`
	Options map[string]any `json:"options,omitempty"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}
