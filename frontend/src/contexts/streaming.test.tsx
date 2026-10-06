import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React, { useEffect } from "react";

// Capture the socket the provider opens so the test can push frames at it.
// Hoisted, because vi.mock's factory runs before any module-level code here.
const { sockets, MockSocket } = vi.hoisted(() => {
  const sockets: any[] = [];
  class MockSocket {
    listeners = new Map<string, Set<(e: any) => void>>();
    closed = false;
    constructor(public url: string) {
      sockets.push(this);
    }
    addEventListener(type: string, fn: (e: any) => void) {
      if (!this.listeners.has(type)) this.listeners.set(type, new Set());
      this.listeners.get(type)!.add(fn);
    }
    removeEventListener(type: string, fn: (e: any) => void) {
      this.listeners.get(type)?.delete(fn);
    }
    close() {
      this.closed = true;
    }
    reconnect() {}
    /** Deliver a websocket frame the way the browser would. */
    emit(type: string, payload: unknown) {
      for (const fn of this.listeners.get(type) ?? []) {
        fn(type === "message" ? { data: JSON.stringify(payload) } : {});
      }
    }
  }
  return { sockets, MockSocket };
});

vi.mock("reconnecting-websocket", () => ({ default: MockSocket }));

import { StreamingContextProvider, useStreaming } from "./streaming";
import { AccountContext } from "./account";

const SESSION_ID = "ses_test";
const INTERACTION_ID = "int_1";
const VIEWER_ID = "user_viewer";
const OWNER_ID = "user_owner";

/** Mounts the provider, attaches it to a session, and reports the socket. */
function mountProvider(userId: string, queryClient: QueryClient) {
  const Attach: React.FC = () => {
    const { setCurrentSessionId } = useStreaming();
    useEffect(() => {
      setCurrentSessionId(SESSION_ID);
    }, [setCurrentSessionId]);
    return null;
  };

  render(
    <QueryClientProvider client={queryClient}>
      <AccountContext.Provider value={{ user: { id: userId } } as any}>
        <StreamingContextProvider>
          <Attach />
        </StreamingContextProvider>
      </AccountContext.Provider>
    </QueryClientProvider>,
  );

  return sockets[sockets.length - 1];
}

/** One interaction_patch frame, in the shape the Go server emits. */
function patchFrame(
  overrides: Partial<{
    owner: string;
    entry_count: number;
    entry_patches: any[];
  }> = {},
) {
  return {
    type: "interaction_patch",
    session_id: SESSION_ID,
    interaction_id: INTERACTION_ID,
    owner: OWNER_ID,
    entry_count: 1,
    entry_patches: [
      {
        index: 0,
        type: "text",
        content: "",
        patch: "hello",
        patch_offset: 0,
        total_length: 5,
        message_id: "msg-1",
      },
    ],
    ...overrides,
  };
}

describe("streaming context: interaction_patch", () => {
  let queryClient: QueryClient;
  let invalidate: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    sockets.length = 0;
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    invalidate = vi.spyOn(queryClient, "invalidateQueries");
  });

  afterEach(() => {
    vi.restoreAllMocks();
    queryClient.clear();
  });

  /** Invalidations of the paginated interactions list, which is the expensive one. */
  const interactionRefetches = () =>
    invalidate.mock.calls.filter(
      (c: any[]) => c[0]?.queryKey?.[0] === "interactions",
    ).length;

  it("pulls the interaction into a viewer's list, once", () => {
    // The reported bug: a viewer of someone else's session never inserted this
    // interaction, so nothing was in the `waiting` state to render as live —
    // no spinner, no streaming, the whole reply landing at once on completion.
    const socket = mountProvider(VIEWER_ID, queryClient);

    act(() => socket.emit("message", patchFrame()));
    expect(interactionRefetches()).toBe(1);

    // ...and not again on every subsequent patch of the same interaction.
    act(() => {
      for (let i = 0; i < 20; i++) socket.emit("message", patchFrame());
    });
    expect(interactionRefetches()).toBe(1);
  });

  it("does not refetch for the person who sent the message", () => {
    // The sender's client gets the interaction through the debounced
    // invalidation every non-patch event already triggers. Doing it here too
    // is one wasted LIST_INTERACTIONS fetch per turn for every user.
    const socket = mountProvider(OWNER_ID, queryClient);

    act(() => socket.emit("message", patchFrame()));
    expect(interactionRefetches()).toBe(0);
  });

  it("resyncs once on a lost baseline, not on every patch after it", () => {
    // A lost baseline cannot be repaired by more deltas. Without suppression
    // the resync deletes the entries, the next patch re-grows an empty array,
    // the invariant fires again — ~20 refetches a second at a 50ms publish
    // interval, with the render frozen throughout regardless.
    const socket = mountProvider(OWNER_ID, queryClient);

    // Two entries exist but only the second is ever patched: entry 0 is a hole
    // we cannot fill locally.
    const lostBaseline = patchFrame({
      entry_count: 2,
      entry_patches: [
        {
          index: 1,
          type: "text",
          content: "",
          patch: "the last segment",
          patch_offset: 0,
          total_length: 16,
          message_id: "msg-2",
        },
      ],
    });

    act(() => socket.emit("message", lostBaseline));
    const afterFirst = interactionRefetches();
    expect(afterFirst).toBe(1);

    act(() => {
      for (let i = 0; i < 20; i++) socket.emit("message", lostBaseline);
    });
    expect(interactionRefetches()).toBe(afterFirst);
  });

  it("resumes judging patches once the interaction completes", () => {
    const socket = mountProvider(OWNER_ID, queryClient);

    const lostBaseline = patchFrame({
      entry_count: 2,
      entry_patches: [
        {
          index: 1,
          type: "text",
          content: "",
          patch: "tail",
          patch_offset: 0,
          total_length: 4,
          message_id: "msg-2",
        },
      ],
    });

    act(() => socket.emit("message", lostBaseline));
    expect(interactionRefetches()).toBe(1);

    // The authoritative content arrives, so the suppression is no longer right.
    act(() =>
      socket.emit("message", {
        type: "interaction_update",
        session_id: SESSION_ID,
        owner: OWNER_ID,
        interaction: { id: INTERACTION_ID, state: "complete" },
      }),
    );

    act(() => socket.emit("message", lostBaseline));
    expect(interactionRefetches()).toBeGreaterThan(1);
  });
});

