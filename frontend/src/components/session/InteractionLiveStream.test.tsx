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

  it("keeps counting from the persisted request time after mounting", () => {
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
      />,
    );

    expect(screen.getByText("Working for 2m 5s")).toBeInTheDocument();
  });
});
