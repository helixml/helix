# Implementation Tasks

## Docker Compose
- [x] Remove `tika` service from `docker-compose.yaml` (lines 83-87)
- [x] Remove `tika` service from `docker-compose.dev.yaml` (lines 209-212)

## Go Code
- [~] Delete `api/pkg/extract/tika_extractor.go`
- [~] Delete `api/pkg/extract/tika_extractor_test.go`
- [~] Remove `Extractor` type and `ExtractorTika`/`ExtractorUnstructured` constants from `api/pkg/types/enums.go` (lines 304-309)
- [~] Remove `Provider` field and `Tika` struct from `TextExtractor` in `api/pkg/config/config.go` (lines 322-332)
- [~] Simplify `serve.go` extractor init — replace switch (lines 313-320) with direct `extractor = extract.NewDefaultExtractor(cfg.TextExtractor.Unstructured.URL)`
- [~] Remove `tikaURL` field and tika-related assertions from `api/pkg/config/proxy_test.go`
- [~] Remove `github.com/google/go-tika` from `go.mod` — run `go mod tidy`

## CI / Scripts
- [ ] Remove all `TEXT_EXTRACTION_TIKA_URL` env vars from `.drone.yml` (active and commented-out, lines 180, 236, 299, 374, 450, 527, 607, 693)
- [ ] Remove `tika` docker service from `.drone.yml` (lines 833-834)
- [ ] Remove `tika` from `docker compose up -d` in `stack` script (line 2066)

## Integration Tests
- [ ] Remove `TEXT_EXTRACTION_TIKA_URL` from `integration-test/api/.test.env` (line 6)

## Helm Charts
- [ ] Delete `charts/helix-controlplane/templates/tika_deployment.yaml`
- [ ] Delete `charts/helix-controlplane/templates/tika_svc.yaml`
- [ ] Remove `tika:` section from `charts/helix-controlplane/values.yaml` (lines 28-32)
- [ ] Remove tika section from `charts/helix-controlplane/values-example.yaml` (lines 373-380)
- [ ] Remove `TEXT_EXTRACTION_TIKA_URL` env var from `charts/helix-controlplane/templates/deployment.yaml` (lines 161-162)

## Verification
- [ ] Run `go build ./...` from the helix root to confirm compilation
- [ ] Run `go vet ./...` to check for issues
- [ ] Grep entire repo for remaining `tika` references (excluding `cinematika` model name) and fix any stragglers
