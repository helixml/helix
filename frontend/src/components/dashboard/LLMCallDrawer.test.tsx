import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TypesLLMCall } from "../../api/api";
import LLMCallDrawer, { formatTokenCount } from "./LLMCallDrawer";

vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const openAICall: TypesLLMCall = {
  id: "llmc_test1",
  created: "2026-08-15T12:00:00Z",
  model: "qwen3.8-27b",
  provider: "vllm",
  session_id: "ses_test1",
  duration_ms: 4200,
  time_to_first_token_ms: 800,
  prompt_tokens: 188049,
  completion_tokens: 828,
  total_tokens: 188877,
  stream: true,
  request: {
    messages: [
      { role: "system", content: "You are helpful." },
      { role: "user", content: "What is the weather in London?" },
    ],
    tools: [{ type: "function", function: { name: "get_weather" } }],
    max_tokens: 4096,
    stream: true,
  } as any,
  response: {
    choices: [{
      finish_reason: "tool_calls",
      message: {
        role: "assistant",
        tool_calls: [{
          id: "call_1",
          type: "function",
          function: { name: "get_weather", arguments: '{"city":"London"}' },
        }],
      },
    }],
  } as any,
};

const anthropicCall: TypesLLMCall = {
  id: "llmc_test2",
  model: "claude-opus-4-8",
  provider: "anthropic",
  response: {
    content: [
      { type: "text", text: "Hello from Claude" },
      { type: "tool_use", name: "read_file", input: { path: "/tmp/x" } },
    ],
    stop_reason: "tool_use",
  } as any,
};

describe("LLMCallDrawer", () => {
  it("renders metadata, token stats, and request summary for an OpenAI-shaped call", () => {
    render(<LLMCallDrawer call={openAICall} open onClose={() => {}} />);

    expect(screen.getByText("qwen3.8-27b")).toBeTruthy();
    // token stats
    expect(screen.getByText("188.0k")).toBeTruthy();
    expect(screen.getByText("828")).toBeTruthy();
    // duration formatting
    expect(screen.getByText("4.2s")).toBeTruthy();
    // request summary derived from the request JSON
    expect(screen.getByText("Messages")).toBeTruthy();
    expect(screen.getByText("Tools")).toBeTruthy();
    // response summary
    expect(screen.getByText("tool_calls")).toBeTruthy();
    expect(screen.getByText("get_weather")).toBeTruthy();
    // streaming indicator
    expect(screen.getByText("stream")).toBeTruthy();
  });

  it("summarizes Anthropic-shaped responses (content blocks)", () => {
    render(<LLMCallDrawer call={anthropicCall} open onClose={() => {}} />);

    expect(screen.getByText("Hello from Claude")).toBeTruthy();
    expect(screen.getByText("read_file")).toBeTruthy();
    expect(screen.getByText("tool_use")).toBeTruthy();
  });

  it("shows the error prominently for failed calls", () => {
    render(
      <LLMCallDrawer
        call={{ id: "llmc_err", model: "m", error: "status code: 400, reasoning effort" }}
        open
        onClose={() => {}}
      />,
    );
    // Appears in both the error banner and the response JSON fallback.
    expect(screen.getAllByText(/reasoning effort/).length).toBeGreaterThan(0);
  });
});

describe("formatTokenCount", () => {
  it("formats counts into readable units", () => {
    expect(formatTokenCount(undefined)).toBe("-");
    expect(formatTokenCount(828)).toBe("828");
    expect(formatTokenCount(188049)).toBe("188.0k");
    expect(formatTokenCount(2500000)).toBe("2.50M");
  });
});
