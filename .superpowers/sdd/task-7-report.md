# Task 7 report: fact-locked Ollama response composition

## RED

Added `internal/response/composer_test.go` with valid-draft, unknown fact ID, missing required fact, unknown entity, and generator-error cases.

`rtk proxy go test ./internal/response -run TestComposer -count=1` failed as expected because `NewComposer` and the composer implementation were undefined.

## GREEN

Implemented:

- `internal/response/prompt.go`: strict response draft types/schema, safe system prompt, and minimal response input payload.
- `internal/response/validator.go`: strict JSON decoding with unknown-field/trailing-data rejection, sentence/character limits, fact-ID coverage, and named-entity validation.
- `internal/response/composer.go`: generator integration, deterministic fallback reasons, fact-locked rendering, and ordered `UsedFactIDs`.
- `internal/response/composer_test.go`: strict schema, trailing data, sentence-count, and text-length regressions.

Focused verification:

```text
rtk proxy go test ./internal/response ./internal/llm -count=1
ok   kaya/internal/response
ok   kaya/internal/llm
```

## Concerns

`rtk proxy go test ./... -count=1` still fails only in `cmd/kaya`; pre-existing parser/turn-plan API drift remains outside Task 7 scope. All internal packages pass.
