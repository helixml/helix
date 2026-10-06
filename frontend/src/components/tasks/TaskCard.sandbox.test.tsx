import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TypesSandboxRuntime } from "../../api/api";
import TaskCard, { SpecTaskWithExtras } from "./TaskCard";

vi.mock("../../hooks/useAccount", () => ({
  default: () => ({
    user: { id: "usr_1", fullName: "Test User", email: "test@example.com" },
    organizationTools: { organization: { memberships: [] } },
  }),
}));

vi.mock("../external-agent/ExternalAgentDesktopViewer", () => ({
  default: () => <div data-testid="desktop-viewer" />,
}));

vi.mock("../../services/specTaskWorkflowService", () => ({
  useApproveImplementation: () => ({ mutate: vi.fn(), isPending: false }),
  useStopAgent: () => ({ mutate: vi.fn(), isPending: false }),
  useMoveToBacklog: () => ({ mutate: vi.fn(), isPending: false }),
}));

vi.mock("../../services/specTaskService", () => ({
  useUpdateSpecTask: () => ({ mutate: vi.fn() }),
  useDeleteSpecTask: () => ({ mutate: vi.fn() }),
}));

vi.mock("../../hooks/useAttentionEvents", () => ({
  useAttentionEvents: () => ({ acknowledge: vi.fn() }),
}));

vi.mock("../specTask/CloneTaskDialog", () => ({ default: () => null }));
vi.mock("../specTask/CloneGroupProgress", () => ({ default: () => null }));
vi.mock("./UsagePulseChart", () => ({ default: () => null }));
vi.mock("./CIStatusIcon", () => ({ default: () => null }));
vi.mock("./SpecTaskActionButtons", () => ({ default: () => null }));
vi.mock("./AssigneeSelector", () => ({ default: () => null }));

function baseTask(overrides: Partial<SpecTaskWithExtras> = {}): SpecTaskWithExtras {
  return {
    id: "spt_1",
    name: "web-pentest: recon",
    status: "implementation",
    phase: "implementation",
    planning_session_id: "ses_1",
    sandbox_state: "running",
    ...overrides,
  };
}

function renderCard(task: SpecTaskWithExtras) {
  return render(
    <TaskCard
      task={task}
      index={0}
      columns={[]}
      projectId="prj_1"
    />,
  );
}

describe("TaskCard sandbox screenshot", () => {
  it("mounts the desktop viewer for a live desktop-runtime task", () => {
    renderCard(baseTask());
    expect(screen.getByTestId("desktop-viewer")).toBeInTheDocument();
  });

  it("does not mount the desktop viewer for a headless-runtime task", () => {
    renderCard(
      baseTask({ sandbox_runtime: TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu }),
    );
    expect(screen.queryByTestId("desktop-viewer")).not.toBeInTheDocument();
  });
});
