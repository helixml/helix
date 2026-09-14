import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
} from "../../api/api";
import ProjectCodeAgentDefaults from "./ProjectCodeAgentDefaults";

const controls = vi.hoisted(() => ({ props: [] as any[] }));

vi.mock("../agent/CodeAgentExecutionControls", () => ({
  default: (props: any) => {
    controls.props.push(props);
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
  beforeEach(() => {
    controls.props = [];
  });

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
          planning_code_agent_config: {
            runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
            credential_type:
              TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
            model: "claude-opus-5",
          },
        }}
        onUpdate={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(controls.props).toHaveLength(2);
    expect(controls.props[0].value.runtime).toBe(
      TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
    );
    expect(controls.props[0].value.model).toBe("claude-opus-5");
    expect(controls.props[1].value.runtime).toBe(
      TypesCodeAgentRuntime.CodeAgentRuntimeDeepSeekHarness,
    );
    expect(controls.props[1].value.model).toBe("qwen3.8-27b");
    // Ungrouped: the SettingRow around this supplies the "Agent" label, so the
    // control's own inline one would print it twice.
    expect(controls.props[0].grouped).toBeFalsy();
    expect(controls.props[1].grouped).toBeFalsy();
    // The whole point of this section is the agent picker, so computeOnly must
    // stay off — that flag is what hid the harness from project settings.
    expect(controls.props[0].computeOnly).toBeFalsy();
    expect(controls.props[1].computeOnly).toBeFalsy();
    expect(controls.props[0].spreadAgentControls).toBe(true);
    expect(controls.props[1].spreadAgentControls).toBe(true);
    // Compute is a sibling SettingRow rendered by ProjectTaskDefaults. Passing
    // no sandbox handlers is what makes this render the agent controls alone.
    expect(controls.props[0].onSandboxResourceOverridesChange).toBeUndefined();
    expect(controls.props[0].onSandboxRuntimeChange).toBeUndefined();
  });

  it("explains the recommended model tradeoff for each phase", () => {
    render(
      <ProjectCodeAgentDefaults project={{ id: "project-1" }} onUpdate={vi.fn()} />,
    );

    expect(
      screen.getByLabelText("Planning benefits from an intelligent model with high reasoning effort."),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText(
        "Implementation can use a faster, lower-cost model because it follows the approved plan.",
      ),
    ).toBeInTheDocument();
  });

  it("patches the selected phase when an agent changes", () => {
    const onUpdate = vi.fn().mockResolvedValue(undefined);
    render(
      <ProjectCodeAgentDefaults project={{ id: "project-1" }} onUpdate={onUpdate} />,
    );

    const buttons = screen.getAllByRole("button", { name: "Change agent" });
    fireEvent.click(buttons[0]);

    expect(onUpdate).toHaveBeenCalledTimes(1);
    expect(onUpdate).toHaveBeenCalledWith({
      planning_code_agent_config: {
        runtime: TypesCodeAgentRuntime.CodeAgentRuntimeOpenCode,
        credential_type:
          TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
        provider_ref: "provider-1",
        model: "qwen3.8-27b",
      },
    });

    fireEvent.click(buttons[1]);
    expect(onUpdate).toHaveBeenLastCalledWith({
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
