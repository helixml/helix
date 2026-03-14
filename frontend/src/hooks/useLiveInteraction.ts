import { useState, useEffect, useRef } from "react";
import { useStreaming } from "../contexts/streaming";
import { TypesInteraction, TypesInteractionState } from "../api/api";
import { ResponseEntry } from "../components/session/InteractionInference";

interface LiveInteractionResult {
  message: string;
  responseEntries: ResponseEntry[] | undefined;
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
              return currentResponse as TypesInteraction;
            }
            return {
              ...prevInteraction,
              ...currentResponse,
            };
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

  // Response entries: prefer completed interaction's entries (from DB, fully corrected),
  // then fall back to streaming entries from currentResponses (built from entry_patches)
  const completedEntries =
    isComplete && (initialInteraction as any)?.response_entries
      ? ((initialInteraction as any).response_entries as ResponseEntry[])
      : undefined;
  const streamingEntries = interactionMatchesCurrent
    ? (interaction as any)?.response_entries as ResponseEntry[] | undefined
    : undefined;
  const responseEntries = completedEntries || streamingEntries;

  const result = {
    // Use interaction message if available, otherwise fall back to preserved message
    // This prevents blank screen when streaming context clears during completion
    message,
    responseEntries,
    status: interaction?.state || "",
    isComplete:
      interaction?.state === TypesInteractionState.InteractionStateComplete,
    stepInfos: stepInfos.get(sessionId) || [],
  };

  return result;
};

export default useLiveInteraction;
