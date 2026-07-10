# Task 4 report

## RED

Added multi-action, fallback, and max-action tests. Focused run initially failed because `TurnPlan`, contextual `Parser.Parse`, `FallbackPlan`, and `ParseTurnPlanJSON` were undefined.

## GREEN

Implemented strict TurnPlan decoding, schema, contextual generation with one repair, low-confidence clarification, deterministic fallback, and updated parser/integration tests.

## Files

- `internal/intent/turn_plan.go`
- `internal/intent/schema.go`
- `internal/intent/fallback.go`
- `internal/intent/parser.go`
- `internal/intent/prompt.go`
- `internal/intent/parser_test.go`
- `internal/intent/ollama_integration_test.go`

## Tests

- `rtk proxy go test ./internal/intent -count=1` PASS
- `rtk proxy go test ./...` blocked by expected Task 5 CLI migration: `cmd/kaya` still consumes legacy `Intent` and parser signature.

## Self-review

- Unknown JSON fields rejected with `DisallowUnknownFields`.
- Required top-level, action, embedded-intent, and question fields validated.
- Four-entry limits, target modes, action/question kinds, confidence bounds, code fences, and trailing JSON validated.
- Generator failures and failed repair deterministically use `FallbackPlan`.

## Concerns

Task 5 must migrate `cmd/kaya` and resolver flow from `Intent` to `TurnPlan` before repository-wide tests can pass.

## Schema null/type hardening

Review identified that JSON `null` could decode into Go zero values. Added RED tests for null top-level fields and embedded `modifiers`; GREEN validation now rejects null and wrong JSON types for all required top-level, action, embedded-intent, and question fields. `modifiers:null` is no longer normalized to an empty array.

After hardening, `rtk proxy go test ./...` still reports only the same `cmd/kaya` TurnPlan migration compile errors; all non-CLI packages pass.
