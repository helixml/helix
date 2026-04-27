# Implementation Tasks

- [x] In `api/pkg/openai/openai_client.go` `openAIClientInterceptor.Do()` (~line 722): add status code check so `reasoningFieldMapper` only wraps 2xx response bodies
- [x] In `api/pkg/openai/openai_client.go` `listOpenAIModels()` (~line 458): read and include the response body in the non-200 error message (match the pattern used in `listAnthropicModels()`)
- [x] Write a unit test for `openAIClientInterceptor.Do()` verifying that a 401 response with plain-text body passes through without JSON parsing errors
- [x] Write a unit test for `listOpenAIModels()` verifying the error message includes the provider's response body text
- [x] Manual test: configure an NVIDIA provider with an invalid API key, verify the error message shows the 401 status and upstream text instead of "invalid character"
- [x] Write per-repo PR description (`pull_request_helix.md`)
