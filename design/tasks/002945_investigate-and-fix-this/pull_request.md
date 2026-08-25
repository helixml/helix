# test(api): expect deferred config for all runtimes

## Summary

`isDeferredNativeHarnessProjectConfig` was broadened in `f0e69c1b1` ("allow deferred model selection for all code agent runtimes in project creation") to return `true` for any API-key config without an explicit provider/model, covering all runtimes — not just Claude and Codex. The existing unit test `TestIsDeferredNativeHarnessProjectConfig/generic_harness` still asserted the old native-only behaviour (`want: false` for the `OpenCode` runtime), causing CI build 3964 to fail with `expected: false, actual: true`.

This commit updates the `generic_harness` test case to `want: true`, matching the intended "all runtimes" semantics.

## Testing

- `go test ./pkg/server/ -run 'TestIsDeferredNativeHarnessProjectConfig|TestAppDefaultsUseNativeHarnessExecution' -count=1` — all subtests pass.
