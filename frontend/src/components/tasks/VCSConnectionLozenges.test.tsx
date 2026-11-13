import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import {
  TypesExternalRepositoryType,
  TypesVCSConnectionInfo,
  TypesVCSConnectionState,
} from "../../api/api";
import VCSConnectionLozenges from "./VCSConnectionLozenges";

const getProjectVcsConnections = vi.hoisted(() => vi.fn());

vi.mock("../../hooks/useApi", () => ({
  default: () => ({
    getApiClient: () => ({ getProjectVcsConnections }),
  }),
}));

vi.mock("../../contexts/settingsDialog", () => ({
  useSettingsDialog: () => ({ openDialog: vi.fn() }),
}));

const renderConnection = async (connection: TypesVCSConnectionInfo) => {
  getProjectVcsConnections.mockResolvedValueOnce({ data: [connection] });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <VCSConnectionLozenges projectId="project" />
    </QueryClientProvider>,
  );
  return screen.findByText(/GitHub/);
};

describe("VCSConnectionLozenges", () => {
  it("describes an unprobed repository token as configured", async () => {
    const chip = await renderConnection({
      provider: TypesExternalRepositoryType.ExternalRepositoryTypeGitHub,
      pushing_as: { username: "@repo-owner" },
      state: TypesVCSConnectionState.VCSConnectionVerified,
      repos: [
        { repo: "acme/checked", has_access: true, verified: true },
        { repo: "acme/unprobed", has_access: true, verified: false },
      ],
    });

    expect(chip.closest(".MuiChip-root")).toHaveClass("MuiChip-outlined");
    expect(chip.closest(".MuiChip-root")).not.toHaveClass(
      "MuiChip-colorSuccess",
    );
    fireEvent.mouseOver(chip);
    expect(await screen.findByText(/Repository sync uses @repo-owner/)).toHaveTextContent(
      "Access has not been verified",
    );
    expect(screen.queryByText(/Pushing as/)).not.toBeInTheDocument();
  });

  it("uses verified styling only after every repository probe succeeds", async () => {
    const chip = await renderConnection({
      provider: TypesExternalRepositoryType.ExternalRepositoryTypeGitHub,
      state: TypesVCSConnectionState.VCSConnectionVerified,
      repos: [{ repo: "acme/private", has_access: true, verified: true }],
    });

    expect(chip.closest(".MuiChip-root")).toHaveClass("MuiChip-colorSuccess");
    fireEvent.mouseOver(chip);
    expect(
      await screen.findByText("Repository access verified for acme/private."),
    ).toBeInTheDocument();
  });

  it("keeps a verified access failure in the warning state", async () => {
    const chip = await renderConnection({
      provider: TypesExternalRepositoryType.ExternalRepositoryTypeGitHub,
      pushing_as: { username: "@repo-owner" },
      state: TypesVCSConnectionState.VCSConnectionNeedsAttention,
      repos: [{ repo: "acme/private", has_access: false, verified: true }],
    });

    expect(chip.closest(".MuiChip-root")).toHaveClass("MuiChip-colorWarning");
    fireEvent.mouseOver(chip);
    expect(
      await screen.findByText(/Repository connection @repo-owner can't access/),
    ).toHaveTextContent(
      "acme/private",
    );
  });
});