describe("streaming context: helix-agent-tool notifications", () => {
  let queryClient: QueryClient;
  let postMessage: ReturnType<typeof vi.fn>;
  let realParent: PropertyDescriptor | undefined;

  beforeEach(() => {
    sockets.length = 0;
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    // jsdom is always top-level (window.parent === window), so stand in a
    // distinct parent to put the provider in the embedded case.
    realParent = Object.getOwnPropertyDescriptor(window, "parent");
    postMessage = vi.fn();
    Object.defineProperty(window, "parent", {
      value: { postMessage },
      configurable: true,
      writable: true,
    });
  });

  afterEach(() => {
    if (realParent) Object.defineProperty(window, "parent", realParent);
    vi.restoreAllMocks();
    queryClient.clear();
  });

  const toolFrame = (status: string, message_id = "msg-tool") =>
    patchFrame({
      entry_patches: [
        {
          index: 0,
          type: "tool_call",
          content: "",
          patch: "findai_search_jobs",
          patch_offset: 0,
          total_length: 18,
          message_id,
          tool_name: "findai_search_jobs",
          tool_status: status,
        },
      ],
    });

  const toolPosts = () =>
    postMessage.mock.calls.filter(
      (c: any[]) => c[0]?.type === "helix-agent-tool",
    );

  it("stays silent while a tool is still running", () => {
    const socket = mountProvider(OWNER_ID, queryClient);
    act(() => socket.emit("message", toolFrame("In Progress")));
    expect(toolPosts()).toHaveLength(0);
  });

  it("announces a completed tool exactly once, not once per patch", () => {
    // The server repeats a tool_call entry's metadata on every patch that
    // touches it, so posting unconditionally makes the host re-render ~20
    // times a second and double- or triple-render whatever it draws.
    const socket = mountProvider(OWNER_ID, queryClient);

    act(() => {
      for (let i = 0; i < 10; i++) socket.emit("message", toolFrame("Completed"));
    });

    const posts = toolPosts();
    expect(posts).toHaveLength(1);
    expect(posts[0][0]).toEqual({
      type: "helix-agent-tool",
      tool: "findai_search_jobs",
      status: "Completed",
      session_id: SESSION_ID,
    });
  });

  it("never forwards the tool's content to the host", () => {
    const socket = mountProvider(OWNER_ID, queryClient);
    act(() => socket.emit("message", toolFrame("Completed")));

    const payload = toolPosts()[0][0];
    expect(Object.keys(payload).sort()).toEqual([
      "session_id",
      "status",
      "tool",
      "type",
    ]);
  });

  it("announces a second, different status for the same tool", () => {
    const socket = mountProvider(OWNER_ID, queryClient);
    act(() => {
      socket.emit("message", toolFrame("Completed"));
      socket.emit("message", toolFrame("Failed"));
    });
    expect(toolPosts()).toHaveLength(2);
  });
});
