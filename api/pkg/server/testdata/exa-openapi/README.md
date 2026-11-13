# Exa OpenAPI spec (test oracle)

`exa-openapi-spec.yaml` is Exa's published OpenAPI document, vendored unmodified.
`websearch_exa_spec_test.go` validates every request the web search integration
test sends, and every response Helix returns, against it.

- Source: https://github.com/exa-labs/openapi-spec
- Commit: `ebdf275f7272274a8d8592d62c80f7dd0196e6aa` (2026-11-02T01:28:26Z)
- URL: https://raw.githubusercontent.com/exa-labs/openapi-spec/ebdf275f7272274a8d8592d62c80f7dd0196e6aa/exa-openapi-spec.yaml
- sha256: `f21ac0e3accc1cb1be6c902d73d86972f4bc1172925b5e34aa13d9b7ce9a5436`

The document is OpenAPI 3.1; the kin-openapi version Helix pins reads 3.0. The
test rewrites the 3.1 constructs at load time, each to its exact 3.0
equivalent, and never edits this file:

| 3.1 | 3.0 |
|---|---|
| `const: v` | `enum: [v]` |
| `type: "null"` | `nullable: true, enum: [null]` (matches `null` and nothing else) |
| `exclusiveMinimum: n` | `minimum: n, exclusiveMinimum: true` |
| `examples: [...]` | `example: <first>` (annotation only) |
| `propertyNames` | dropped (no 3.0 equivalent; on these endpoints it is always `{type: string}`, which every JSON object key satisfies) |

The deliberate differences between Helix and this spec are applied in
`exaSpecHelixDifferences`, each with its reason.

To update: download the new commit's file over this one, update the commit,
date and hash above, and run `go test -run TestWebSearchIntegration ./pkg/server/`.
