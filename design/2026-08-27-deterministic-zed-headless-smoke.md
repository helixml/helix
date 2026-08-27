# Deterministic Zed headless smoke test

Drone build 4001 failed in `zed-e2e-headless-smoke` after the configured
Bunker endpoint returned HTTP 502. Builds 3995, 3996, and 3998–4000 failed the
same step because the configured model endpoint was unavailable. The
credential-free headless plan and lifecycle tests passed in those builds.

The smoke test is a product/protocol gate, not a provider health check. Its
native Zed agent previously depended on a live model to choose `read_file` and
reply with a fixture value, so provider availability and model behavior could
fail an otherwise unrelated change.

The E2E server now exposes a local OpenAI-compatible streaming endpoint in
smoke mode. Its first response requests `read_file` for the fixture, and its
second response validates that the tool result contains the expected value
before completing the turn. CI no longer injects model credentials into this
step. The test still covers the native agent, tool execution, WebSocket sync,
streaming, and interaction completion.
