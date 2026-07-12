# Intent Model Selection Research

Date: 2026-07-12

## Decision

Kaya should not optimize its intent contract for Qwen or any other single
model. Define one compact model-neutral semantic contract, then select the
runtime model through a repeatable local benchmark.

The current result does not show that Qwen is uniquely unsuitable:

| Installed model | Raw exact | Resolved exact | Errors | Duration |
|---|---:|---:|---:|---:|
| `qwen3.5:4b` | 1/52 (1.9%) | 52/52 (100%) | 0 | 158 s |
| `mistral:latest` | 1/52 (1.9%) | 48/52 (92.3%) | 1 | 167 s |

Both models received the existing large nested schema and under-specified
prompt. Their nearly identical raw failure rate is strong evidence that the
contract is the first bottleneck. Model selection should happen after the
compact contract is available.

## Local Hardware Envelope

The development machine has an NVIDIA RTX 4070 SUPER with 12,282 MiB VRAM and
Ollama 0.31.2.

For low-latency play, prefer model files below roughly 10 GB so weights and
runtime cache can remain primarily on the GPU. Models around 14-15 GB can run
with system-memory offload but are less attractive for a conversational game.

## Requirements

The intent model needs:

- Reliable schema-constrained JSON.
- Precise instruction following for field ownership.
- Ordered multi-action extraction.
- Singular versus explicit-plural distinction.
- Robustness to conversational wording and spelling mistakes.
- Low enough latency for every player message.
- A license suitable for future distribution.
- Stable Ollama support.

General reasoning, creative prose, vision, and very long context are secondary
for intent parsing.

## Candidate Shortlist

### Tier A: benchmark first

#### 1. Ministral 3 14B

- Ollama artifact: `ministral-3:14b`
- Approximate model size: 9.1 GB
- Fits the 12 GB GPU envelope.
- Designed for edge deployment and advertises tool use.
- Strongest first candidate for the balance of model size and structured task
  capability.

