import { describe, expect, it } from "vitest";

import { deriveSandboxState, isSandboxOffline } from "./sandboxState";

describe("deriveSandboxState", () => {
  it("reports loading until the session has been fetched", () => {
    expect(deriveSandboxState(undefined)).toEqual({
      sandboxState: "loading",
      statusMessage: "",
      hasDesktopLifecycleState: false,
    });
  });

  it("treats an explicit stopped status as absent even with a container name", () => {
    // The backend clears container metadata when it downgrades a session, but a
    // read that races the write can still carry both fields.
    const state = deriveSandboxState({
      external_agent_status: "stopped",
      container_name: "headless-external-ses_1",
    });

    expect(state.sandboxState).toBe("absent");
    expect(state.hasDesktopLifecycleState).toBe(true);
  });

  it("treats an idle-terminated sandbox as absent", () => {
    // The idle checker writes this status and leaves container_name in place.
    // Reading it as running showed stop/restart actions for a sandbox that was
    // gone, and left the changes view waiting on requests that only ever 503.
    const state = deriveSandboxState({
      external_agent_status: "terminated_idle",
      container_name: "headless-external-ses_1",
    });

    expect(state.sandboxState).toBe("absent");
    expect(state.hasDesktopLifecycleState).toBe(true);
    expect(
      isSandboxOffline({
        external_agent_status: "terminated_idle",
        container_name: "headless-external-ses_1",
      }),
    ).toBe(true);
  });

  it("reports running for a running container", () => {
    expect(
      deriveSandboxState({
        external_agent_status: "running",
        container_name: "headless-external-ses_1",
      }).sandboxState,
    ).toBe("running");
  });

  it("reports starting during boot", () => {
    expect(
      deriveSandboxState({ external_agent_status: "starting" }).sandboxState,
    ).toBe("starting");
  });

  it("surfaces the transient status message", () => {
    expect(
      deriveSandboxState({
        external_agent_status: "starting",
        status_message: "Unpacking build cache (2.1/7.0 GB)",
      }).statusMessage,
    ).toBe("Unpacking build cache (2.1/7.0 GB)");
  });

  it("reports absent with no lifecycle state for a plain chat session", () => {
    const state = deriveSandboxState({ agent_type: "text" });

    expect(state.sandboxState).toBe("absent");
    expect(state.hasDesktopLifecycleState).toBe(false);
  });
});

describe("isSandboxOffline", () => {
  it("is true for a session whose container has stopped", () => {
    expect(
      isSandboxOffline({
        external_agent_status: "stopped",
        container_name: "headless-external-ses_1",
      }),
    ).toBe(true);
  });

  it("is false while the sandbox is running", () => {
    expect(
      isSandboxOffline({
        external_agent_status: "running",
        container_name: "headless-external-ses_1",
      }),
    ).toBe(false);
  });

  it("is false while the sandbox is still booting", () => {
    expect(isSandboxOffline({ external_agent_status: "starting" })).toBe(false);
  });

  // Guards the regression this helper could easily introduce: an ordinary LLM
  // chat has no container fields at all and derives to "absent", but its agent
  // is not offline. Treating it as offline would suppress the streaming
  // indicator for every normal chat in the product.
  it("is false for a plain chat session with no sandbox", () => {
    expect(isSandboxOffline({ agent_type: "text" })).toBe(false);
    expect(isSandboxOffline({})).toBe(false);
  });

  it("is false before the session has loaded", () => {
    expect(isSandboxOffline(undefined)).toBe(false);
  });
});

// "restarting" is the backend marker written before StopDesktop so the teardown
// half of a restart still reads as a boot in flight. The UI deliberately has no
// fourth SandboxState for it — mapping it onto "starting" is what gives every
// consumer (viewer overlay, toolbar, Kanban card, isSandboxOffline) the right
// behaviour without a per-call-site change.
describe('deriveSandboxState("restarting")', () => {
  it('maps onto "starting" so the spinner shows and no Start button appears', () => {
    expect(
      deriveSandboxState({ external_agent_status: "restarting" }).sandboxState,
    ).toBe("starting");
  });

  it('still maps to "starting" while the old container name is on the row', () => {
    // This is the exact shape mid-restart: StopDesktop leaves container_name
    // set, and the old code read that as a running/stopped container.
    expect(
      deriveSandboxState({
        external_agent_status: "restarting",
        container_name: "ubuntu-external-ses_1",
      }).sandboxState,
    ).toBe("starting");
  });

  it("carries the backend's progress text through", () => {
    expect(
      deriveSandboxState({
        external_agent_status: "restarting",
        status_message: "Restarting desktop...",
      }).statusMessage,
    ).toBe("Restarting desktop...");
  });

  it("is not offline", () => {
    expect(isSandboxOffline({ external_agent_status: "restarting" })).toBe(false);
  });
});
