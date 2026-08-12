import { describe, expect, it } from "vitest";

import {
  TypesCodeAgentRuntime,
  TypesInteractionState,
} from "../../api/api";
import { buildInteractionDebugContext } from "./interactionDebugContext";

describe("buildInteractionDebugContext", () => {
  it("captures interaction, tool, error, model, and session routing details", () => {
    const result = JSON.parse(
      buildInteractionDebugContext(
        {
          id: "int_123",
          session_id: "ses_123",
          state: TypesInteractionState.InteractionStateError,
          prompt_message: "Fix the failing build",
          response_message: "I could not finish",
          error: "agent disconnected",
          runner: "runner-1",
          usage: { total_tokens: 42 },
          response_entries: [
            {
              type: "tool_call",
              tool_name: "bash",
              tool_status: "failed",
              content: "exit 1",
            },
          ] as unknown as number[],
        },
        {
          id: "ses_123",
          model_name: "claude-opus-4-6",
          provider: "anthropic",
          config: {
            code_agent_runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
            spec_task_id: "st_123",
            zed_thread_id: "thread-123",
            callback_url: "https://example.test/private-callback",
          },
        },
        [
          {
            id: "step-1",
            interaction_id: "int_123",
            details: { arguments: { command: "go test ./..." } },
          },
        ],
        { version: "2.11.32", edition: "cloud" },
        {
          capturedAt: "2026-07-13T12:00:00.000Z",
          sourceUrl: "https://app.example.test/spec-tasks/st_123",
          userAgent: "test-browser",
        },
      ),
    );

    expect(result.format).toBe("helix-interaction-debug-context/v1");
    expect(result.interaction.error).toBe("agent disconnected");
    expect(result.interaction.response_entries[0].tool_name).toBe("bash");
    expect(result.session.model_name).toBe("claude-opus-4-6");
    expect(result.session.config.zed_thread_id).toBe("thread-123");
    expect(result.session.config.callback_url).toBeUndefined();
    expect(result.session_steps[0].details.arguments.command).toBe("go test ./...");
  });
});
