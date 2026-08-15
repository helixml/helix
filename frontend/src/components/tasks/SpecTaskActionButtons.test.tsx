import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import SpecTaskActionButtons, {
  SANDBOX_STOPPED_TOOLTIP,
  type SpecTaskForActions,
} from "./SpecTaskActionButtons";

// The component's data hooks all resolve to "nothing pending, nothing to
// connect" so the tests exercise the button-gating logic rather than mutation
// or OAuth state.
const idleMutation = { mutate: vi.fn(), isPending: false };

vi.mock("../../services/specTaskWorkflowService", () => ({
  useApproveImplementation: () => idleMutation,
  useStopAgent: () => idleMutation,
  useSkipSpec: () => idleMutation,
  useReopenTask: () => idleMutation,
}));

vi.mock("../../services/specTaskService", () => ({
  useUpdateSpecTask: () => idleMutation,
}));

vi.mock("../../services/oauthProvidersService", () => ({
  useListOAuthProviders: () => ({ data: [] }),
  useListOAuthConnections: () => ({ data: [] }),
}));

vi.mock("../../hooks/useOAuthFlow", () => ({
  useOAuthFlow: () => ({ startOAuthFlow: vi.fn(), isLoading: false }),
}));

const implementationTask = (
  overrides: Partial<SpecTaskForActions> = {},
): SpecTaskForActions => ({
  id: "spt_1",
  status: "implementation",
  branch_name: "feature/000345-can-we-look-into",
  base_branch: "master",
  last_push_at: "2026-08-15T07:30:00.000Z",
  sandbox_state: "running",
  ...overrides,
});

const openPRButton = () => screen.getByRole("button", { name: /Open PR/i });

describe.each(["inline", "stacked"] as const)(
  "SpecTaskActionButtons (%s) Open PR gating",
  (variant) => {
    const renderButtons = (task: SpecTaskForActions) =>
      render(
        <SpecTaskActionButtons task={task} variant={variant} hasExternalRepo />,
      );

    it("enables Open PR while the sandbox is running", () => {
      renderButtons(implementationTask());

      expect(openPRButton()).toBeEnabled();
    });

    // Approving an implementation tells the agent to push its branch from
    // inside the sandbox. The sandbox owns the working copy and may not even be
    // on a filesystem the control plane can reach, so with the sandbox stopped
    // there is nothing to push from.
    it("disables Open PR once the sandbox has stopped", () => {
      renderButtons(implementationTask({ sandbox_state: "absent" }));

      expect(openPRButton()).toBeDisabled();
    });

    // "starting" means the container exists but the agent has not connected
    // yet, so it cannot receive the push instruction either.
    it("disables Open PR while the sandbox is still starting", () => {
      renderButtons(implementationTask({ sandbox_state: "starting" }));

      expect(openPRButton()).toBeDisabled();
    });

    it("disables Open PR when the task never had a sandbox", () => {
      renderButtons(implementationTask({ sandbox_state: undefined }));

      expect(openPRButton()).toBeDisabled();
    });

    it("explains why Open PR is unavailable", async () => {
      renderButtons(implementationTask({ sandbox_state: "absent" }));

      // MUI renders the tooltip text into the trigger's aria description via
      // the title attribute on the wrapping span until hover; assert on the
      // accessible description the user gets either way.
      expect(
        await screen.findByLabelText(SANDBOX_STOPPED_TOOLTIP, { exact: false }),
      ).toBeInTheDocument();
    });

    it("still blocks Open PR before the agent has pushed", () => {
      renderButtons(
        implementationTask({ last_push_at: undefined, sandbox_state: "running" }),
      );

      expect(openPRButton()).toBeDisabled();
    });
  },
);
