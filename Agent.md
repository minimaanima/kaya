# Go Agent Guide

Use this guide when adding or reviewing Go code in this workspace. The defaults below come from the Go project documentation and common production Go style guides.

## Core Rules

- Prefer clear, small packages with names that are short, lowercase, and specific. Avoid vague names like `util`, `common`, `types`, and `helpers`.
- Let package names reduce repetition. Prefer `httpserver.Server` over `httpserver.HTTPServer`, and `ring.New` over `ring.NewRing` when the package context is obvious.
- Run `gofmt` on all changed Go files. Use `goimports` when imports changed.
- Keep normal control flow at the left edge. Handle errors early, return, then continue the happy path without unnecessary `else` blocks.
- Do not use `panic` for ordinary errors. Return `error` values and handle them once.
- Error strings should normally be lowercase and should not end with punctuation.
- Do not discard errors with `_` unless there is a documented reason and the operation is provably safe to ignore.
- Avoid package-level mutable state. Prefer explicit dependencies passed through constructors or function parameters.
- Keep interfaces small and consumer-owned. Define an interface where it is used when practical, not where the concrete type is implemented.

## Context And Cancellation

- Pass `context.Context` explicitly as the first parameter for request-scoped work:

```go
func LoadUser(ctx context.Context, id string) (*User, error)
```

- Do not store `context.Context` in structs unless required by an external interface.
- Propagate cancellation and deadlines through database, HTTP, RPC, and worker calls.
- Use `context.Background()` only at process boundaries such as `main`, tests, and top-level service startup.

## Errors

- Wrap errors with useful operation context:

```go
if err != nil {
	return fmt.Errorf("load user %s: %w", id, err)
}
```

- Use `errors.Is` and `errors.As` for sentinel and typed errors.
- Return extra values for missing or optional results instead of encoding errors into in-band values:

```go
func Lookup(key string) (value string, ok bool)
```

- Avoid logging and returning the same error unless the current layer is intentionally adding an audit trail. Let one layer own the final log line.

## Concurrency

- Prefer synchronous functions. Let callers add goroutines when they need concurrency.
- Every goroutine needs an obvious exit path. If the lifetime is not obvious, document what stops it.
- Avoid fire-and-forget goroutines in libraries.
- Protect shared mutable state with channels, mutexes, or atomics. Do not rely on timing.
- Run race-sensitive changes with:

```sh
go test -race ./...
```

- Be careful copying values that contain locks, buffers, slices, maps, or other references.

## Slices, Maps, And Pointers

- A nil slice is the preferred empty slice in most cases:

```go
var items []Item
```

- Use an explicit empty slice when the observable output requires it, such as JSON `[]` instead of `null`.
- Copy slices and maps at API boundaries when retaining or returning them would expose mutable internal state.
- Do not pass pointers to small values, strings, or interfaces just to avoid copying.
- Use pointer receivers when methods mutate the receiver, the receiver contains a lock, the receiver is large, or consistency with other methods requires it.

## Testing

- Put tests in `*_test.go` files and run them with `go test ./...`.
- Prefer table-driven tests for repeated input/output cases.
- Test exported behavior, edge cases, and error paths. Add regression tests for bug fixes.
- Failure messages should include the input, actual value, and expected value.
- Use fuzzing for parsers, decoders, validators, protocol handling, and other input-heavy code:

```sh
go test -fuzz=FuzzName ./path/to/package
```

- Fuzz targets should be deterministic, fast, and independent of global state.

## Dependencies And Modules

- Keep `go.mod` and `go.sum` committed.
- Use Go commands to edit dependency state:

```sh
go get example.com/module@v1.2.3
go mod tidy
```

- Run `go mod tidy` after adding, removing, or moving imports.
- Avoid long-lived local `replace` directives in committed code unless they are intentional and documented.
- For Go 1.24 or newer, track Go-based developer tools with `go get -tool` and run them with `go tool` when the project uses that pattern.

## Security

- Use `crypto/rand`, not `math/rand`, for secrets, tokens, keys, nonces, and security-sensitive randomness.
- Validate and bound untrusted inputs before parsing, allocating, or spawning work from them.
- Avoid `unsafe` unless there is a measured need and a clear comment explaining the invariant that makes it safe.
- Use `govulncheck` when dependencies or security-sensitive code changed:

```sh
govulncheck ./...
```

## Pre-Submit Checklist

Run the relevant subset before handing off Go changes:

```sh
gofmt -w .
go test ./...
go test -race ./...
go mod tidy
govulncheck ./...
```

If this repository later gains a Makefile, task runner, or CI script, prefer the repository command over ad hoc commands.

## Sources

- Effective Go: https://go.dev/doc/effective_go
- Go Code Review Comments: https://go.dev/wiki/CodeReviewComments
- Managing dependencies: https://go.dev/doc/modules/managing-dependencies
- Go testing tutorial: https://go.dev/doc/tutorial/add-a-test
- Go fuzzing: https://go.dev/doc/security/fuzz/
- Go data race detector: https://go.dev/doc/articles/race_detector
- Go security docs and govulncheck: https://go.dev/doc/security/
- Uber Go Style Guide: https://github.com/uber-go/guide/blob/master/style.md
