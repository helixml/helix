import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import SwitchAgentControl from "./SwitchAgentControl";

vi.mock("../../hooks/useApps", () => ({
  default: () => ({
    apps: [
      {
        id: "app_claude",
        config: {
          helix: {
            name: "Claude Code",
            assistants: [{ code_agent_runtime: "claude_code" }],
          },
        },
      },
      {
        id: "app_codex",
        config: {
          helix: {
            name: "Codex",
            assistants: [{ code_agent_runtime: "codex_cli" }],
          },
        },
      },
    ],
  }),
}));

const { disabledRuntimes } = vi.hoisted(() => ({ disabledRuntimes: new Set<string>() }));
vi.mock("../../services/codeAgentHarnessesService", () => ({
  useIsCodeAgentRuntimeEnabled: () => (runtime?: string) => !disabledRuntimes.has(runtime || ""),
}));

vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ success: vi.fn() }),
}));

vi.mock("../../services/projectService", () => ({
  useListProjectSpecTaskAgents: () => ({
    data: [
      { id: "app_claude", name: "Claude Code" },
      { id: "app_codex", name: "Codex" },
    ],
  }),
}));

vi.mock("../../services/sessionService", () => ({
  useGetSession: () => ({
    data: { data: { parent_app: "app_claude", config: {} } },
  }),
  useSwitchAgent: () => ({ mutateAsync: vi.fn() }),
}));

describe("SwitchAgentControl", () => {
  beforeEach(() => disabledRuntimes.clear());

  it("uses the project allow-list when legacy app records omit agent_kind", () => {
    render(
      <SwitchAgentControl
        sessionId="session-one"
        projectId="project-one"
        displayMode="compact"
      />,
    );

    const button = screen.getByRole("button", { name: "Switch agent" });
    expect(button).toBeEnabled();
    expect(within(button).getByText("Claude Code")).toBeInTheDocument();

    fireEvent.click(button);
    expect(screen.getByRole("menuitem", { name: /Claude Code/ })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /Codex/ })).toBeInTheDocument();
  });

  it("hides agents whose runtime the organization disabled", () => {
    disabledRuntimes.add("codex_cli");
    render(
      <SwitchAgentControl
        sessionId="session-one"
        projectId="project-one"
        displayMode="compact"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Switch agent" }));
    expect(screen.getByRole("menuitem", { name: /Claude Code/ })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /Codex/ })).not.toBeInTheDocument();
  });
});
