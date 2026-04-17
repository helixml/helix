# Reduce embedding log noise

## Summary
Embedding operations generated 3 INFO-level log lines per chunk (request, completion, provider init), flooding production logs. Changed these to DEBUG level so they're hidden by default but available when needed.

## Changes
- Changed 4 embedding log statements in `api/pkg/openai/logger/openai_logger.go` from `log.Info()` to `log.Debug()` (request/completion for both standard and flexible embeddings)
- Changed 1 provider initialization log in `api/pkg/openai/manager/provider_manager.go` from `log.Info()` to `log.Debug()`
- Error logs for embedding failures remain at `log.Error()` level
