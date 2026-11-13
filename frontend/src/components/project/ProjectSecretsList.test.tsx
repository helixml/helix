import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import { TypesSecret, TypesSecretScope } from "../../api/api";
import ProjectSecretsList from "./ProjectSecretsList";

const mocks = vi.hoisted(() => ({
  update: vi.fn(),
}));

vi.mock("../../hooks/useApi", () => ({
  default: () => ({
    getApiClient: () => ({ v1SecretsUpdate: mocks.update }),
  }),
}));

vi.mock("../../hooks/useSnackbar", () => ({
  default: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const secrets: TypesSecret[] = [
  { id: "sec_ok", name: "API_URL", scope: TypesSecretScope.SecretScopeBoth },
  {
    id: "sec_stale",
    name: "PENTEST_TOKEN",
    scope: TypesSecretScope.SecretScopeDev,
    undecryptable: true,
  },
];

function renderList(onDelete = vi.fn()) {
  const queryClient = new QueryClient();
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  render(
    <QueryClientProvider client={queryClient}>
      <ProjectSecretsList
        projectId="prj_1"
        secrets={secrets}
        onDelete={onDelete}
        deleting={false}
      />
    </QueryClientProvider>,
  );
  return { invalidate, onDelete };
}

describe("ProjectSecretsList", () => {
  it("badges only the undecryptable secret and offers to re-enter it", () => {
    renderList();

    expect(screen.getAllByText("encrypted")).toHaveLength(1);
    expect(screen.getAllByText("Can't decrypt — re-enter")).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "Re-enter" })).toHaveLength(1);
  });

  it("re-enters the value in place and refreshes the list", async () => {
    mocks.update.mockResolvedValue({ data: {} });
    const { invalidate } = renderList();

    fireEvent.click(screen.getByRole("button", { name: "Re-enter" }));
    expect(screen.getByText("Re-enter PENTEST_TOKEN")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Secret Value"), {
      target: { value: "hi" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith("sec_stale", {
        name: "PENTEST_TOKEN",
        value: [104, 105],
      }),
    );
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith({
        queryKey: ["project-secrets", "prj_1"],
      }),
    );
  });

  it("deletes by secret id", () => {
    const { onDelete } = renderList();

    fireEvent.click(screen.getAllByRole("button", { name: "Delete" })[1]);
    expect(onDelete).toHaveBeenCalledWith("sec_stale");
  });
});
