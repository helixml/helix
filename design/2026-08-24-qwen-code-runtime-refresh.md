# Qwen Code runtime refresh

Helix installs the official bundled `@qwen-code/qwen-code` npm release into
desktop images. `QWEN_VERSION` in `sandbox-versions.txt` is the only version
pin; the build no longer depends on an adjacent `helixml/qwen-code` checkout.

To upgrade:

1. Change `QWEN_VERSION` in `sandbox-versions.txt`.
2. Run `./stack build-ubuntu`.
3. Start a new Qwen Code task and complete a live ACP turn against a supported
   model. Existing sessions retain their old desktop container.

The settings daemon starts `qwen --acp`, points `QWEN_HOME` and
`QWEN_RUNTIME_DIR` at persistent workspace storage, disables telemetry and
automatic updates, and forwards the task's reasoning effort through Zed's
`default_config_options`. It also writes the same tier to Qwen's documented
`model.generationConfig.extra_body.reasoning_effort` setting. That preserves
the provider's required top-level OpenAI-compatible wire shape for self-hosted
models instead of relying on Qwen's model-family-specific reasoning mapping.
Unrelated workspace Qwen settings are preserved.

Qwen Code remains a first-class `qwen_code` runtime in the backend harness
catalogue and the frontend picker. The existing `AgentHarness` asset provides
its canonical icon and labels.
