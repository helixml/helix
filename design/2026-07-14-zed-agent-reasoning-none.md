# Zed agent reasoning `none`

A built-in Zed agent using `gpt-5.6-sol` failed before its first tool call. The
provider rejected function tools on `/v1/chat/completions` because the request
omitted `reasoning_effort`; the model's provider-side default enabled reasoning.

The assistant stored `reasoning_effort: "none"`, but Helix normalized that to an
empty value for every runtime. Claude Code and Codex intentionally use an empty
value to select their upstream default. Zed's custom OpenAI model instead needs
the explicit `none` value so it can serialize `reasoning_effort: "none"` on Chat
Completions requests.

The fix preserves `none` only for the built-in Zed runtime, includes it in the
generated custom model entry, and teaches Zed's native OpenAI provider to pass a
custom Chat Completions model's configured reasoning effort into the request.
