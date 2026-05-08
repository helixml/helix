# Design: Remove Tika from Helix

## Approach

Remove Tika completely and make "unstructured" (llamaindex) the sole text extraction backend. Since the `Extractor` interface and `DefaultExtractor` already exist, the change is straightforward: delete Tika-specific code and collapse the provider switch so the API always uses `DefaultExtractor`.

## Key Decisions

**Collapse the provider config vs keep the interface:** Remove the `TEXT_EXTRACTION_PROVIDER` config field and `ExtractorTika`/`ExtractorUnstructured` enums entirely. The `Extractor` interface can stay (it's clean and testable), but `serve.go` should directly instantiate `NewDefaultExtractor` without a switch. The `TextExtractor` config struct simplifies to just the `Unstructured.URL` field.

**What about the default URL?** The existing default for `TEXT_EXTRACTION_URL` is `http://llamaindex:5000/api/v1/extract`. This stays unchanged.

## Files to Modify

### Delete entirely
| File | Reason |
|------|--------|
| `api/pkg/extract/tika_extractor.go` | Tika extractor implementation |
| `api/pkg/extract/tika_extractor_test.go` | Tika extractor tests |
| `charts/helix-controlplane/templates/tika_deployment.yaml` | K8s deployment |
| `charts/helix-controlplane/templates/tika_svc.yaml` | K8s service |

### Modify
| File | Change |
|------|--------|
| `docker-compose.yaml` | Remove `tika` service (lines 83-87) |
| `docker-compose.dev.yaml` | Remove `tika` service (lines 209-212) |
| `api/pkg/config/config.go` | Remove `Provider` field, remove `Tika` struct from `TextExtractor` |
| `api/pkg/types/enums.go` | Remove `Extractor` type, `ExtractorTika`, `ExtractorUnstructured` constants |
| `api/cmd/helix/serve.go` | Replace switch with direct `NewDefaultExtractor(cfg.TextExtractor.Unstructured.URL)` |
| `api/pkg/config/proxy_test.go` | Remove `tikaURL` test field and all tika assertions |
| `.drone.yml` | Remove `TEXT_EXTRACTION_TIKA_URL` env vars (lines 180, 236, 299, 374, 450, 527, 607, 693) and tika docker service (lines 833-834) |
| `stack` | Remove `tika` from `docker compose up -d` command (line 2066) |
| `integration-test/api/.test.env` | Remove `TEXT_EXTRACTION_TIKA_URL` line |
| `charts/helix-controlplane/values.yaml` | Remove `tika:` section |
| `charts/helix-controlplane/values-example.yaml` | Remove tika section |
| `charts/helix-controlplane/templates/deployment.yaml` | Remove `TEXT_EXTRACTION_TIKA_URL` env var |
| `go.mod` / `go.sum` | Remove `github.com/google/go-tika` dependency (run `go mod tidy`) |

## Codebase Patterns Discovered

- The project uses an `Extractor` interface in `api/pkg/extract/extract.go` with `Extract(ctx, *Request) (string, error)`. Both `DefaultExtractor` (llamaindex/unstructured) and `TikaExtractor` implement it. The interface itself is fine to keep — it has a mockgen directive used for testing.
- Config uses `envconfig` struct tags with env var names and defaults.
- `serve.go` uses a switch on `cfg.TextExtractor.Provider` to select the extractor — this collapses to a single line.
- The `proxy_test.go` tests NO_PROXY configuration for internal service hostnames. Tika hostname should be removed from these test cases.
