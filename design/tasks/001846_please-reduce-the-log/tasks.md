# Implementation Tasks

- [ ] In `api/pkg/openai/logger/openai_logger.go`: change `log.Info()` to `log.Debug()` for the "Embedding request" log (~line 459)
- [ ] In `api/pkg/openai/logger/openai_logger.go`: change `log.Info()` to `log.Debug()` for the "Embedding completed" log (~line 485)
- [ ] In `api/pkg/openai/logger/openai_logger.go`: change `log.Info()` to `log.Debug()` for the "Flexible embedding request" log (~line 510)
- [ ] In `api/pkg/openai/logger/openai_logger.go`: change `log.Info()` to `log.Debug()` for the "Flexible embedding completed" log (~line 544)
- [ ] In `api/pkg/openai/manager/provider_manager.go`: change `log.Info()` to `log.Debug()` for the "Initializing client for database-configured provider" log (~line 403)
- [ ] Verify embedding error logs still use `log.Error()` (no changes to error paths)
