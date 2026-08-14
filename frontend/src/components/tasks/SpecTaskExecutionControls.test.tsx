import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TypesCodeAgentRuntime, TypesSandboxRuntime } from "../../api/api";
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

  it("shows the active harness icon and name when hovering the model control", async () => {
    render(
      <SpecTaskExecutionControls
        agents={[]}
        selectedAgentId=""
        currentExecutionConfig={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
          model: "deepseek-v4-flash",
        }}
        onAgentModelChange={vi.fn()}
        onSandboxResourceOverridesChange={vi.fn()}
      />,
    );

    fireEvent.mouseOver(screen.getByRole("button", { name: "Change coding model" }));

    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("opencode");
    expect(tooltip.querySelector('[data-harness-mark="opencode"]')).toBeInTheDocument();
  });

  it("groups harness, model, and compute controls for task details", () => {
    render(
      <SpecTaskExecutionControls
        agents={[]}
        selectedAgentId=""
        currentExecutionConfig={{
          runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
          model: "deepseek-v4-flash",
          reasoning_effort: "medium",
        }}
        onAgentModelChange={vi.fn()}
        onSandboxResourceOverridesChange={vi.fn()}
        grouped
      />,
    );

    const executionConfig = screen.getByLabelText("Execution configuration");
    expect(executionConfig).toHaveTextContent("Harness:");
    expect(executionConfig).toHaveTextContent("Model:");
    expect(executionConfig).toHaveTextContent("Compute:");
    expect(executionConfig).toHaveTextContent("deepseek-v4-flash");
    expect(executionConfig).toHaveTextContent("Medium");
    expect(executionConfig).toHaveTextContent("4 vCPU");
    const harness = screen.getByLabelText("Harness: opencode");
    expect(harness).toHaveTextContent("opencode");
    expect(harness.querySelector('[data-harness-mark="opencode"]')).toBeInTheDocument();
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

  it("offers full desktop and headless environments when creating a task", async () => {
    const setRuntime = vi.fn();
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent]}
        selectedAgentId={codexAgent.id}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        sandboxRuntime={TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop}
        onAgentModelChange={vi.fn()}
        onSandboxResourceOverridesChange={vi.fn()}
        onSandboxRuntimeChange={setRuntime}
      />,
    );

    const computeButton = screen.getByRole("button", { name: "Change sandbox size" });
    expect(computeButton.querySelector(".lucide-monitor"))
      .toBeInTheDocument();
    fireEvent.click(computeButton);
    expect(screen.getByRole("menuitem", { name: /Full Desktop.*Selected/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("menuitem", { name: "Headless" }));

    await waitFor(() => expect(setRuntime).toHaveBeenCalledWith(
      TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
    ));
  });

  it("omits the desktop icon from the collapsed control for headless tasks", () => {
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent]}
        selectedAgentId={codexAgent.id}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        sandboxRuntime={TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu}
        onAgentModelChange={vi.fn()}
        onSandboxResourceOverridesChange={vi.fn()}
        onSandboxRuntimeChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Change sandbox size" }).querySelector(".lucide-monitor"))
      .not.toBeInTheDocument();
  });

  it("shows a locked environment with guidance after task creation", async () => {
    render(
      <SpecTaskExecutionControls
        agents={[codexAgent]}
        selectedAgentId={codexAgent.id}
        sandboxResourceOverrides={{ vcpus: 4, memory_mb: 8192 }}
        sandboxRuntime={TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu}
        onAgentModelChange={vi.fn()}
        onSandboxResourceOverridesChange={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Change sandbox size" }));
    const lockedRuntime = screen.getByRole("menuitem", { name: /Headless.*Selected/ });
    expect(lockedRuntime).toHaveAttribute("aria-disabled", "true");
    fireEvent.mouseOver(lockedRuntime);

    expect(await screen.findByText(
      "Sandbox environment can't be changed after the task starts. Start a new task to use a different environment.",
    )).toBeInTheDocument();
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
