# Compact Intent Model Contract Design

## Purpose

Improve the raw intent accuracy of the installed `qwen3.5:4b` model without
weakening Kaya's deterministic safety layer or changing the 52 expected corpus
plans.

The current model receives a large nested schema whose action entries repeat
confidence, raw text, and clarification metadata. The prompt does not define the
ownership of `target`, `item`, or `direction` and contains no examples. The
measured result is 1 raw exact match out of 52 even though the resolved hybrid
parser reaches 52 out of 52.

## Chosen Approach

Introduce a compact model-only transport contract and convert it into the
existing `TurnPlan` domain type. The game engine, resolver, and public parser
result keep using `TurnPlan`; only the data requested from Ollama becomes
smaller.

This is preferred over immediately changing models because a five-message probe
showed that explicit field rules, schema descriptions, and examples corrected
the model's action and field selection. A larger model can be benchmarked after
the contract itself is sound.

## Compact Transport

The model returns:

- `actions`, with at most four ordered entries.
- Per action: `action`, `target`, `item`, `direction`, `modifiers`, and
  `targetMode`.
- `questions`, with at most four entries containing `kind`, `target`, and
  `targetMode`.
- `needsClarification`.

The model does not return confidence, raw text, or clarification wording.
Those are parser-owned values:

- `rawText` comes from the original player message.
- Executable model plans receive a stable parser confidence.
- Clarification plans receive low confidence and the engine's default question.
- Every converted action receives initialized modifier and metadata fields.

Strict JSON decoding, unknown-field rejection, four-entry limits, action enums,
question enums, and target-mode validation remain mandatory.

## Prompt Contract

The system prompt defines each canonical action and field:

- Movement stores its direction only in `direction`.
- Inspection, search, taking, hiding, and listening store the acted-on object in
  `target`.
- `take_item` stores the collected object in `target`, never `item`.
- Activation stores the device in `item`.
- `use_item` stores the tool in `item` and destination in `target`.
- `explore` is reserved for tactile wall exploration.
- `targetMode` is `all` only when the player explicitly says all, both, or
  every; otherwise it is `single`.
- Compound actions preserve exact left-to-right order and are never duplicated.
- Unused fields are empty strings or empty arrays.

The prompt includes 8 to 12 hand-selected examples covering room awareness,
search, inventory questions, item use, movement, compounds, plural targets,
and clarification. Examples are selected from behavior categories rather than
copying all corpus messages.

The compact JSON schema is serialized into the model prompt as grounding while
also being supplied through Ollama's `format` field.

## Data Flow

1. `Parser` builds the compact model prompt from the player message and
   perception snapshot.
2. Ollama receives the compact schema through `format` and as prompt text.
3. The parser strictly decodes the compact response.
4. The parser converts it into a fully initialized `TurnPlan`.
5. Existing contextual canonicalization produces the safe resolved plan.
6. Provenance retains the pre-canonical converted plan so the evaluator can
   compare raw and resolved accuracy honestly.
7. Repair uses the same compact schema and conversion path.

Gameplay fallback behavior remains unchanged when generation or both decoding
attempts fail.

## Testing

Test-first coverage will verify:

- The model request uses the compact schema rather than `TurnPlanSchema`.
- The prompt contains schema grounding, field ownership rules, and ordered
  examples.
- Compact JSON converts to a fully initialized `TurnPlan`.
- Unknown fields, invalid actions, invalid modes, null arrays, and more than
  four entries are rejected.
- Repair receives and returns the compact contract.
- Existing deterministic and parser tests remain green.
- The 52 corpus messages and expected semantic plans remain unchanged.

The live evaluator remains the final measurement and must report:

- Raw model exact accuracy.
- Resolved parser exact accuracy.
- Canonicalized or fallback-assisted count.
- Repair count and generator/decoding errors.

## Acceptance Criteria

- The installed `qwen3.5:4b` model is used for the first benchmark; no model
  swap is bundled into this change.
- Raw exact accuracy improves by at least 20 percentage points over the measured
  1.9 percent baseline.
- Resolved exact accuracy remains at least 90 percent, with a target of 52 out
  of 52.
- Generator and decoding fallback errors remain zero during the live run.
- No expected corpus plan is weakened or changed to match model mistakes.
- Ordinary `go test ./...` remains independent of Ollama.

If raw accuracy does not improve by 20 percentage points, the compact contract
is not accepted and the result is documented before testing a different model
or API style.

## Non-Goals

- Switching to `qwen3.5:9b` or another model.
- Fine-tuning or LoRA training.
- Removing deterministic canonicalization.
- Changing world resolution, action execution, or Kaya's prose.
- Treating the 52 corpus cases as proof of arbitrary-language coverage.
