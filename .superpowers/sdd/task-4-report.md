# Task 4 Report: Adversarial and Conversation Invariants

## Status

Complete. No unresolved Critical or Important findings.

## Commits

- `test: harden prototype adversarial behavior`

## Changed Files

- `internal/playtest/adversarial_test.go`
- `internal/playtest/response.go`
- `internal/playtest/response_test.go`
- `internal/playtest/runner.go`
- `internal/playtest/runner_test.go`
- `internal/actions/resolver.go`
- `internal/actions/resolver_test.go`
- `.superpowers/sdd/task-4-report.md`

## Characterization and Reproduced Defects

| Case | Result | Evidence |
| --- | --- | --- |
| take-before-discovery | Reproduced engine defect; fixed | Undiscovered visible items returned `item_not_discovered`, leaking their existence. Now returns `item_not_found`. |
| locked-door-does-not-move | Characterization | Locked north door leaves Kaya in Storage Room after 2 seconds. |
| dark-inspection-hides-objects | Characterization | Pitch-black inspection exposes no objects and only the known return route. |
| ambiguous-doctor-remembers-both | Characterization | Initial ambiguity stores the two doctors; literal `both` searches the remembered cabinet and door targets. |
| failed-first-action-stops-compound | Characterization | Failed key pickup stops the planned eastward move. |
| repeated-search-after-take | Characterization | Search, take, and repeat search produce found, taken, then empty outcomes. |
| known dark return route | Reproduced response-check defect; fixed | The checker falsely rejected known `west`; it now checks only exits not in `KnownExitDirections`. |

## RED/GREEN Evidence

- RED: `go test ./internal/playtest -run TestAdversarial -count=1 -v`
  - Isolated to `take-before-discovery`: got `item_not_discovered`, wanted `item_not_found`.
- RED: `go test ./internal/actions -run 'TestTakeItemBeforeDiscoveryDoesNotRevealItem|TestTakeUndiscoveredNonPortableItemDoesNotLeakPortability' -count=1 -v`
  - Both tests got `item_not_discovered`, wanted `item_not_found`.
- GREEN: the same focused actions command passed; `go test ./internal/playtest -run TestAdversarial -count=1 -v` passed all six rows.
- RED: `go test ./internal/playtest -run TestCheckResponse -count=1 -v`
  - Failed to build before `CheckResponse` existed.
- GREEN: the same focused response command passed prefix, fact grounding, clarification time, darkness, debug, fallback, word-boundary, and known-return-route checks.
- RED: `go test ./internal/playtest -run TestRunnerStepStoresResponseViolationBeforeReturning -count=1 -v`
  - Runner returned nil for `debug: raw plan`.
- GREEN: `go test ./internal/playtest -run 'TestRunnerStepStoresResponseViolationBeforeReturning|TestCheckResponse' -count=1 -v` passed.

## Verification Commands

- `go test ./internal/playtest -run 'TestAdversarial|TestResponse|TestPrototypeThousand' -count=1 -v` - PASS, including 1,000 deterministic sessions and all nine placement combinations.
- `go test ./internal/playtest -count=1` - PASS.
- `go test ./... -count=1` - PASS.
- `go vet ./...` - PASS.
- `git diff --check` - PASS; Git emitted only existing LF-to-CRLF warnings.

## Invariant Checklist

- Deterministic fixed placements are used for every adversarial precondition.
- Every adversarial step asserts time, inventory, discovery, light, room, and stairwell-door state.
- Literal `both` exercises conversation memory and executes searches for both remembered doctors.
- Compound failure proves the second planned action did not execute.
- Non-fallback fact IDs are checked against `Result.FactBundle(step.Player)` from the stored turn.
- Fallback responses remain subject to prefix, clarity/time, darkness, and debug checks.
- Pitch-black detection compares normalized token/name sequences and excludes known exits.
- Runner appends response violations before storing the Step and reports the code plus response text immediately.

## Deviations

- The brief's first doctors message is an ambiguity turn with `target_ambiguous`; the required `searched_empty` terminal outcome occurs on the literal remembered `both` follow-up.
- Go test and vet commands required shared Go build-cache access outside the filesystem sandbox.

## Concerns

- None.