Source: [Ollama Ministral 3](https://ollama.com/library/ministral-3)

#### 2. Gemma 3 12B

- Ollama artifact: `gemma3:12b`
- Approximate model size: 8.1 GB
- Fits the GPU comfortably.
- Google positions Gemma 3 as a capable single-GPU model.
- Does not advertise native tool calling on its Ollama page, so structured
  extraction quality must be measured rather than assumed.

Source: [Ollama Gemma 3](https://ollama.com/library/gemma3)

#### 3. Mistral NeMo 12B

- Ollama artifact: `mistral-nemo:12b`
- Approximate model size: 7.1 GB
- Fits comfortably and supports tool-oriented use.
- Older than Ministral 3 but useful as a mature 12B baseline.

Source: [Ollama Mistral NeMo](https://ollama.com/library/mistral-nemo)

#### 4. Qwen 3.5 9B

- Ollama artifact: `qwen3.5:9b`
- Approximate model size: 6.6 GB
- Fits comfortably.
- Included as a control to determine whether additional capacity helps once
  the contract is model-neutral, not as the predetermined winner.

Source: [Ollama Qwen 3.5 tags](https://registry.ollama.com/library/qwen3.5/tags)

### Tier B: low-latency alternatives

#### 5. Llama 3.1 8B

- Ollama artifact: `llama3.1:8b`
- Approximate model size: 4.9 GB.
- Native tool-oriented model family and broad runtime support.
- Useful portable baseline, though older than the Tier A models.

Source: [Ollama Llama 3.1 tags](https://ollama.com/library/llama3.1/tags)

#### 6. Granite 4 7B-A1B-H

- Ollama artifact: `granite4:7b-a1b-h`
- Approximate model size: 4.2 GB.
- IBM describes Granite 4 as improved for instruction following, extraction,
  classification, and function calling.
- Very low active parameter count may make it fast, but exact semantic accuracy
  needs direct measurement.

Source: [Ollama Granite 4](https://registry.ollama.com/library/granite4)

#### 7. Phi-4 Mini 3.8B

- Ollama artifact: `phi4-mini:3.8b`
- Approximate model size: 2.5 GB.
- Supports function calling and is designed for constrained, latency-sensitive
  environments.
- Best candidate when speed matters more than maximum raw accuracy.

Source: [Ollama Phi-4 Mini](https://ollama.com/library/phi4-mini)

### Tier C: quality reference, likely too large for default play

#### 8. GPT-OSS 20B

- Ollama artifact: `gpt-oss:20b`
- Approximate model size: 14 GB.
- Explicit support for structured outputs, function calling, and configurable
  reasoning.
- Exceeds GPU VRAM and is documented as requiring about 16 GB total memory.
- Valuable as an upper-quality reference, but likely too slow as Kaya's default
  intent model on this machine.

Source: [Ollama GPT-OSS](https://ollama.com/library/gpt-oss%3Alatest)

#### 9. Mistral Small 3.2 24B

- Ollama artifact: `mistral-small3.2`
- Approximate model size: 15 GB.
- Specifically improves instruction following, repetition, and function
  calling.
- Exceeds the GPU envelope and should be tested only as a quality reference.

Source: [Ollama Mistral Small 3.2](https://ollama.com/library/mistral-small3.2)

## Model-Neutral Contract

Every candidate must receive the same compact semantic schema:

```json
{
  "actions": [
    {
      "action": "take_item",
      "target": "flashlight",
      "item": "",
      "direction": "",
      "modifiers": [],
      "targetMode": "single"
    }
  ],
  "questions": [],
  "needsClarification": false
}
```

The prompt defines field ownership and includes a small fixed set of category
examples. The schema is sent both through Ollama's `format` field and as prompt
grounding. Ollama recommends including the schema in the prompt and using low
temperature for reliable structured output.

Source: [Ollama structured outputs](https://docs.ollama.com/capabilities/structured-outputs)

No model-specific examples, aliases, or fallback rules are allowed during the
initial tournament. If a model needs a special adapter, benchmark it separately
and include the adapter cost in the decision.

## Benchmark Design

### Stage 1: smoke test

Run ten representative messages:

- Room awareness.
- Inspect and search.
- Inventory question.
- Take and use item.
- Movement.
- Explicit plural.
- Ambiguous follow-up.
- Two ordered compound actions.

Reject models that cannot produce valid schema or preserve compound order.

### Stage 2: current corpus

Run all 52 existing cases and record:

- Raw semantic exact accuracy.
- Per-action accuracy.
- Field accuracy for target, item, direction, and target mode.
- Valid JSON rate.
- Repair rate.
- Resolved parser accuracy.
- Canonicalization/fallback assistance rate.
- Mean, p50, and p95 latency.

### Stage 3: unseen holdout

Create at least 100 reviewed paraphrases not present in prompt examples. Keep
the holdout file unchanged while comparing models.

The holdout should include:

- Polite requests and indirect wording.
- Misspellings.
- Pronouns and remembered referents.
- Negation.
- Multi-object ambiguity.
- Two to four ordered actions.
- Questions that resemble commands.
- Unsupported conversation that should clarify.

## Acceptance Gates

A production intent model should meet:

- 100% valid schema after at most one repair.
- At least 80% raw exact accuracy on the unseen holdout.
- At least 95% action classification accuracy.
- At least 95% compound action order accuracy.
- At least 98% resolved semantic accuracy.
- No generator or decoding fallback errors.
- At most 20% canonicalization/fallback assistance.
- p95 parser latency at or below 3 seconds on the RTX 4070 SUPER.

These are initial engineering gates, not claims that arbitrary language is
solved. Adjust them only from measured playtest needs, never to make a preferred
model pass.

## Recommended Test Order

1. Implement the compact model-neutral contract.
2. Benchmark installed `qwen3.5:4b` and `mistral:latest` again.
3. Download and benchmark `ministral-3:14b`.
4. Benchmark `gemma3:12b`.
5. Benchmark `mistral-nemo:12b`.
6. Use `qwen3.5:9b` as the family-size control.
7. Test Phi-4 Mini or Granite 4 if Tier A latency is too high.
8. Test GPT-OSS 20B only as a quality ceiling.

The winner is the smallest model that clears the semantic and latency gates,
not the model with the strongest general benchmark reputation.

## Conclusion

Do not choose a permanent model yet. The current prompt/schema makes model
comparisons unreliable, as shown by both installed models scoring 1.9% raw.

The strongest first download for this hardware is `ministral-3:14b`, followed by
`gemma3:12b` and `mistral-nemo:12b`. However, the compact model-neutral contract
must come first; otherwise the project will only repeat the same contract failure
with a larger and slower model.
