# Implementation Tasks

## Docker Compose
- [x] Remove `tika` service from `docker-compose.yaml` (lines 83-87)
- [x] Remove `tika` service from `docker-compose.dev.yaml` (lines 209-212)

## Go Code
- [x] Delete `api/pkg/extract/tika_extractor.go`
- [x] Delete `api/pkg/extract/tika_extractor_test.go`
- [x] Remove `Extractor` type and `ExtractorTika`/`ExtractorUnstructured` constants from `api/pkg/types/enums.go`
- [x] Remove `Provider` field and `Tika` struct from `TextExtractor` in `api/pkg/config/config.go`
- [x] Simplify `serve.go` extractor init — direct `NewDefaultExtractor(cfg.TextExtractor.URL)`
- [x] Remove `tikaURL` field and tika-related assertions from `api/pkg/config/proxy_test.go`
- [x] Remove tika URL from `proxy.go` `InternalServiceURLs()`
- [x] Remove `github.com/google/go-tika` from `go.mod` via `go mod tidy`

## CI / Scripts
- [x] Remove all `TEXT_EXTRACTION_TIKA_URL` env vars from `.drone.yml` (active and commented-out)
- [x] Remove `tika` docker service from `.drone.yml`
- [x] Remove `tika` from `docker compose up -d` in `stack` script
- [x] Remove `TEXT_EXTRACTION_TIKA_URL` export from `stack` script

## Integration Tests
- [x] Remove `TEXT_EXTRACTION_TIKA_URL` from `integration-test/api/.test.env`

## Helm Charts
- [x] Delete `charts/helix-controlplane/templates/tika_deployment.yaml`
- [x] Delete `charts/helix-controlplane/templates/tika_svc.yaml`
- [x] Remove `tika:` section from `charts/helix-controlplane/values.yaml`
- [x] Remove tika section from `charts/helix-controlplane/values-example.yaml`
- [x] Remove `TEXT_EXTRACTION_TIKA_URL` env var from `charts/helix-controlplane/templates/deployment.yaml`

## Documentation
- [x] Remove tika from `local-development.md` (container list + test command)
- [x] Remove tika from `CONTRIBUTING.md` (test command)
- [x] Remove tika from `README.md` (architecture diagram)
- [x] Remove tika from `scripts/docker-image-cleanup-node04.sh` (comment)

## Verification
- [x] `go build` succeeds for all changed packages
- [x] Grep confirms no remaining active tika references (only historical design doc `design/2025-01-07-haystack-rag-integration.md`)
