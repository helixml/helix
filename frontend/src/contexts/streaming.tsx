import React, {
  createContext,
  useContext,
  ReactNode,
  useState,
  useCallback,
  useEffect,
  useRef,
} from "react";
import ReconnectingWebSocket from "reconnecting-websocket";
import {
  IEntryPatch,
  IWebsocketEvent,
  WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE,
  WEBSOCKET_EVENT_TYPE_INTERACTION_PATCH,
  WORKER_TASK_RESPONSE_TYPE_PROGRESS,
  ISessionChatRequest,
  ISessionType,
  IAgentType,
} from "../types";
import {
  applyPatch,
  hasLostBaseline,
  isTerminalToolStatus,
} from "../utils/patchUtils";
import useAccount from "../hooks/useAccount";

/**
 * An embedded agent can signal its host; a top-level one has nobody to tell.
 *
 * Evaluated per call rather than once at module load: the answer is fixed for
 * a real document, but pinning it at import time makes it unobservable to a
 * test, and this gate decides whether the host is told anything at all.
 */
const isEmbedded = () =>
  typeof window !== "undefined" && window.parent !== window;
import { TypesInteraction, TypesInteractionState, TypesMessage, TypesSession } from "../api/api";
import { ResponseEntry } from "../components/session/InteractionInference";
import {
  GET_SESSION_QUERY_KEY,
  SESSION_STEPS_QUERY_KEY,
  LIST_INTERACTIONS_QUERY_KEY,
} from "../services/sessionService";
import { useQueryClient } from "@tanstack/react-query";
import { invalidateSessionsQuery } from "../services/sessionService";

// CSRF helper - reads the CSRF token from the helix_csrf cookie
const getCSRFToken = (): string | null => {
  const match = document.cookie.match(/(^| )helix_csrf=([^;]+)/);
  return match ? decodeURIComponent(match[2]) : null;
};

// The streaming context holds either raw API interactions (response_entries: number[])
// or locally-assembled streaming state (response_entries: ResponseEntry[]).
type StreamingInteraction = Omit<Partial<TypesInteraction>, 'response_entries'> & {
  response_entries?: ResponseEntry[] | number[];
};

interface NewInferenceParams {
  regenerate?: boolean;
  type: ISessionType;
  message: string;
  messages?: TypesMessage[];
  image?: string;
  image_filename?: string;
  appId?: string;
  projectId?: string;
  assistantId?: string;
  interactionId?: string;
  provider?: string;
  modelName?: string;
  reasoningEffort?: string;
  sessionId?: string;
  orgId?: string;
  attachedImages?: File[];
  agentType?: IAgentType;
  externalAgentConfig?: any;
  interrupt?: boolean; // If true, interrupt current agent work; if false/undefined, queue after current work
  sessionRole?: string;
}

interface StreamingContextType {
  NewInference: (params: NewInferenceParams) => Promise<TypesSession>;
  setCurrentSessionId: (sessionId: string) => void;
  currentResponses: Map<string, StreamingInteraction>;
  stepInfos: Map<string, any[]>;
  wsConnected: boolean;
  updateCurrentResponse: (
    sessionId: string,
    interaction: Partial<TypesInteraction>,
  ) => void;
}

const StreamingContext = createContext<StreamingContextType | undefined>(
  undefined,
);

export const useStreaming = (): StreamingContextType => {
  const context = useContext(StreamingContext);
  if (context === undefined) {
    throw new Error("useStreaming must be used within a StreamingProvider");
  }
  return context;
};

