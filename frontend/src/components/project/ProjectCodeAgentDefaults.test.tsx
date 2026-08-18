import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
} from "../../api/api";
import ProjectCodeAgentDefaults from "./ProjectCodeAgentDefaults";

const controls = vi.hoisted(() => ({ props: undefined as any }));

vi.mock("../agent/CodeAgentExecutionControls", () => ({
  default: (props: any) => {
    controls.props = props;
    return (
      <button
        onClick={() =>
          props.onChange({
            runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
            credential_type:
              TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
            provider_ref: "provider-1",
            model: "qwen3.8-27b",
          })
        }
      >
        Change agent
      </button>
    );
  },
}));

describe("ProjectCodeAgentDefaults", () => {
  it("renders the project's stored harness and model", () => {
    render(
      <ProjectCodeAgentDefaults
        project={{
          id: "project-1",
          code_agent_config: {
            runtime: TypesCodeAgentRuntime.CodeAgentRuntimeDeepSeekHarness,
            credential_type:
              TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
            provider_ref: "provider-1",
            model: "qwen3.8-27b",
          },
        }}
        onUpdate={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(controls.props.value.runtime).toBe(
      TypesCodeAgentRuntime.CodeAgentRuntimeDeepSeekHarness,
    );
    expect(controls.props.value.model).toBe("qwen3.8-27b");
    expect(controls.props.grouped).toBe(true);
    // The whole point of this section is the agent picker, so computeOnly must
    // stay off — that flag is what hid the harness from project settings.
    expect(controls.props.computeOnly).toBeFalsy();
    // Compute lives on the Sandbox tab. Passing no sandbox handlers is what
    // makes this render the agent row alone.
    expect(controls.props.onSandboxResourceOverridesChange).toBeUndefined();
    expect(controls.props.onSandboxRuntimeChange).toBeUndefined();
  });

  it("patches only code_agent_config when the agent changes", () => {
    const onUpdate = vi.fn().mockResolvedValue(undefined);
    render(
      <ProjectCodeAgentDefaults project={{ id: "project-1" }} onUpdate={onUpdate} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Change agent" }));

    expect(onUpdate).toHaveBeenCalledTimes(1);
    expect(onUpdate).toHaveBeenCalledWith({
      code_agent_config: {
        runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
        credential_type:
          TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
        provider_ref: "provider-1",
        model: "qwen3.8-27b",
      },
    });
  });
});
