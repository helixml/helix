import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import InteractionDebugCopyButton from "./InteractionDebugCopyButton";

vi.mock("../../contexts/streaming", () => ({
  useStreaming: () => ({ currentResponses: new Map() }),
}));

describe("InteractionDebugCopyButton", () => {
  const writeText = vi.fn().mockResolvedValue(undefined);

  beforeEach(() => {
    writeText.mockClear();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
  });

  it("copies a parseable debug bundle for the selected interaction", async () => {
    render(
      <InteractionDebugCopyButton
        interaction={{ id: "int_1", session_id: "ses_1", error: "boom" }}
        session={{ id: "ses_1", model_name: "test-model" }}
        sessionSteps={[
          { interaction_id: "int_1", name: "included" },
          { interaction_id: "int_2", name: "excluded" },
        ]}
        serverConfig={{ version: "test-version" }}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Copy interaction debug context" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledOnce());
    const copied = JSON.parse(writeText.mock.calls[0][0]);
    expect(copied.interaction.id).toBe("int_1");
    expect(copied.session.model_name).toBe("test-model");
    expect(copied.session_steps).toEqual([
      { interaction_id: "int_1", name: "included" },
    ]);
  });
});
