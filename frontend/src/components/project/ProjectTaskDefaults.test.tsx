import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
  TypesSandboxRuntime,
} from "../../api/api";
import { IApp } from "../../types";
import ProjectTaskDefaults from "./ProjectTaskDefaults";

const controls = vi.hoisted(() => ({ props: undefined as any }));

vi.mock("../tasks/SpecTaskExecutionControls", () => ({
  default: (props: any) => {
    controls.props = props;
    return (
      <div>
        <button
          onClick={() =>
            props.onAgentModelChange("app-codex", {}, {
              runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
              credential_type:
                TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
              model: "gpt-5.6-sol",
              reasoning_effort: "high",
            })
          }
        >
          Change model
        </button>
        <button
          onClick={() =>
            props.onSandboxResourceOverridesChange({
              vcpus: 8,
              memory_mb: 16384,
            })
          }
        >
          Change compute
        </button>
        <button
          onClick={() =>
            props.onSandboxRuntimeChange(
              TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
            )
          }
        >
          Change environment
        </button>
      </div>
    );
  },
}));

const zedAgent = {
  id: "app-zed",
  config: {
    helix: {
      name: "Zed Agent",
      assistants: [
        {
          agent_type: "zed_external",
          code_agent_runtime: "zed_agent",
          code_agent_credential_type: "api_key",
          provider: "provider-1",
          model: "model-1",
          reasoning_effort: "high",
        },
      ],
    },
  },
} as IApp;

describe("ProjectTaskDefaults", () => {
  it("round-trips project-owned execution defaults without an App ID", async () => {
    const onUpdate = vi.fn().mockResolvedValue(undefined);
    render(
      <ProjectTaskDefaults
        agents={[zedAgent]}
        project={{
          id: "project-1",
          code_agent_config: {
            runtime: TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent,
            credential_type:
              TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
            provider_ref: "provider-1",
            model: "model-1",
            reasoning_effort: "high",
          },
          default_sandbox_runtime:
            TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop,
          default_sandbox_resource_overrides: {
            vcpus: 4,
            memory_mb: 8192,
          },
        }}
        onUpdate={onUpdate}
      />,
    );

    expect(controls.props.selectedAgentId).toBe("app-zed");
    expect(controls.props.currentExecutionConfig.provider_ref).toBe(
      "provider-1",
    );
    expect(controls.props.currentExecutionConfig.model).toBe("model-1");
    expect(controls.props.grouped).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Change model" }));
    fireEvent.click(screen.getByRole("button", { name: "Change compute" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Change environment" }),
    );

    expect(onUpdate).toHaveBeenNthCalledWith(1, {
      code_agent_config: {
        runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
        credential_type:
          TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
        model: "gpt-5.6-sol",
        reasoning_effort: "high",
      },
    });
    expect(onUpdate).toHaveBeenNthCalledWith(2, {
      default_sandbox_resource_overrides: {
        vcpus: 8,
        memory_mb: 16384,
      },
    });
    expect(onUpdate).toHaveBeenNthCalledWith(3, {
      default_sandbox_runtime:
        TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
    });
    expect(onUpdate.mock.calls.flat()).not.toContainEqual(
      expect.objectContaining({ default_helix_app_id: expect.anything() }),
    );
  });
});
