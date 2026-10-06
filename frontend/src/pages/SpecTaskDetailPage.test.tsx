import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import SpecTaskDetailPage from "./SpecTaskDetailPage";

vi.mock("react-router5", () => ({
  useRoute: () => ({
    route: {
      name: "org_project-task-detail",
      params: { id: "project-1", taskId: "task-1" },
    },
  }),
}));
vi.mock("../services/specTaskService", () => ({
  useSpecTask: () => ({ data: { id: "task-1", name: "Test task" }, isLoading: false }),
}));
vi.mock("../services", () => ({
  useGetProject: () => ({ data: { id: "project-1", name: "Test project" }, isLoading: false }),
}));
vi.mock("../hooks/useAccount", () => ({
  default: () => ({ orgNavigate: vi.fn() }),
}));
vi.mock("../lib/navHistory", () => ({ cacheTaskName: vi.fn() }));
vi.mock("../components/system/Page", () => ({
  default: ({ children, topbarContent }: { children: React.ReactNode; topbarContent?: React.ReactNode }) => (
    <div>{topbarContent}{children}</div>
  ),
}));
vi.mock("../components/tasks/SpecTaskDetailContent", () => ({
  default: () => <div data-testid="task-content" />,
}));
vi.mock("../components/tasks/NewSpecTaskForm", () => ({
  default: () => <div data-testid="new-spec-task-form" />,
}));

describe("SpecTaskDetailPage task creation", () => {
  it("does not open a new task when Enter comes from a shadow-DOM editor", () => {
    render(<SpecTaskDetailPage />);

    const editorHost = document.createElement("diffs-container");
    const shadowRoot = editorHost.attachShadow({ mode: "open" });
    const editor = document.createElement("div");
    editor.contentEditable = "true";
    shadowRoot.appendChild(editor);
    screen.getByTestId("task-content").appendChild(editorHost);
    let outerTarget: EventTarget | null = null;
    const observeTarget = (event: KeyboardEvent) => {
      outerTarget = event.target;
    };
    window.addEventListener("keydown", observeTarget);

    editor.dispatchEvent(new KeyboardEvent("keydown", {
      key: "Enter",
      bubbles: true,
      composed: true,
    }));
    window.removeEventListener("keydown", observeTarget);

    expect(outerTarget).toBe(editorHost);
    expect(screen.queryByTestId("new-spec-task-form")).not.toBeInTheDocument();
  });

  it("opens a new task from the explicit toolbar action", () => {
    render(<SpecTaskDetailPage />);

    fireEvent.click(screen.getByRole("button", { name: "Create New Task" }));

    expect(screen.getByTestId("new-spec-task-form")).toBeInTheDocument();
  });
});
