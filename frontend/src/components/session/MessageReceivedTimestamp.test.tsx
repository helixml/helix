import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import MessageReceivedTimestamp, {
  formatMessageReceivedAt,
} from "./MessageReceivedTimestamp";

describe("MessageReceivedTimestamp", () => {
  it("shows the completion time and reveals the full local date and time", async () => {
    const completedAt = "2026-08-05T14:07:09Z";
    const formatted = formatMessageReceivedAt(completedAt);

    expect(formatted).not.toBeNull();
    render(<MessageReceivedTimestamp completedAt={completedAt} />);

    const timestamp = screen.getByText(formatted!.short);
    expect(timestamp).toHaveAttribute("datetime", completedAt);

    fireEvent.mouseOver(timestamp);
    expect(await screen.findByRole("tooltip")).toHaveTextContent(formatted!.full);
  });

  it.each([undefined, "", "not-a-date", "0001-01-01T00:00:00Z"])(
    "does not render an unusable completion timestamp (%s)",
    (completedAt) => {
      const { container } = render(
        <MessageReceivedTimestamp completedAt={completedAt} />,
      );

      expect(container).toBeEmptyDOMElement();
    },
  );
});
