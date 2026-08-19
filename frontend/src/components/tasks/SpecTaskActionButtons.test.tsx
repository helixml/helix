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

    // The commits already reached the control plane's copy of the repo, so it
    // can push them to the remote and open the PR without the sandbox. This is
    // the common "agent finished, container was reaped" case.
    it("enables Open PR once the sandbox has stopped, if the agent pushed", () => {
      renderButtons(implementationTask({ sandbox_state: "absent" }));

      expect(openPRButton()).toBeEnabled();
    });

    // Nothing has reached the server, but a live agent can still be told to
    // commit and push before the PR opens.
    it("enables Open PR before the first push while the sandbox is live", () => {
      renderButtons(
        implementationTask({ last_push_at: undefined, sandbox_state: "running" }),
      );

      expect(openPRButton()).toBeEnabled();
    });

    // Neither source of commits exists: nothing pushed, and no agent to push.
    it("disables Open PR when nothing was pushed and the sandbox is gone", () => {
      renderButtons(
        implementationTask({ last_push_at: undefined, sandbox_state: "absent" }),
      );

      expect(openPRButton()).toBeDisabled();
    });

    // "starting" means the container exists but the agent has not connected
    // yet, so it cannot receive the commit-and-push instruction.
    it("disables Open PR before the first push while the sandbox is starting", () => {
      renderButtons(
        implementationTask({ last_push_at: undefined, sandbox_state: "starting" }),
      );

      expect(openPRButton()).toBeDisabled();
    });

    it("disables Open PR when the task never had a sandbox or a push", () => {
      renderButtons(
        implementationTask({ last_push_at: undefined, sandbox_state: undefined }),
      );

      expect(openPRButton()).toBeDisabled();
    });

    it("explains why Open PR is unavailable", async () => {
      renderButtons(
        implementationTask({ last_push_at: undefined, sandbox_state: "absent" }),
      );

      // MUI renders the tooltip text into the trigger's aria description via
      // the title attribute on the wrapping span until hover; assert on the
      // accessible description the user gets either way.
      expect(
        await screen.findByLabelText(SANDBOX_STOPPED_TOOLTIP, { exact: false }),
      ).toBeInTheDocument();
    });
  },
);

// sandbox_state is REQUIRED on SpecTaskForActions, and this is what keeps it
// that way. Every test above supplies it, which is precisely why the suite
// stayed green while both real call sites — SpecTaskDetailContent.renderTaskActions
// and TaskCard — rebuilt the task as a subset object and dropped it. An absent
// field reads as "sandbox stopped", which disabled Open PR while Reject (which
// does not consult it) stayed enabled: the button was permanently grey on
// desktop and mobile alike, with no way to merge from the UI.
//
// If someone makes the field optional again, the @ts-expect-error below becomes
// an unused suppression and the type-check fails. That is deliberate: this class
// of bug is invisible to a runtime test that constructs its own fixtures.
describe("SpecTaskForActions", () => {
  it("requires sandbox_state so a call site cannot silently omit it", () => {
    // @ts-expect-error - sandbox_state is required and deliberately missing here
    const missingSandboxState: SpecTaskForActions = {
      id: "spt_1",
      status: "implementation",
      branch_name: "feature/x",
      base_branch: "main",
      last_push_at: "2026-08-19T11:18:13.000Z",
    };
    expect(missingSandboxState.id).toBe("spt_1");
  });

  // The production case from 2026-08-19: the agent had pushed and the sandbox
  // was live, but the call site never forwarded sandbox_state.
  it("enables Open PR when the sandbox is running and the agent has pushed", () => {
    render(
      <SpecTaskActionButtons
        task={implementationTask({ sandbox_state: "running" })}
        hasExternalRepo
        externalRepoType="github"
      />,
    );
    expect(screen.getByRole("button", { name: /Open PR/i })).toBeEnabled();
  });
});