export const StreamingContextProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const queryClient = useQueryClient();
  const account = useAccount();
  const [currentResponses, setCurrentResponses] = useState<
    Map<string, StreamingInteraction>
  >(new Map());
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);
  const [stepInfos, setStepInfos] = useState<Map<string, any[]>>(new Map());
  const [wsConnected, setWsConnected] = useState(false);

  // Add refs for managing streaming state
  const messageBufferRef = useRef<Map<string, string[]>>(new Map());
  const pendingUpdateRef = useRef<boolean>(false);
  const messageHistoryRef = useRef<Map<string, string>>(new Map());
  const invalidateTimerRef = useRef<NodeJS.Timeout | null>(null);
  // Track structured response entries per interaction for per-entry patch updates.
  // Keyed by interactionId, stores the current ResponseEntry[] built from entry_patches.
  const patchEntriesRef = useRef<Map<string, ResponseEntry[]>>(new Map());
  const patchPendingRef = useRef<boolean>(false);
  // Interaction ids we have already pulled into the rendered list.
  //
  // Streaming patches deliberately do NOT invalidate the interactions query —
  // they are high-frequency and the owner's own client already inserted the
  // interaction optimistically when it sent the prompt. But a VIEWER of someone
  // else's session never ran that insert, so without this the interaction never
  // reaches their list: no spinner (nothing is in the `waiting` state to render
  // as live), no streaming, and the whole reply appearing at once on completion.
  // Reported by Leah watching Luke's session, 9 Sept 2026.
  //
  // So: refetch ONCE per interaction we have not seen, and keep the
  // high-frequency path allocation-free after that.
  //
  // Gated on NOT being the owner. Whoever submitted the message gets their
  // interaction into the list through the debounced invalidation that every
  // non-patch event already triggers; refetching for them here is one wasted
  // LIST_INTERACTIONS fetch per turn, which is exactly the per-patch cache
  // churn the note at the bottom of this file says never to introduce.
  const seenInteractionsRef = useRef<Set<string>>(new Set());
  // Interactions whose baseline we have given up on until they complete.
  //
  // A lost baseline cannot be repaired by more deltas — the missing bytes are
  // only in the database. Without this, the resync in the patch branch deletes
  // patchEntriesRef, the next patch re-grows an empty array, the invariant
  // fires again, and we invalidate again — at a 50ms publish interval that is
  // ~20 refetches per second for the rest of the interaction, while the render
  // stays frozen regardless. So: resync once, then stay quiet until the
  // completed interaction arrives with the authoritative content.
  const resyncingRef = useRef<Set<string>>(new Set());
  // Tool-call notifications already posted to an embedding host, keyed
  // `interaction:message:status`. The server re-sends a tool_call entry's
  // metadata on every patch that touches it, so without this a host gets one
  // message per patch and re-renders on each.
  const notifiedToolsRef = useRef<Set<string>>(new Set());
  // The signed-in user, read through a ref so the websocket callback keeps a
  // stable identity (it is a dependency of the connection effect — putting the
  // id in its dep array would tear down and reopen the socket when the account
  // loads).
  const currentUserIdRef = useRef<string | undefined>(undefined);
  // Track whether the WebSocket has experienced a disconnect since last connect.
  // Used to detect reconnection events (vs initial connection) so we can refresh
  // stale state missed during the outage.
  const wsWasDisconnectedRef = useRef<boolean>(false);

  // Clear all streaming state when switching sessions
  const clearSessionData = useCallback(
    (sessionId: string | null) => {
      // Don't clear anything if setting to the same session ID
      if (sessionId === currentSessionId) return;

      // Clear ALL streaming state for the old session to prevent stale data leaking
      if (currentSessionId) {
        // Clear stepInfos for the old session
        setStepInfos((prev) => {
          const newMap = new Map(prev);
          newMap.delete(currentSessionId);
          return newMap;
        });

        // Clear currentResponses for the old session
        setCurrentResponses((prev) => {
          const newMap = new Map(prev);
          newMap.delete(currentSessionId);
          return newMap;
        });

        // Clear message buffer and history for the old session
        messageBufferRef.current.delete(currentSessionId);
        messageHistoryRef.current.delete(currentSessionId);
      }

      // Clear all patch state (not session-keyed, so clear everything)
      patchEntriesRef.current.clear();
      patchPendingRef.current = false;
      seenInteractionsRef.current.clear();
      resyncingRef.current.clear();
      notifiedToolsRef.current.clear();

      // Also clear stepInfos for the new session (fresh start)
      if (sessionId) {
        setStepInfos((prev) => {
          const newMap = new Map(prev);
          newMap.delete(sessionId);
          return newMap;
        });
      }

      setCurrentSessionId(sessionId);
    },
    [currentSessionId],
  );

  // Kept current every render so the websocket callback can read it without
  // taking a dependency on it.
  currentUserIdRef.current = account.user?.id;

  // Function to flush message buffer to state
  const flushMessageBuffer = useCallback((sessionId: string) => {
    const chunks = messageBufferRef.current.get(sessionId);
    if (!chunks || chunks.length === 0) return;

    setCurrentResponses((prev) => {
      const current = prev.get(sessionId) || {};
      const existingMessage = messageHistoryRef.current.get(sessionId) || "";
      const newChunks = chunks.join("");
      const newMessage = existingMessage + newChunks;

      // Update our history ref with the new complete message
      messageHistoryRef.current.set(sessionId, newMessage);

      // Clear just the buffer, keeping the history
      messageBufferRef.current.set(sessionId, []);

      return new Map(prev).set(sessionId, {
        ...current,
        response_message: newMessage,
      });
    });
  }, []);

  // Schedule a flush if one isn't already pending
  const scheduleFlush = useCallback(
    (sessionId: string) => {
      if (pendingUpdateRef.current) return;
      pendingUpdateRef.current = true;

      requestAnimationFrame(() => {
        flushMessageBuffer(sessionId);
        pendingUpdateRef.current = false;
      });
    },
    [flushMessageBuffer],
  );

  // Function to add a message chunk to the buffer
  const addMessageChunk = useCallback(
    (sessionId: string, chunk: string) => {
      const chunks = messageBufferRef.current.get(sessionId) || [];
      chunks.push(chunk);
      messageBufferRef.current.set(sessionId, chunks);
      scheduleFlush(sessionId);
    },
    [scheduleFlush],
  );

  // Is this interaction already in a rendered interactions list?
  //
  // The query key carries pagination (`["interactions", id, page, perPage,
  // order]`), so getQueryData with a partial key would miss every cached page.
  // getQueriesData prefix-matches, the same way invalidateQueries does.
  const isInRenderedList = useCallback(
    (interactionId: string): boolean =>
      queryClient
        .getQueriesData<{ data?: { interactions?: { id?: string }[] } }>({
          queryKey: ["interactions", currentSessionId],
        })
        .some(([, cached]) =>
          cached?.data?.interactions?.some((i) => i.id === interactionId),
        ),
    [currentSessionId, queryClient],
  );

  // Tell an embedding page when the agent finishes a tool call.
  //
  // An embedded agent cannot render into its host — it is cross-origin — so a
  // host that wants to show the RESULT of a tool call (job cards, a chart,
  // anything) has no way to know one happened. This is the signal.
  //
  // It carries the tool NAME and STATUS only, never the result. The host
  // fetches its own data from its own API; nothing here is a channel for data
  // the host would then have to trust. `"*"` as the target origin is safe for
  // the same reason — there is nothing sensitive in the message.
  //
  // Once per completed call, not once per patch. The server repeats a
  // tool_call entry's metadata on every patch that touches it, so posting
  // unconditionally means a host re-renders ~20 times a second while a tool
  // runs. Deduped on the entry's message_id so a status that changes twice is
  // announced twice, but the same status never is.
  const notifyHostOfToolCalls = useCallback(
    (interactionId: string, entryPatches: IEntryPatch[]) => {
      if (!isEmbedded()) return;
      for (const ep of entryPatches) {
        if (ep.type !== "tool_call" || !ep.tool_name) continue;
        if (!isTerminalToolStatus(ep.tool_status)) continue;
        const key = `${interactionId}:${ep.message_id}:${ep.tool_status}`;
        if (notifiedToolsRef.current.has(key)) continue;
        notifiedToolsRef.current.add(key);
        window.parent.postMessage(
          {
            type: "helix-agent-tool",
            tool: ep.tool_name,
            status: ep.tool_status,
            session_id: currentSessionId,
          },
          "*",
        );
      }
    },
    [currentSessionId],
  );

  const handleWebsocketEvent = useCallback(
    (parsedData: IWebsocketEvent) => {
      if (!currentSessionId) return;

      if ((parsedData.type as string) === "step_info") {
        const stepInfo = parsedData.step_info;

        setStepInfos((prev) => {
          const currentSteps = prev.get(currentSessionId) || [];
          const updatedSteps = [...currentSteps, stepInfo];
          return new Map(prev).set(currentSessionId, updatedSteps);
        });
      }

      if (
        parsedData.type === WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE &&
        parsedData.worker_task_response
      ) {
        const workerResponse = parsedData.worker_task_response;

        // Use requestAnimationFrame to batch updates and prevent UI blocking
        requestAnimationFrame(() => {
          setCurrentResponses((prev) => {
            const current = prev.get(currentSessionId) || {};
            let updatedInteraction: StreamingInteraction = { ...current };

            if (workerResponse.type === WORKER_TASK_RESPONSE_TYPE_PROGRESS) {
              if (workerResponse.status) {
                updatedInteraction.status = workerResponse.status;
              }
            }

            // Store the latest state in the ref
            const newMap = new Map(prev).set(
              currentSessionId,
              updatedInteraction,
            );
            return newMap;
          });
        });
      }

      // If there's a session update with state changes
      if (parsedData.type === "session_update" && parsedData.session) {
        // Discard events for a different session to prevent cross-session contamination
        if (
          parsedData.session.id &&
          parsedData.session.id !== currentSessionId
        ) {
          return;
        }

        const newInteractionCount =
          parsedData.session.interactions?.length || 0;

        // Always reject session updates with 0 interactions - these are invalid
        if (newInteractionCount === 0) {
          return;
        }

        // useGetSession suffixes its query key with 'full' (interactions
        // included) or 'skip' (interactions omitted; EmbeddedSessionView
        // path). setQueryData/getQueryData require an exact key match —
        // unlike invalidateQueries, which prefix-matches — so we must use
        // the suffixed keys to actually reach the queries that consumers
        // read. Read from 'full' (the only variant that carries
        // interactions for the stale-update count check); write to both.
        const fullKey = [...GET_SESSION_QUERY_KEY(currentSessionId), 'full'];
        const skipKey = [...GET_SESSION_QUERY_KEY(currentSessionId), 'skip'];

        // Get current session data to compare interaction counts
        // NOTE: React Query cache stores { data: TypesSession } (Axios response format)
        const cachedResponse = queryClient.getQueryData(fullKey) as
          | { data?: TypesSession }
          | undefined;
        const currentSessionData = cachedResponse?.data;
        if (currentSessionData && currentSessionData.interactions) {
          const currentInteractionCount =
            currentSessionData.interactions.length;
          // Reject updates with fewer interactions than current (stale updates)
          if (newInteractionCount < currentInteractionCount) {
            return;
          }
        }

        const lastInteraction =
          parsedData.session.interactions?.[
            parsedData.session.interactions.length - 1
          ];

        if (!lastInteraction) return;

        // CRITICAL: Update React Query cache directly with session data from WebSocket
        // This prevents the race condition where:
        // 1. WebSocket sends session_update with state=complete
        // 2. isLive becomes false, InteractionLiveStream stops rendering
        // 3. Interaction component renders with OLD cached data (before refetch completes)
        // By updating cache immediately, the Interaction component gets fresh data
        //
        // IMPORTANT: Preserve the existing session config to prevent desktop stream
        // reconnection flicker. The session_update WebSocket message may carry a stale
        // external_agent_status (e.g. "starting" when it's actually "running"), which
        // would cause useSandboxState to briefly flip isRunning and flash "Reconnecting...".
        const writeMerged = (
          stripInteractions: boolean,
        ) => (oldData: { data?: TypesSession } | undefined) => {
          const existingConfig = oldData?.data?.config;
          const merged: TypesSession = {
            ...parsedData.session,
            ...(existingConfig ? { config: existingConfig } : {}),
          };
          if (stripInteractions) {
            // The 'skip' variant fetches without interactions; never seed
            // it with interactions or it'll override the paginated source.
            delete (merged as Partial<TypesSession>).interactions;
          }
          return { data: merged };
        };
        queryClient.setQueryData(fullKey, writeMerged(false));
        queryClient.setQueryData(skipKey, writeMerged(true));

        // Update currentResponses with the latest interaction state
        // This ensures useLiveInteraction will receive the updated state
        // CRITICAL: Include response_message for external agent streaming (WebSocket-based, not SSE)
        if (lastInteraction.id) {
          requestAnimationFrame(() => {
            setCurrentResponses((prev) => {
              const current = prev.get(currentSessionId) || {};
              // IMPORTANT: Only fall back to current values if the interaction ID matches
              // Otherwise we'd show stale content from a previous interaction
              const isSameInteraction = current.id === lastInteraction.id;

              // When it's a different interaction, start fresh - don't spread current
              const updatedInteraction: Partial<TypesInteraction> & { response_entries?: ResponseEntry[] } =
                isSameInteraction
                  ? {
                      ...current,
                      id: lastInteraction.id,
                      state: lastInteraction.state,
                      prompt_message:
                        lastInteraction.prompt_message ||
                        current.prompt_message,
                      response_message:
                        lastInteraction.response_message ||
                        current.response_message,
                      // Preserve streaming entries — session_update doesn't carry them
                      response_entries: (current as any).response_entries,
                    }
                  : {
                      // New interaction - start with clean slate, only use server data
                      id: lastInteraction.id,
                      state: lastInteraction.state,
                      prompt_message: lastInteraction.prompt_message,
                      response_message: lastInteraction.response_message,
                    };

              const newMap = new Map(prev).set(
                currentSessionId,
                updatedInteraction,
              );
              return newMap;
            });
          });
        }
      }

      // OPTIMIZED: Handle single interaction updates (reduces O(n) to O(1) updates)
      // This is used for streaming updates from external agents (Zed) where we only
      // need to update a single interaction, not replace the entire session
      if (parsedData.type === "interaction_update" && parsedData.interaction) {
        const updatedInteraction = parsedData.interaction;

        // Clear patch entries ref since we have the full interaction now
        if (updatedInteraction.id) {
          patchEntriesRef.current.delete(updatedInteraction.id);
        }

        // When interaction is complete, cancel any pending patch RAF to prevent
        // a stale patch callback from overwriting the final response_message
        if (
          updatedInteraction.state ===
          TypesInteractionState.InteractionStateComplete
        ) {
          patchPendingRef.current = false;
          // The authoritative content has arrived, so a baseline we gave up on
          // is no longer relevant. Releasing the suppression here (rather than
          // on any interaction_update) keeps a mid-turn update from restarting
          // the resync cycle it exists to stop.
          if (updatedInteraction.id) {
            resyncingRef.current.delete(updatedInteraction.id);
          }
        }

        // Surgically update just this interaction in the React Query cache
        queryClient.setQueryData(
          GET_SESSION_QUERY_KEY(currentSessionId),
          (oldData: { data?: TypesSession } | undefined) => {
            if (!oldData?.data) return oldData;

            const session = oldData.data;
            const interactions = [...(session.interactions || [])];

            // Find and update the specific interaction
            const idx = interactions.findIndex(
              (i) => i.id === updatedInteraction.id,
            );
            if (idx >= 0) {
              interactions[idx] = updatedInteraction;
            } else {
              // New interaction - append it
              interactions.push(updatedInteraction);
            }

            return { data: { ...session, interactions } };
          },
        );

        // Also update currentResponses for live streaming display
        if (updatedInteraction.id) {
          // When interaction is complete, update synchronously (not via RAF) to avoid
          // race conditions where a stale patch RAF fires after this and overwrites
          // the final content with truncated streaming data
          const isComplete =
            updatedInteraction.state ===
            TypesInteractionState.InteractionStateComplete;

          const doUpdate = () => {
            setCurrentResponses((prev) => {
              const current = prev.get(currentSessionId) || {};
              const isSameInteraction = current.id === updatedInteraction.id;

              // When complete, use server's response_message and response_entries directly.
              // Do NOT fall back to current values which may be truncated streaming data.
              // Storing response_entries here prevents the 3s poll from overwriting the
              // correct final entries with stale pre-completion data from the DB.
              const serverEntries = (updatedInteraction as any)?.response_entries as ResponseEntry[] | undefined;
              const updated: StreamingInteraction = isSameInteraction
                ? {
                    ...current,
                    id: updatedInteraction.id,
                    state: updatedInteraction.state,
                    prompt_message:
                      updatedInteraction.prompt_message ||
                      current.prompt_message,
                    response_message: isComplete
                      ? updatedInteraction.response_message // Complete: use server data only
                      : updatedInteraction.response_message ||
                        current.response_message,
                    ...(isComplete && serverEntries ? { response_entries: serverEntries } : {}),
                  }
                : {
                    // New interaction - start with clean slate
                    id: updatedInteraction.id,
                    state: updatedInteraction.state,
                    prompt_message: updatedInteraction.prompt_message,
                    response_message: updatedInteraction.response_message,
                    ...(isComplete && serverEntries ? { response_entries: serverEntries } : {}),
                  };

              const newMap = new Map(prev).set(currentSessionId, updated);
              return newMap;
            });
          };

          if (isComplete) {
            // Synchronous update for completion — prevents RAF race condition
            doUpdate();
          } else {
            requestAnimationFrame(doUpdate);
          }
        }
      }

      // ENTRY-BASED STREAMING: Handle per-entry delta updates from Go server.
      // Each entry gets its own string patch so the frontend maintains a ResponseEntry[]
      // with correct type boundaries (text vs tool_call) during streaming.
      // CRITICAL: We do NOT update React Query cache here — that would create new
      // interactions arrays and trigger re-renders of the entire session. The cache is
      // only updated on completion (via interaction_update or session_update).
      if (
        parsedData.type === WEBSOCKET_EVENT_TYPE_INTERACTION_PATCH &&
        parsedData.interaction_id
      ) {
        const interactionId = parsedData.interaction_id;
        const entryPatches = parsedData.entry_patches;
        const entryCount = parsedData.entry_count;

        // An interaction we have never rendered needs to enter the list before
        // any of this can show. See seenInteractionsRef.
        //
        // Only for someone who is NOT the owner: the sender's own client gets
        // this interaction through the debounced invalidation below, so doing
        // it here as well is a wasted fetch on every turn for every user. The
        // cache check covers the rest — a viewer who already refetched, or a
        // second tab that is already rendering it.
        if (
          !seenInteractionsRef.current.has(interactionId) &&
          parsedData.owner !== currentUserIdRef.current &&
          !isInRenderedList(interactionId)
        ) {
          seenInteractionsRef.current.add(interactionId);
          queryClient.invalidateQueries({
            queryKey: ["interactions", currentSessionId],
          });
        }

        if (entryPatches && entryCount) {
          // Tool notifications go out BEFORE the baseline checks below. They
          // carry no streamed content, so they stay correct even when the
          // entries we hold do not — and a tool completing inside a resync
          // window is exactly when a host most needs to know to refetch.
          notifyHostOfToolCalls(interactionId, entryPatches);

          // Baseline already given up on for this interaction — see
          // resyncingRef. Nothing below can succeed until the completed
          // interaction arrives, and retrying it every 50ms is a refetch storm.
          if (resyncingRef.current.has(interactionId)) return;

          const currentEntries = patchEntriesRef.current.get(interactionId) || [];
          // Grow array to entry_count if new entries appeared
          while (currentEntries.length < entryCount) {
            currentEntries.push({ type: "text", content: "", message_id: "" });
          }

          // Deltas are only meaningful against the baseline they were computed
          // from, and a baseline that is gone cannot be rebuilt from more
          // deltas — only from the database. hasLostBaseline documents the
          // three ways it goes bad; all three render a wrong reply silently.
          if (hasLostBaseline(currentEntries, entryPatches)) {
            resyncingRef.current.add(interactionId);
            patchEntriesRef.current.delete(interactionId);
            queryClient.invalidateQueries({
              queryKey: ["interactions", currentSessionId],
            });
            return;
          }
          // Apply each entry patch
          for (const ep of entryPatches) {
            if (ep.index < currentEntries.length) {
              currentEntries[ep.index] = {
                type: ep.type as "text" | "tool_call",
                content: applyPatch(
                  currentEntries[ep.index].content,
                  ep.patch_offset,
                  ep.patch,
                  ep.total_length,
                ),
                message_id: ep.message_id,
                tool_name: ep.tool_name || currentEntries[ep.index].tool_name,
                tool_status: ep.tool_status || currentEntries[ep.index].tool_status,
              };
            }
          }
          patchEntriesRef.current.set(interactionId, currentEntries);
        }

        // Batch state update via RAF to avoid per-patch re-renders
        if (!patchPendingRef.current) {
          patchPendingRef.current = true;
          requestAnimationFrame(() => {
            patchPendingRef.current = false;

            // Shallow clone so React sees a new reference (the ref array is mutated in place)
            const rawEntries = patchEntriesRef.current.get(interactionId);
            const latestEntries = rawEntries ? [...rawEntries] : undefined;

            setCurrentResponses((prev) => {
              const current = prev.get(currentSessionId!) || {};

              const isSameInteraction = current.id === interactionId;
              const updated: StreamingInteraction = isSameInteraction
                ? {
                    ...current,
                    id: interactionId,
                    response_entries: latestEntries,
                  }
                : {
                    id: interactionId,
                    response_entries: latestEntries,
                  };
              return new Map(prev).set(currentSessionId!, updated);
            });
          });
        }
      }
    },
    [currentSessionId],
  );

  useEffect(() => {
    // With BFF auth, session cookie is automatically sent with WebSocket connections
    if (!currentSessionId) return;

    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsHost = window.location.host;
    const url = `${wsProtocol}//${wsHost}/api/v1/ws/user?session_id=${currentSessionId}`;
    const rws = new ReconnectingWebSocket(url);

    const messageHandler = (event: MessageEvent<any>) => {
      const parsedData = JSON.parse(event.data) as IWebsocketEvent;
      if (parsedData.session_id !== currentSessionId) return;

      handleWebsocketEvent(parsedData);

      if (parsedData.step_info && parsedData.step_info.type === "thinking") {
        // Don't reload on thinking info events as we will get a lot of them
        return;
      }

      if (parsedData.type === WEBSOCKET_EVENT_TYPE_INTERACTION_PATCH) {
        // Don't trigger query invalidation for streaming patches — they're
        // high-frequency events handled entirely through currentResponses.
        // The cache will be updated on completion via interaction_update.
        return;
      }

      // Use debounced invalidation to prevent excessive re-renders
      if (invalidateTimerRef.current) {
        clearTimeout(invalidateTimerRef.current);
      }
      invalidateTimerRef.current = setTimeout(() => {
        // Invalidate paginated interactions cache so new/completed interactions appear
        queryClient.invalidateQueries({
          queryKey: ["interactions", currentSessionId],
        });
        // Invalidate session steps (thinking/progress indicators)
        queryClient.invalidateQueries({
          queryKey: SESSION_STEPS_QUERY_KEY(currentSessionId),
        });
        // NOTE: We intentionally do NOT invalidate GET_SESSION_QUERY_KEY here.
        // useSandboxState polls the session every 3s for desktop state. Invalidating
        // the session cache on every chat WebSocket event causes useSandboxState to
        // refetch, which briefly flips isRunning and flashes the "Reconnecting..."
        // overlay on the desktop stream. Interactions are now served via
        // LIST_INTERACTIONS_QUERY_KEY (since PR #2146), so the session cache
        // invalidation is no longer needed for chat updates.
        invalidateSessionsQuery(queryClient);
        invalidateTimerRef.current = null;
      }, 500);
    };

    const openHandler = () => {
      if (wsWasDisconnectedRef.current) {
        // Reconnection after a dropout: clear stale streaming state and refresh
        // from the database so any updates missed during the outage are loaded.
        wsWasDisconnectedRef.current = false;
        setCurrentResponses((prev) => {
          const next = new Map(prev);
          next.delete(currentSessionId!);
          return next;
        });
        patchEntriesRef.current.clear();
        // A reconnect is the one event that can restore a lost baseline: the
        // server sends a catch-up snapshot on the new connection. So give up
        // giving up — let the invariant judge the next patch on its merits.
        resyncingRef.current.clear();
        queryClient.invalidateQueries({ queryKey: ["interactions", currentSessionId] });
        queryClient.invalidateQueries({ queryKey: SESSION_STEPS_QUERY_KEY(currentSessionId!) });
      }
      setWsConnected(true);
    };
    const closeHandler = () => {
      wsWasDisconnectedRef.current = true;
      setWsConnected(false);
    };

    rws.addEventListener("message", messageHandler);
    rws.addEventListener("open", openHandler);
    rws.addEventListener("close", closeHandler);

    // After a laptop sleep / wifi switch the socket is frequently left half-open:
    // the browser reports it OPEN but no data flows and no 'close' event fires, so
    // reconnecting-websocket never retries and live streaming silently stalls until
    // a manual page refresh. Proactively force a reconnect when the tab regains
    // focus or the network comes back — the 'open' handler then clears stale state
    // and refetches, and the server's late-joiner catch-up (websocket_server_user.go)
    // resumes streaming, exactly as a refresh does today.
    const forceReconnectOnResume = () => {
      if (document.visibilityState === "visible") rws.reconnect();
    };
    const handleOnline = () => rws.reconnect();
    document.addEventListener("visibilitychange", forceReconnectOnResume);
    window.addEventListener("online", handleOnline);

    return () => {
      document.removeEventListener("visibilitychange", forceReconnectOnResume);
      window.removeEventListener("online", handleOnline);
      rws.removeEventListener("message", messageHandler);
      rws.removeEventListener("open", openHandler);
      rws.removeEventListener("close", closeHandler);
      rws.close();
      // Reset disconnect tracking on intentional session change so the next
      // session's initial connect is not mistaken for a reconnect.
      wsWasDisconnectedRef.current = false;
      setWsConnected(false);
      // Clear any pending invalidation timer
      if (invalidateTimerRef.current) {
        clearTimeout(invalidateTimerRef.current);
        invalidateTimerRef.current = null;
      }
    };
  }, [currentSessionId]);

  const NewInference = async ({
    regenerate = false,
    type,
    message,
    messages,
    appId = "",
    projectId = "",
    assistantId = "",
    provider = "",
    modelName = "",
    reasoningEffort = "",
    sessionId = "",
    interactionId = "",
    orgId = "",
    image = undefined,
    image_filename = undefined,
    attachedImages = [],
    agentType = "helix_agent",
    externalAgentConfig = undefined,
    interrupt = true, // Default to interrupt for backwards compatibility
    sessionRole = "",
  }: NewInferenceParams): Promise<TypesSession> => {
    // Clear both buffer and history for new sessions
    messageBufferRef.current.delete(sessionId);
    messageHistoryRef.current.delete(sessionId);

    // Clear stepInfos for the session to reset the Glowing Orb list between interactions
    setStepInfos((prev) => {
      const newMap = new Map(prev);
      if (sessionId) {
        newMap.delete(sessionId);
      }
      return newMap;
    });

    // Construct the content parts first
    const currentContentParts: any[] = [];
    let determinedContentType: string = "text"; // Default for MessageContent.content_type

    // Add text part if message is provided
    if (message) {
      currentContentParts.push({
        type: "text",
        text: message,
      });
    }

    // Handle attached images
    if (attachedImages && attachedImages.length > 0) {
      for (const file of attachedImages) {
        const reader = new FileReader();
        const imageData = await new Promise<string>((resolve) => {
          reader.onloadend = () => resolve(reader.result as string);
          reader.readAsDataURL(file);
        });

        currentContentParts.push({
          type: "image_url",
          image_url: {
            url: imageData,
          },
        });
      }
      determinedContentType = "multimodal_text";
    } else if (image && image_filename) {
      currentContentParts.push({
        type: "image_url",
        image_url: {
          url: image,
        },
      });
      determinedContentType = "multimodal_text";
    } else if (!message) {
      console.warn("NewInference called with no message and no image.");
    }

    // This is the payload for Message.Content, matching the Go types.MessageContent struct
    const messagePayloadContent = {
      content_type: determinedContentType,
      parts: currentContentParts,
    };

    // Serialize external agent config to ensure no React elements are included
    const sanitizedExternalAgentConfig = externalAgentConfig
      ? {
          workspace_dir: externalAgentConfig.workspace_dir,
          project_path: externalAgentConfig.project_path,
          env_vars: externalAgentConfig.env_vars,
          auto_connect_rdp: externalAgentConfig.auto_connect_rdp,
        }
      : undefined;

    // Assign the constructed content to the message
    const sessionChatRequest: ISessionChatRequest = {
      regenerate: regenerate,
      type, // This is ISessionType (e.g. text, image) for the overall session/request
      stream: true,
      app_id: appId,
      project_id: projectId,
      organization_id: orgId,
      assistant_id: assistantId,
      interaction_id: interactionId,
      provider: provider,
      model: modelName,
      reasoning_effort: reasoningEffort || undefined,
      session_id: sessionId,
      agent_type: agentType,
      external_agent_config: sanitizedExternalAgentConfig,
      interrupt: interrupt,
      session_role: sessionRole || undefined,
      messages: [
        {
          role: "user",
          content: messagePayloadContent as any, // Use the correctly structured object, cast to any to bypass TS type mismatch
        },
      ],
    };

    // If messages are supplied in the request, overwrite the default user message
    if (messages && messages.length > 0) {
      sessionChatRequest.messages = messages;
    }

    console.log("📡 Sending session chat request:", {
      url: "/api/v1/sessions/chat",
      payload: sessionChatRequest,
      modelName: sessionChatRequest.model,
      agentType: sessionChatRequest.agent_type,
      externalAgentConfig: sessionChatRequest.external_agent_config,
    });

    try {
      // With BFF auth, session cookie is sent automatically with same-origin requests
      // Include CSRF token for protection against cross-site request forgery
      const csrfToken = getCSRFToken();
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
      };
      if (csrfToken) {
        headers["X-CSRF-Token"] = csrfToken;
      }

      const response = await fetch("/api/v1/sessions/chat", {
        method: "POST",
        headers,
        credentials: "same-origin",
        body: JSON.stringify(sessionChatRequest),
      });

      if (!response.ok) {
        throw new Error("Failed to create or update session");
      }

      if (!response.body) {
        throw new Error("Response body is null");
      }

      // Read interaction ID from response header (set by handleStreamingSession)
      // This allows useLiveInteraction to match SSE data to the correct interaction
      const sseInteractionId =
        response.headers.get("X-Interaction-ID") || undefined;

      const reader = response.body.getReader();
      let sessionData: TypesSession | null = null;
      let promiseResolved = false;
      let decoder = new TextDecoder();
      let buffer = "";

      const processStream = new Promise<void>((resolveStream, rejectStream) => {
        const processChunk = (chunk: string) => {
          const lines = chunk.split("\n");

          if (buffer) {
            lines[0] = buffer + lines[0];
            buffer = "";
          }

          if (!chunk.endsWith("\n")) {
            buffer = lines.pop() || "";
          }

          for (const line of lines) {
            const trimmedLine = line.trim();
            if (!trimmedLine) continue;

            if (trimmedLine.startsWith("data: ")) {
              const data = trimmedLine.slice(6); // 'data: ' = 6 chars

              // Check for SSE [DONE] marker (can come as "[DONE]" or " [DONE]" with leading space)
              if (data.trim() === "[DONE]" || data === "[DONE]") {
                console.log("[SSE] Received [DONE] marker - completing stream");

                // Invalidate the session query
                queryClient.invalidateQueries({
                  queryKey: GET_SESSION_QUERY_KEY(sessionId),
                });

                if (sessionData?.id) {
                  // Final flush of any remaining content
                  flushMessageBuffer(sessionData.id);
                  setCurrentResponses((prev) => {
                    const newMap = new Map(prev);
                    newMap.delete(sessionData?.id || "");
                    return newMap;
                  });
                }
                resolveStream();
                return;
              }

              try {
                const parsedData = JSON.parse(data);
                if (!sessionData) {
                  sessionData = parsedData;
                  if (sessionData?.id) {
                    // Set the current session ID first
                    clearSessionData(sessionData.id);

                    // Explicitly clear any existing data for this session
                    messageBufferRef.current.set(sessionData.id, []);
                    messageHistoryRef.current.set(sessionId, "");

                    // Initialize with empty response until we get content
                    // Include interaction ID so useLiveInteraction can match SSE data
                    // to the correct interaction (prevents stale content from previous interaction)
                    setCurrentResponses((prev) => {
                      return new Map(prev).set(sessionData?.id || "", {
                        id: sseInteractionId,
                        prompt_message: "",
                      });
                    });

                    if (parsedData.choices?.[0]?.delta?.content) {
                      addMessageChunk(
                        sessionData.id,
                        parsedData.choices[0].delta.content,
                      );
                    }

                    if (!promiseResolved) {
                      promiseResolved = true;
                    }
                  } else {
                    console.error(
                      "Invalid session data received:",
                      sessionData,
                    );
                    rejectStream(new Error("Invalid session data"));
                  }
                } else if (parsedData.choices?.[0]?.delta?.content) {
                  addMessageChunk(
                    sessionData?.id || "",
                    parsedData.choices[0].delta.content,
                  );
                }
              } catch (error) {
                console.error("Error parsing SSE data:", error);
                console.warn("Continuing despite parse error");
              }
            }
          }
        };

        const pump = async () => {
          try {
            while (true) {
              const { done, value } = await reader.read();

              if (done) {
                if (buffer) {
                  processChunk(buffer);
                }
                if (sessionData?.id) {
                  flushMessageBuffer(sessionData.id);
                }
                resolveStream();
                break;
              }

              const chunk = decoder.decode(value, { stream: true });
              processChunk(chunk);
            }
          } catch (error) {
            rejectStream(error);
          }
        };

        pump().catch((error) => {
          console.error("Pump error:", error);
          rejectStream(error);
        });
      });

      await Promise.race([
        new Promise<void>((resolve) => {
          const checkResolved = () => {
            if (promiseResolved) {
              resolve();
            } else {
              setTimeout(checkResolved, 10);
            }
          };
          checkResolved();
        }),
        new Promise<void>((_, reject) =>
          setTimeout(
            () => reject(new Error("Timeout waiting for first chunk")),
            30000,
          ),
        ),
      ]);

      processStream.catch((error) => {
        console.error("Error processing stream:", error);
      });

      if (!sessionData) {
        throw new Error("Failed to receive session data");
      }

      console.log("streaming done");

      return sessionData;
    } catch (error) {
      console.error("Error in NewInference:", error);
      throw error;
    }
  };

  const updateCurrentResponse = (
    sessionId: string,
    interaction: Partial<TypesInteraction>,
  ) => {
    setCurrentResponses((prev) => {
      const current = prev.get(sessionId) || {};
      return new Map(prev).set(sessionId, { ...current, ...interaction });
    });
  };

  const value = {
    NewInference,
    setCurrentSessionId: clearSessionData,
    currentResponses,
    updateCurrentResponse,
    stepInfos,
    wsConnected,
  };

  return (
    <StreamingContext.Provider value={value}>
      {children}
    </StreamingContext.Provider>
  );
};
