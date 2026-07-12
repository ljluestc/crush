# fix: OpenRouter (and all provider) models not refreshing after `update-providers` on Windows (#1552)

## Problem

Users on Windows (PowerShell) who ran `crush update-providers` to pull the
latest model list from Catwalk would still see the stale model list the next
time they launched Crush — even though the cache file on disk had been
successfully updated.

Two bugs compounded to cause this:

### 1. `sync.Once` singletons were never reset after a cache write

`Providers()` is guarded by a package-level `providerOnce sync.Once`, and the
inner `catwalkSync.Get()` is guarded by its own `sync.Once`. Once either fired
it would never run again within the same process lifetime.

`UpdateProviders()` wrote a fresh cache file to disk but did **not** reset
those singletons. Any same-process call to `Providers()` after
`UpdateProviders()` — for example from a long-running TUI session — would
silently return the old in-memory list instead of reading the updated cache.

### 2. The resolved cache path was invisible to users

On Windows the cache lives at `%LOCALAPPDATA%\crush\providers.json`, but this
path was never shown in the `update-providers` output. If the path resolved
differently between sessions (e.g. elevated vs. non-elevated PowerShell,
different user profiles, or a missing `LOCALAPPDATA` env-var), the update
would write to one location while Crush read from another — with no feedback
to help diagnose the mismatch.

### 3. Logging bug: `cachePathFor` passed as a function value

```go
// Before — logs the Go function-pointer representation, not the path
slog.Info("Providers updated successfully", "to", cachePathFor)

// After
slog.Info("Providers updated successfully", "to", cachePath)
```

## Changes

### `internal/config/provider.go`

- **Extract `resetProviderState()` from test helper into production code.**
  Resets `providerOnce`, `providerList`, `providerErr`, `catwalkSyncer`, and
  `hyperSyncer` so the next `Providers()` call re-reads from disk.
- **Call `resetProviderState()` at the end of both `UpdateProviders()` and
  `UpdateHyper()`** so the updated cache is visible immediately in the same
  process.
- **Export `ProviderCachePath()` and `HyperCachePath()`** — thin wrappers
  around the internal `cachePathFor` helper — for diagnostics and for use in
  the CLI layer.
- **Fix the logging bug** — pass `cachePath` (the evaluated string) instead of
  `cachePathFor` (the function value).

### `internal/config/provider_test.go`

- Remove the now-duplicate `resetProviderState()` test helper (it lived in the
  same `package config` block and was therefore identical to the production
  version).  All existing tests continue to compile and pass unchanged.

### `internal/cmd/update_providers.go`

- After a successful update, print the resolved cache path in a faint line
  below the `SUCCESS` banner:

  ```
    SUCCESS

    catwalk provider updated successfully.
    Cache: C:\Users\you\AppData\Local\crush\providers.json
  ```

  Windows users can now immediately confirm that the file was written to the
  path Crush will read from when next launched.

## Testing

```
go test ./internal/config/... -count=1
```

All existing tests pass.  The `TestProviders_Integration_AutoUpdateDisabled`
test exercises `resetProviderState()` — it continues to work because the
helper is now defined in production code.

## Relates to

Closes #1552
