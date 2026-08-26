# OpenCode image attachment capability

## Root cause

Helix uploaded chat and spec-task image attachments correctly, and OpenCode's
`read` tool returned the PNG as an image content block. The generated OpenCode
custom-model configuration omitted `attachment` and `modalities`, however.
OpenCode therefore treated the custom model as text-only even when the upstream
model accepted image input.

Plain OpenAI-compatible `/v1/models` responses do not consistently advertise
modalities. The Qwen 3.8 27B endpoint used to reproduce the failure returns its
context length but no modality metadata, while a direct image request succeeds.

## Fix

- Carry input and output modalities in `CodeAgentConfig`.
- Prefer provider-advertised modalities, then catalogue metadata.
- Refresh the bundled OpenRouter catalogue with Qwen 3.8 27B metadata for
  providers whose own model listing omits modalities.
- Generate OpenCode custom-model `attachment: true` and explicit input/output
  modalities only when Helix knows the model accepts a non-text input.
- Leave unknown models unchanged instead of guessing attachment support.

## Verification

Local Helix CLI E2E on 2026-08-24:

- Built and deployed `helix-ubuntu:7caeb6`.
- Forked a real sample repository with OpenCode and Qwen 3.8 27B.
- Attached `cowsay.png` through `helix spectask attach`.
- Started a new desktop session through `helix spectask start --wait`.
- Confirmed the API and generated OpenCode config contained text/image input,
  text output, and `attachment: true`.
- The live OpenCode agent called `read` on the PNG and correctly returned the
  sentence `< I want to break freee >` and identified the ASCII animal as a cow.
- The proxied multimodal request contained both `image_url` and `data:image`
  content in the recorded LLM call.
