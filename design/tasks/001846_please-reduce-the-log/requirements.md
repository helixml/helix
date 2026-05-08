# Requirements: Reduce Embedding Log Noise

## User Stories

- As a developer monitoring production logs, I want embedding operations to log at DEBUG level so that routine embedding traffic doesn't drown out important log messages.
- As a developer debugging embedding issues, I want to be able to enable verbose embedding logging when needed.

## Acceptance Criteria

- [ ] Embedding request/completion logs in `openai_logger.go` (lines ~459-500, ~510-559) are changed from `log.Info()` to `log.Debug()`
- [ ] Provider client initialization log in `provider_manager.go` (line ~403) is changed from `log.Info()` to `log.Debug()`
- [ ] Error logs (`log.Error()`) for embedding failures remain unchanged
- [ ] No new configuration flags or environment variables are needed — zerolog's global level already controls DEBUG visibility
- [ ] Existing non-embedding log messages are not affected
