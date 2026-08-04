import { useState, useEffect, useRef } from "react";
import { useStreaming } from "../contexts/streaming";
import { TypesInteraction, TypesInteractionState } from "../api/api";
import { ResponseEntry } from "../components/session/InteractionInference";
import { getInteractionDurationMs } from "../components/session/interactionDuration";

interface LiveInteractionResult {
  message: string;
  responseEntries: ResponseEntry[] | undefined;
  durationMs: number | undefined;
  status: string;
  isComplete: boolean;
  stepInfos: any[];
}

const useLiveInteraction = (
  sessionId: string,
  initialInteraction: TypesInteraction | null,
): LiveInteractionResult => {
  const [interaction, setInteraction] = useState<TypesInteraction | null>(
    initialInteraction,
  );
  const { currentResponses, stepInfos } = useStreaming();
  // Preserve the last known message to prevent blank screen during completion
  // This fixes the flickering issue where streaming context clears before interaction updates
  const [lastKnownMessage, setLastKnownMessage] = useState<string>("");
  // Track the current interaction ID to detect when a new interaction starts
  const [currentInteractionId, setCurrentInteractionId] = useState<
    string | undefined
  >(initialInteraction?.id);

  // Reset lastKnownMessage when interaction ID OR session ID changes
  // This prevents showing stale content from a previous interaction or session
  useEffect(() => {
    if (
      initialInteraction?.id &&
      initialInteraction.id !== currentInteractionId
    ) {
      setCurrentInteractionId(initialInteraction.id);
      setLastKnownMessage("");
    }
  }, [initialInteraction?.id, currentInteractionId]);

  // Reset ALL state when sessionId changes to prevent cross-session content leaks
  const prevSessionIdRef = useRef(sessionId);
  useEffect(() => {
    if (sessionId !== prevSessionIdRef.current) {
      prevSessionIdRef.current = sessionId;
      setLastKnownMessage("");
      setCurrentInteractionId(undefined);
      setInteraction(null);
    }
  }, [sessionId]);



  useEffect(() => {
    if (sessionId) {
      const currentResponse = currentResponses.get(sessionId);
      // CRITICAL: Only use currentResponse if it matches the initialInteraction we're rendering
      // currentResponses is keyed by sessionId, so it may contain data from a different interaction
      const responseMatchesInteraction =
        currentResponse?.id === initialInteraction?.id;

      if (currentResponse && responseMatchesInteraction) {
        // SSE streaming active - use currentResponses (matches our interaction)
        setInteraction(
          (prevInteraction: TypesInteraction | null): TypesInteraction => {
            if (prevInteraction === null) {
              return currentResponse as unknown as TypesInteraction;
            }
            return {
              ...prevInteraction,
              ...currentResponse,
            } as unknown as TypesInteraction;
          },
        );
        // Preserve message when we get updates
        if (currentResponse.response_message) {
          setLastKnownMessage(currentResponse.response_message);
        }
      } else {
        // No SSE streaming OR response is for different interaction - use initialInteraction
        // CRITICAL: This enables external agent streaming via WebSocket session updates
        if (initialInteraction) {
          setInteraction(initialInteraction);
          // Also preserve message from query updates
          if (initialInteraction.response_message) {
            setLastKnownMessage(initialInteraction.response_message);
          }
        }
      }
    }
  }, [sessionId, currentResponses, initialInteraction]);

  // Update lastKnownMessage when interaction.response_message changes
  // CRITICAL: Only update if the interaction ID matches to prevent stale content
  useEffect(() => {
    if (
      interaction?.response_message &&
      interaction?.id === currentInteractionId
    ) {
      setLastKnownMessage(interaction.response_message);
    }
  }, [interaction?.response_message, interaction?.id, currentInteractionId]);


  // CRITICAL: Only use interaction.response_message if it matches the current interaction
  // This prevents showing stale content from a previous interaction while waiting for new data
  const interactionMatchesCurrent = interaction?.id === currentInteractionId;
  const safeResponseMessage = interactionMatchesCurrent
    ? interaction?.response_message
    : undefined;

  // When the interaction is complete, prioritize the response_message from initialInteraction
  // (which comes from React Query cache, updated by interaction_update event with full content)
  // over lastKnownMessage (which may be truncated streaming data from the throttled patch pipeline)
  const isComplete =
    interaction?.state === TypesInteractionState.InteractionStateComplete;
  const completedMessage =
    isComplete && initialInteraction?.response_message
      ? initialInteraction.response_message
      : undefined;

  const message =
    completedMessage || safeResponseMessage || lastKnownMessage || "";

  // Response entries for completed interactions: prefer entries from currentResponses
  // (set directly from the interaction_update WebSocket event) over initialInteraction
  // (from the React Query cache / 3s poll). The 3s poll can overwrite the cache with
  // stale pre-completion data, whereas currentResponses is always set from the authoritative
  // interaction_update event which carries the fully corrected final entries.
  const streamingEntries = interactionMatchesCurrent
    ? (interaction as any)?.response_entries as ResponseEntry[] | undefined
    : undefined;
  const completedEntries =
    isComplete && (initialInteraction as any)?.response_entries
      ? ((initialInteraction as any).response_entries as ResponseEntry[])
      : undefined;
  // For complete interactions, streamingEntries (from interaction_update) takes priority
  // over completedEntries (from poll) to prevent stale poll data from winning.
  const responseEntries = isComplete
    ? (streamingEntries || completedEntries)
    : (completedEntries || streamingEntries);

  // ===== TEMPORARY INSTRUMENTATION (task 002552) — REMOVE BEFORE MERGE =====
  // Hunting the clobber: the signal is msgLen or entryCount going DOWN.
  const dbgPrevRef = useRef<{ msgLen: number; entryCount: number }>({
    msgLen: 0,
    entryCount: 0,
  });
  {
    const cr = currentResponses.get(sessionId);
    const guardMatched = cr?.id === initialInteraction?.id;
    const src = cr && guardMatched ? "LIVE" : "DB";
    const entries = (interaction as any)?.response_entries as
      | ResponseEntry[]
      | undefined;
    const msgLen = (message || "").length;
    const entryCount = entries?.length || 0;
    const prev = dbgPrevRef.current;
    const shrank = msgLen < prev.msgLen || entryCount < prev.entryCount;
    const payload = {
      src,
      crId: cr?.id,
      iiId: initialInteraction?.id,
      guardMatched,
      msgLen,
      prevMsgLen: prev.msgLen,
      lastKnownLen: lastKnownMessage.length,
      entryCount,
      prevEntryCount: prev.entryCount,
      iiMsgLen: initialInteraction?.response_message?.length || 0,
      iiEntryCount:
        ((initialInteraction as any)?.response_entries as ResponseEntry[])
          ?.length || 0,
      crEntryCount: (cr as any)?.response_entries?.length || 0,
      lastEntryTail: entries?.length
        ? entries[entries.length - 1].content.slice(-40)
        : "",
    };
    const w = window as any;
    if (!w.__liveLog) w.__liveLog = [];
    w.__liveLog.push({ t: Date.now(), shrank, ...payload });
    if (w.__liveLog.length > 3000) w.__liveLog.shift();
    if (shrank) {
      console.warn("[LIVE-SHRANK]", payload);
    }
    dbgPrevRef.current = { msgLen, entryCount };
  }
  // ===== END TEMPORARY INSTRUMENTATION =====

  const result = {
    // Use interaction message if available, otherwise fall back to preserved message
    // This prevents blank screen when streaming context clears during completion
    message,
    responseEntries,
    durationMs: getInteractionDurationMs(interaction),
    status: interaction?.state || "",
    isComplete:
      interaction?.state === TypesInteractionState.InteractionStateComplete,
    stepInfos: stepInfos.get(sessionId) || [],
  };

  return result;
};

export default useLiveInteraction;
