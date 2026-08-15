import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import useLiveInteraction from "../../hooks/useLiveInteraction";
import { TypesInteractionState } from "../../api/api";
import { InteractionLiveStream } from "./InteractionLiveStream";

vi.mock("../../hooks/useLiveInteraction", () => ({
  default: vi.fn(),
}));

const mockedUseLiveInteraction = vi.mocked(useLiveInteraction);

describe("InteractionLiveStream", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-03T00:02:05.000Z"));
    mockedUseLiveInteraction.mockReturnValue({
      message: "",
      responseEntries: undefined,
      durationMs: 0,
      status: TypesInteractionState.InteractionStateWaiting,
      isComplete: false,
      stepInfos: [],
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  const renderStream = (agentOffline?: boolean) =>
    render(
      <InteractionLiveStream
        session_id="session-1"
        interaction={{
          id: "interaction-1",
          created: "2026-08-03T00:00:00.000Z",
          state: TypesInteractionState.InteractionStateWaiting,
        }}
        session={{ id: "session-1" }}
        serverConfig={{ filestore_prefix: "/api/v1/filestore" }}
        agentOffline={agentOffline}
      />,
    );

  it("keeps counting from the persisted request time after mounting", () => {
    renderStream();

    expect(screen.getByText("Working for 2m 5s")).toBeInTheDocument();
  });

  // An interaction stays `state=waiting` long after the container backing it has
  // exited — until the agent answers or the auto-wake worker errors it out. The
  // row alone cannot express that, so without the sandbox state the chat renders
  // a ticking timer against a dead sandbox.
  it("does not show a running timer when the sandbox has stopped", () => {
    renderStream(true);

    expect(screen.queryByText(/Working for/)).not.toBeInTheDocument();
  });

  it("explains that the sandbox stopped instead of the timer", () => {
    renderStream(true);

    expect(screen.getByRole("status", { name: "Sandbox stopped" })).toBeInTheDocument();
    expect(screen.getByText(/the agent is not running/i)).toBeInTheDocument();
  });

  it("still shows the timer when the sandbox is alive", () => {
    renderStream(false);

    expect(screen.getByText("Working for 2m 5s")).toBeInTheDocument();
    expect(screen.queryByRole("status", { name: "Sandbox stopped" })).not.toBeInTheDocument();
  });
});
