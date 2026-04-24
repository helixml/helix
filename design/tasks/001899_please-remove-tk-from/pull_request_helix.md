# Remove Apache Tika text extraction service

## Summary
Remove Apache Tika and all references to it from the codebase. The "unstructured" (llamaindex) extractor is now the sole text extraction backend, simplifying the configuration and removing an unnecessary Docker service.

## Changes
- **Docker Compose**: Removed `tika` service from both `docker-compose.yaml` and `docker-compose.dev.yaml`
- **Go code**: Deleted `tika_extractor.go` and tests, removed `Extractor` type enum, simplified `TextExtractor` config struct (single `URL` field instead of provider switch), collapsed extractor initialization in `serve.go` to directly use `DefaultExtractor`
- **CI**: Removed `TEXT_EXTRACTION_TIKA_URL` env vars and tika docker service from `.drone.yml`
- **Scripts**: Removed tika from `stack` script (docker compose up command and env var export)
- **Helm charts**: Deleted `tika_deployment.yaml` and `tika_svc.yaml` templates, removed tika values and env var from chart configs
- **Integration tests**: Removed `TEXT_EXTRACTION_TIKA_URL` from `.test.env`
- **Docs**: Updated `README.md`, `CONTRIBUTING.md`, `local-development.md` to remove tika mentions
- **Dependencies**: Removed `github.com/google/go-tika` from `go.mod`

## Breaking Changes
- `TEXT_EXTRACTION_PROVIDER` and `TEXT_EXTRACTION_TIKA_URL` env vars are no longer recognized
- The `tika` Helm chart values are removed — deployments using `tika.enabled: true` will need to remove that config
