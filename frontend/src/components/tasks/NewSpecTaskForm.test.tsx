import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import NewSpecTaskForm from "./NewSpecTaskForm";

vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: [] }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));
vi.mock("react-dropzone", () => ({
  useDropzone: () => ({ getRootProps: () => ({}), isDragActive: false }),
}));
vi.mock("../../hooks/useAccount", () => ({
  default: () => ({
    user: { id: "user-1", email: "user@example.com" },
    organizationTools: { organization: { memberships: [] } },
    setShowLoginWindow: vi.fn(),
  }),
}));
vi.mock("../../hooks/useApi", () => ({
  default: () => ({ getApiClient: () => ({}) }),
}));
vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ error: vi.fn(), success: vi.fn() }),
}));
vi.mock("../../hooks/useApps", () => ({
  default: () => ({ apps: [{ id: "app-1", config: { helix: {} } }], loadApps: vi.fn() }),
}));
vi.mock("../../utils/apps", () => ({ isCodingAgent: () => true }));
vi.mock("../../services", () => ({
  useGetProject: () => ({ data: { code_agent_config: undefined } }),
  useGetProjectRepositories: () => ({ data: [] }),
}));
vi.mock("../../services/specTaskService", () => ({
  useSpecTasks: () => ({ data: [] }),
  useProjectLabels: () => ({ data: [] }),
  useAddLabel: () => ({ mutateAsync: vi.fn() }),
}));
vi.mock("../../services/specTaskAttachmentsService", () => ({
  SPEC_TASK_ATTACHMENT_ACCEPTED_MIME: { "text/plain": [".txt"] },
  SPEC_TASK_ATTACHMENT_MAX_BYTES: 1_000_000,
  SPEC_TASK_ATTACHMENT_MAX_PER_TASK: 10,
  useUploadSpecTaskAttachments: () => ({ mutateAsync: vi.fn() }),
}));
vi.mock("./AssigneeSelector", () => ({ default: () => null }));
vi.mock("../widgets/OrganizationUserAvatar", () => ({
  default: () => null,
  resolveOrganizationUser: () => undefined,
}));
vi.mock("./GooseRecipeSelector", () => ({ default: () => null }));
vi.mock("../agent/CodingAgentForm", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../agent/CodingAgentForm")>()),
  default: () => null,
}));
vi.mock("./SpecTaskExecutionControls", () => ({ default: () => null }));

describe("NewSpecTaskForm", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("keeps plain Enter available for new lines in the description", () => {
    const onClose = vi.fn();
    render(
      <NewSpecTaskForm
        projectId="project-1"
        onTaskCreated={vi.fn()}
        onClose={onClose}
      />,
    );

    const description = screen.getByRole("textbox", {
      name: /Describe what you want to get done/i,
    });
    expect(fireEvent.keyDown(description, { key: "Enter" })).toBe(true);
    expect(onClose).not.toHaveBeenCalled();

    fireEvent.change(description, { target: { value: "First line\nSecond line" } });
    expect(description).toHaveValue("First line\nSecond line");
  });
});
