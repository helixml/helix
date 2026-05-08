# Requirements: Remove Tika from Helix

## Background

Apache Tika is a text extraction service used to extract text from documents (PDFs, etc.). It runs as a Docker sidecar (`apache/tika:2.9.2.1`) and is accessed via the `TEXT_EXTRACTION_TIKA_URL` env var. The codebase also supports an alternative extractor ("unstructured"/llamaindex). Tika should be fully removed — the service, configuration, code, tests, Helm charts, and CI references.

## User Stories

- As a platform operator, I want Tika removed from all docker-compose files so it no longer starts as a service.
- As a developer, I want Tika-related Go code, config, and env vars removed so the codebase is simpler.
- As a Helm user, I want the Tika Kubernetes templates and values removed from the chart.

## Acceptance Criteria

1. No `tika` service in `docker-compose.yaml` or `docker-compose.dev.yaml`.
2. No `TEXT_EXTRACTION_TIKA_URL` env var anywhere (`.drone.yml`, `stack`, `.test.env`, Helm templates).
3. No `TEXT_EXTRACTION_PROVIDER` config or `ExtractorTika` enum — the "unstructured" extractor becomes the only option (no provider switch needed).
4. `tika_extractor.go`, `tika_extractor_test.go` deleted.
5. `go-tika` Go module dependency removed from `go.mod`/`go.sum`.
6. Helm chart files `tika_deployment.yaml` and `tika_svc.yaml` deleted; tika values removed from `values.yaml` and `values-example.yaml`; tika env var removed from `deployment.yaml` template.
7. `proxy_test.go` updated to remove `tikaURL` test field.
8. `stack` script no longer starts the tika service.
9. `serve.go` no longer switches on extractor provider — directly uses `DefaultExtractor` (unstructured).
10. `go build` succeeds, existing tests pass.

## Out of Scope

- The model entry `openrouter/cinematika-7b` in `model_info.json` is unrelated and must NOT be touched.
- The `nvidia-container-toolkit` (CTK) references in `install.sh` are unrelated.
- The `GTK_IM_MODULE` references in desktop configs are unrelated.
- The `tk` loop variable in `zapier.go` is unrelated.
