# Intent Corpus Design

## Purpose

Create a reusable corpus of natural player messages paired with exact expected
intent plans. The corpus will test both Kaya's deterministic parser behavior and
the real Ollama-backed parser without maintaining two sets of examples.

The first corpus contains approximately 50 cases based on language a player is
likely to use, including imperfect spelling and multi-action turns.

## Shared Corpus

Each corpus case contains:

- A descriptive name.
- The player's message.
- An ordered list of expected actions.
- For each action: action, target, item, direction, and target mode.
- The expected clarification state and, where relevant, expected questions.

Expected values use Kaya's normalized domain types rather than raw model JSON.
Confidence values and raw text are excluded from equality checks because they do
not change the meaning of a correctly parsed plan.

The initial cases cover:

- Room awareness and object inspection.
- Container and object searches.
- Inventory and item-location questions.
- Taking, using, turning on, and throwing items.
- Cardinal, relative, and return movement.
- Listening, hiding, waiting, and exploration.
- Singular, plural, and contextual targets.
- Ordered compound actions.
- Common spelling mistakes observed during playtests.
- Ambiguous or non-action messages that must request clarification.

## Deterministic Contract Test

The normal unit test suite runs every corpus message through the deterministic
fallback parser. It compares the resulting plan to the exact expected semantic
fields and validates the resulting plan's structural invariants.

This test is fast, has no network or Ollama dependency, and runs as part of
`go test ./...`. A failure identifies the case and prints a compact expected and
actual plan comparison.

The deterministic suite is the minimum behavior guaranteed by the engine. Cases
which intentionally require model-only reasoning are not included until a stable
deterministic interpretation exists; this prevents the shared corpus from having
different meanings in its two consumers.

## Ollama Evaluation

An opt-in integration test sends the same messages through `Parser` using the
configured Ollama generator. It is enabled only when `KAYA_OLLAMA_EVAL` is truthy,
so ordinary tests remain fast and reproducible.

The evaluator processes every case even after mismatches. It reports:

- Total cases, exact matches, mismatches, and accuracy percentage.
- Each failed message.
- A compact difference between expected and actual semantic plans.
- Parser errors separately from semantic mismatches.

The evaluation fails when any parser error occurs or when exact-match accuracy is
below the documented threshold. The initial threshold is 90 percent. Exact
results remain visible even when the threshold passes, so tolerated model errors
cannot disappear from the report.

The command is:

```powershell
$env:KAYA_OLLAMA_EVAL="1"
go test ./internal/intent -run TestOllamaIntentCorpus -v
```

Ollama model and endpoint configuration continue to use the project's existing
configuration. The corpus adds no second model client.

## Boundaries

This feature evaluates intent parsing only. It does not execute actions against a
game state, resolve whether an object is visible, or verify Kaya's final prose.
Those behaviors belong to resolver, turn executor, and end-to-end playtests.

The corpus is hand-curated. Automatically generated phrases may be proposed in a
future tool, but they must be reviewed and assigned an explicit expected plan
before becoming contract cases.

## Success Criteria

- Approximately 50 realistic player messages have exact expected plans.
- The deterministic corpus passes in the normal test suite.
- The Ollama evaluation uses the same corpus and is skipped by default.
- Live evaluation reports all mismatches and a final accuracy percentage.
- Adding a new regression requires adding one small corpus entry rather than a
  new custom test function.
