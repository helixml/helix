# Design: Reduce Embedding Log Noise

## Problem

Every embedding request generates 3 INFO-level log lines (request, completion, provider init), repeated per chunk. A single user action can produce dozens of lines, making it hard to spot meaningful events in production logs.

## Solution

Change the log level from `Info()` to `Debug()` for routine embedding operations. This is the simplest fix — no new config, no new abstractions.

### Files to Change

1. **`api/pkg/openai/logger/openai_logger.go`**
   - Lines ~459, ~485: `CreateEmbeddings()` — request and completion logs
   - Lines ~510, ~544: `CreateFlexibleEmbeddings()` — request and completion logs
   - Change: `log.Info()` → `log.Debug()`
   - Keep: `log.Error()` for failure cases (lines ~475, ~534) stays as-is

2. **`api/pkg/openai/manager/provider_manager.go`**
   - Line ~403: "Initializing client for database-configured provider" log
   - Change: `log.Info()` → `log.Debug()`

### What stays at INFO

- Embedding error/failure logs (these are important signals)
- All other non-embedding log messages

### Codebase Notes

- Logging library: `github.com/rs/zerolog/log`
- Pattern: fluent API — `log.Info().Str("key", "val").Msg("message")`
- The change is mechanical: replace `.Info()` with `.Debug()` on 5 log statements
- zerolog's global level (set at app startup) controls whether DEBUG logs appear — no additional wiring needed
