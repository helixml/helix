import { useState, useEffect, useMemo } from "react";
import { INTERACTION_STATE_COMPLETE } from "../types";
import { useStreaming } from "../contexts/streaming";
import { TypesInteraction, TypesInteractionState } from "../api/api";

interface LiveInteractionResult {
    message: string;
    status: string;
    isComplete: boolean;
    isStale: boolean;
    stepInfos: any[]; // Add this line
}

const useLiveInteraction = (
    sessionId: string,
    initialInteraction: TypesInteraction | null,
    staleThreshold = 10000,
): LiveInteractionResult => {
    const [interaction, setInteraction] = useState<TypesInteraction | null>(
        initialInteraction,
    );
    const { currentResponses, stepInfos } = useStreaming();
    const [recentTimestamp, setRecentTimestamp] = useState(Date.now());
    const [isStale, setIsStale] = useState(false);
    // Preserve the last known message to prevent blank screen during completion
    // This fixes the flickering issue where streaming context clears before interaction updates
    const [lastKnownMessage, setLastKnownMessage] = useState<string>("");
    // Track the current interaction ID to detect when a new interaction starts
    const [currentInteractionId, setCurrentInteractionId] = useState<string | undefined>(initialInteraction?.id);

    // Reset lastKnownMessage when interaction ID changes (new interaction started)
    // This prevents showing stale content from the previous interaction
    useEffect(() => {
        if (initialInteraction?.id && initialInteraction.id !== currentInteractionId) {
            setCurrentInteractionId(initialInteraction.id);
            setLastKnownMessage("");
        }
    }, [initialInteraction?.id, currentInteractionId]);

    // Removed excessive debug logging

    const isAppTryHelixDomain = useMemo(() => {
        return window.location.hostname === "app.helix.ml";
    }, []);

    useEffect(() => {
        if (sessionId) {
            const currentResponse = currentResponses.get(sessionId);
            // CRITICAL: Only use currentResponse if it matches the initialInteraction we're rendering
            // currentResponses is keyed by sessionId, so it may contain data from a different interaction
            // Match by interaction ID when available, but also accept responses with no ID
            // (SSE streaming path doesn't set .id on currentResponses — it only sets prompt_message/response_message)
            const responseMatchesInteraction = !currentResponse?.id || currentResponse?.id === initialInteraction?.id;

            if (currentResponse && responseMatchesInteraction) {
                // SSE streaming active - use currentResponses (matches our interaction)
                setInteraction(
                    (
                        prevInteraction: TypesInteraction | null,
                    ): TypesInteraction => {
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
                setRecentTimestamp(Date.now());
                // Reset stale state when we get an update
                if (isStale) {
                    setIsStale(false);
                }
            } else {
                // No SSE streaming OR response is for different interaction - use initialInteraction
                // CRITICAL: This enables external agent streaming via WebSocket session updates
                if (initialInteraction) {
                    setInteraction(initialInteraction);
                    // Also preserve message from query updates
                    if (initialInteraction.response_message) {
                        setLastKnownMessage(initialInteraction.response_message);
                        setRecentTimestamp(Date.now());
                    }
                }
            }
        }
    }, [sessionId, currentResponses, isStale, initialInteraction]);

    // Update lastKnownMessage when interaction.response_message changes
    // CRITICAL: Only update if the interaction ID matches to prevent stale content
    useEffect(() => {
        if (interaction?.response_message && interaction?.id === currentInteractionId) {
            setLastKnownMessage(interaction.response_message);
        }
    }, [interaction?.response_message, interaction?.id, currentInteractionId]);

    // Check for stale state, but only update when it changes from non-stale to stale
    useEffect(() => {
        // Only run stale check if we're on the tryhelix domain
        if (!isAppTryHelixDomain) return;

        const checkStale = () => {
            const shouldBeStale = Date.now() - recentTimestamp > staleThreshold;
            // Only update state if it's different (prevents unnecessary re-renders)
            if (shouldBeStale !== isStale) {
                setIsStale(shouldBeStale);
            }
        };

        // Check immediately and then set up interval
        checkStale();
        const intervalID = setInterval(checkStale, 1000);

        return () => clearInterval(intervalID);
    }, [recentTimestamp, staleThreshold, isStale, isAppTryHelixDomain]);

    // DEBUG: Removed render-time console.log that was causing excessive logging
    // This was running on EVERY render during streaming (100+ times/sec)
    // causing performance issues and blocking screenshot updates

    // CRITICAL: Only use interaction.response_message if it matches the current interaction
    // This prevents showing stale content from a previous interaction while waiting for new data
    const interactionMatchesCurrent = interaction?.id === currentInteractionId;
    const safeResponseMessage = interactionMatchesCurrent ? interaction?.response_message : undefined;
    const message = safeResponseMessage || lastKnownMessage || "";

    const result = {
        // Use interaction message if available, otherwise fall back to preserved message
        // This prevents blank screen when streaming context clears during completion
        message,
        status: interaction?.state || "",
        isComplete:
            interaction?.state ===
            TypesInteractionState.InteractionStateComplete,
        isStale,
        stepInfos: stepInfos.get(sessionId) || [],
    };



    return result;
};

export default useLiveInteraction;
