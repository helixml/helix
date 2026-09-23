import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { InteractionInference } from "./InteractionInference";

const { navigate } = vi.hoisted(() => ({ navigate: vi.fn() }));

// Only the chrome around the error block matters here, so the heavy content
// renderers are stubbed out. The error block itself is the real thing.
vi.mock("./Markdown", () => ({ default: () => <div /> }));
vi.mock("./WorkLog", () => ({ default: () => <div /> }));
vi.mock("./ActivitySummary", () => ({ default: () => <div /> }));
vi.mock("./PlanProgress", () => ({ SessionPlanProgress: () => <div /> }));
vi.mock("./ToolStepsWidget", () => ({ default: () => <div /> }));
vi.mock("./WorkspaceReviewMessage", () => ({ default: () => <div /> }));
vi.mock("./InteractionDebugCopyButton", () => ({ default: () => <div /> }));
vi.mock("./MessageReceivedTimestamp", () => ({ default: () => <div /> }));
vi.mock("./CopyButtonWithCheck", () => ({ default: () => <div /> }));
vi.mock("./ImageLightbox", () => ({ default: () => <div /> }));
vi.mock("../export/ExportDocument", () => ({ default: () => <div /> }));
vi.mock("../export/ToPDF", () => ({ default: () => <div /> }));
vi.mock("../../hooks/useAccount", () => ({
  default: () => ({ user: { id: "usr_1" }, admin: false, serverConfig: {} }),
}));
vi.mock("../../hooks/useRouter", () => ({
  default: () => ({ navigate, params: { org_id: "org_1" } }),
}));
vi.mock("../../services/interactionsService", () => ({
  useUpdateInteractionFeedback: () => ({ updateFeedback: vi.fn() }),
}));

const baseProps = {
  serverConfig: { filestore_prefix: "/api/v1/filestore" } as any,
  session: { id: "ses_1" } as any,
  interaction: { id: "int_1", prompt_message: "Do the work" } as any,
  isFromAssistant: true,
  onRegenerate: vi.fn(),
};

describe("InteractionInference error display", () => {
  it("offers Retry while the failure is the latest thing that happened", () => {
    render(
      <InteractionInference
        {...baseProps}
        error="agent turn aborted: insufficient balance"
      />,
    );

    expect(screen.getByText("Turn failed")).toBeInTheDocument();
    expect(
      screen.getByText("agent turn aborted: insufficient balance"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Retry/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add credits" }));
    expect(navigate).toHaveBeenCalledWith("org_billing", { org_id: "org_1" });
  });

  it("does not offer credits for unrelated failures", () => {
    render(<InteractionInference {...baseProps} error="agent turn timed out" />);

    expect(
      screen.queryByRole("button", { name: "Add credits" }),
    ).not.toBeInTheDocument();
  });

  it("withholds Retry once the session has recovered", () => {
    // Retry re-sends this interaction's prompt. After the session has moved on
    // and completed later work, that prompt is stale — offering the button
    // invites the user to re-run something the session already left behind,
    // and the red alert makes a working session look broken.
    render(
      <InteractionInference
        {...baseProps}
        error="agent turn aborted"
        errorIsHistorical
      />,
    );

    expect(
      screen.queryByRole("button", { name: /Retry/i }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Turn failed")).not.toBeInTheDocument();
    // Not erased, though: the turn did fail and its work was abandoned.
    expect(
      screen.getByText(/This turn was interrupted and did not finish/),
    ).toBeInTheDocument();
    expect(screen.getByText("agent turn aborted")).toBeInTheDocument();
  });
});
