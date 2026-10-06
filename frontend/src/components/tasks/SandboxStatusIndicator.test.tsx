import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import SandboxStatusIndicator from "./SandboxStatusIndicator";

describe("SandboxStatusIndicator", () => {
  it("identifies a running sandbox", () => {
    render(<SandboxStatusIndicator state="running" />);

    const indicator = screen.getByRole("status", { name: "Sandbox running" });
    expect(indicator).toHaveAttribute(
      "data-state",
      "running",
    );
    expect(indicator.firstChild).toHaveStyle({ backgroundColor: "#34d399" });
  });

  it.each([
    ["starting", "Sandbox starting"],
    ["stopped", "Sandbox stopped"],
  ] as const)("keeps the %s state distinct from running", (state, label) => {
    render(<SandboxStatusIndicator state={state} />);

    const indicator = screen.getByRole("status", { name: label });
    expect(indicator).toHaveAttribute(
      "data-state",
      state,
    );
    expect(indicator.firstChild).toHaveStyle({ backgroundColor: "#a1a1aa" });
  });
});
