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

    // Removed excessive debug logging

    const isAppTryHelixDomain = useMemo(() => {
        return window.location.hostname === "app.helix.ml";
    }, []);

    useEffect(() => {
        if (sessionId) {
            const currentResponse = currentResponses.get(sessionId);
            if (currentResponse) {
                // SSE streaming active - use currentResponses
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
                // No SSE streaming - use initialInteraction (updated via React Query refetch)
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
    useEffect(() => {
        if (interaction?.response_message) {
            setLastKnownMessage(interaction.response_message);
        }
    }, [interaction?.response_message]);

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

    // DEBUG: Log what we're working with
    if (interaction?.state === 'waiting' && interaction?.response_message) {
        console.log('[useLiveInteraction] Partial response for waiting interaction:', {
            sessionId,
            interactionId: interaction.id,
            state: interaction.state,
            responseLength: interaction.response_message?.length,
            responsePreview: interaction.response_message?.substring(0, 50),
            currentResponsesHas: currentResponses.has(sessionId),
            currentResponsesMessage: currentResponses.get(sessionId)?.response_message?.substring(0, 50)
        });
    }

    const result = {
        // Use interaction message if available, otherwise fall back to preserved message
        // This prevents blank screen when streaming context clears during completion
        message: interaction?.response_message || lastKnownMessage || "",
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
