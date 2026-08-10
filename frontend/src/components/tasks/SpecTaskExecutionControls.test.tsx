import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TypesCodeAgentRuntime } from "../../api/api";
import { AGENT_TYPE_ZED_EXTERNAL, IApp } from "../../types";
import SpecTaskExecutionControls from "./SpecTaskExecutionControls";

vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const codexAgent = {
  id: "app_codex",
  config: {
    helix: {
      name: "Codex",
      description: "",
      external_url: "",
      assistants: [{
        agent_type: AGENT_TYPE_ZED_EXTERNAL,
        code_agent_runtime: "codex_cli",
        code_agent_credential_type: "subscription",
        model: "gpt-5.6-sol",
        reasoning_effort: "medium",
      }],
    },
    secrets: {},
    allowed_domains: [],
  },
} as IApp;

const claudeAgent = {
  ...codexAgent,
  id: "app_claude",
  config: {
    ...codexAgent.config,
    helix: {
      ...codexAgent.config.helix,
      name: "Claude Code",
      assistants: [{
        agent_type: AGENT_TYPE_ZED_EXTERNAL,
        code_agent_runtime: "claude_code",
        code_agent_credential_type: "subscription",
        claude_subscription_model: "claude-opus-5",
      }],
    },
  },
} as IApp;

describe("SpecTaskExecutionControls", () => {
  it("uses a searchable provider-style model picker", async () => {
    const update = vi.fn();
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent, claudeAgent]}
        selectedAgentId={codexAgent.id}
        codeAgentOverrides={{}}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        onAgentModelChange={update}
        onSandboxResourceOverridesChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Change coding model" })).toHaveTextContent("GPT-5.6 Sol");
    expect(screen.getByRole("button", { name: "Change coding model" })).not.toHaveTextContent("Codex");
    fireEvent.click(screen.getByRole("button", { name: "Change coding model" }));
    expect(screen.getByRole("textbox", { name: "Search models" })).toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: "Search models" }), {
      target: { value: "terra" },
    });
    fireEvent.click(screen.getByRole("button", { name: /GPT-5.6 Terra.*Codex/ }));

    await waitFor(() => expect(update).toHaveBeenCalledWith(
      "app_codex",
      { provider_ref: "", model: "gpt-5.6-terra" },
    ));
  });

  it("uses coding agents in the left rail and models in the right pane", async () => {
    const update = vi.fn();
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent, claudeAgent]}
        selectedAgentId={codexAgent.id}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        onAgentModelChange={update}
        onSandboxResourceOverridesChange={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Change coding model" }));
    fireEvent.click(screen.getByRole("button", { name: "Claude Code" }));
    fireEvent.click(screen.getByRole("button", { name: /Claude Opus 5.*Claude Code/ }));

    await waitFor(() => expect(update).toHaveBeenCalledWith(
      "app_claude",
      { provider_ref: "", model: "claude-opus-5" },
    ));
  });

  it("lets a legacy task with a deleted agent switch to an available coding agent", async () => {
    const update = vi.fn();
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent, claudeAgent]}
        selectedAgentId="deleted-agent"
        currentExecutionConfig={{
          agent_id: "deleted-agent",
          agent_name: "codex",
          agent_available: false,
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
        }}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        onAgentModelChange={update}
        onSandboxResourceOverridesChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Change coding model" })).toHaveTextContent("codex model");
    expect(screen.queryByRole("button", { name: "Change reasoning and service tier" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Change coding model" }));
    fireEvent.click(screen.getByRole("button", { name: /GPT-5.6 Sol.*Codex/ }));

    await waitFor(() => expect(update).toHaveBeenCalledWith(
      "app_codex",
      { provider_ref: "", model: "gpt-5.6-sol" },
    ));
  });

  it("offers the supported sandbox presets through the CPU control", async () => {
    const resize = vi.fn();
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent]}
        selectedAgentId={codexAgent.id}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        onAgentModelChange={vi.fn()}
        onSandboxResourceOverridesChange={resize}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Change sandbox size" }));
    fireEvent.click(screen.getByRole("menuitem", { name: /8 vCPU.*16 GB RAM/ }));

    await waitFor(() => expect(resize).toHaveBeenCalledWith({ vcpus: 8, memory_mb: 16384 }));
  });

  it("shows the effective 4 vCPU default for legacy tasks without an override", () => {
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent]}
        selectedAgentId={codexAgent.id}
        onAgentModelChange={vi.fn()}
        onSandboxResourceOverridesChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Change sandbox size" })).toHaveTextContent("4 vCPU");
    expect(screen.queryByText("Uncapped")).not.toBeInTheDocument();
  });

  it("applies a live effort change immediately", async () => {
    const update = vi.fn().mockResolvedValue(undefined);
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent]}
        selectedAgentId={codexAgent.id}
        codeAgentOverrides={{}}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        onAgentModelChange={update}
        onSandboxResourceOverridesChange={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Change reasoning and service tier" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "High" }));

    await waitFor(() => expect(update).toHaveBeenCalledWith(
      "app_codex",
      { reasoning_effort: "high" },
    ));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("applies a same-agent model change immediately", async () => {
    const update = vi.fn().mockResolvedValue(undefined);
    render(
      <SpecTaskExecutionControls
        agents={[claudeAgent]}
        selectedAgentId={claudeAgent.id}
        codeAgentOverrides={{ model: "claude-opus-5" }}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        onAgentModelChange={update}
        onSandboxResourceOverridesChange={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Change coding model" }));
    fireEvent.click(screen.getByRole("button", { name: /Claude Sonnet.*Claude Code/ }));

    await waitFor(() => expect(update).toHaveBeenCalledWith(
      "app_claude",
      { provider_ref: "", model: "sonnet" },
    ));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
