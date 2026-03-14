import { useState, useEffect } from "react";
import { useStreaming } from "../contexts/streaming";
import { TypesInteraction, TypesInteractionState } from "../api/api";

interface LiveInteractionResult {
    message: string;
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
    const [currentInteractionId, setCurrentInteractionId] = useState<string | undefined>(initialInteraction?.id);

    // Reset lastKnownMessage when interaction ID changes (new interaction started)
    // This prevents showing stale content from the previous interaction
    useEffect(() => {
        if (initialInteraction?.id && initialInteraction.id !== currentInteractionId) {
            setCurrentInteractionId(initialInteraction.id);
            setLastKnownMessage("");
        }
    }, [initialInteraction?.id, currentInteractionId]);

    useEffect(() => {
        if (sessionId) {
            const currentResponse = currentResponses.get(sessionId);
            // CRITICAL: Only use currentResponse if it matches the initialInteraction we're rendering
            // currentResponses is keyed by sessionId, so it may contain data from a different interaction
            const responseMatchesInteraction = currentResponse?.id === initialInteraction?.id;

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
        if (interaction?.response_message && interaction?.id === currentInteractionId) {
            setLastKnownMessage(interaction.response_message);
        }
    }, [interaction?.response_message, interaction?.id, currentInteractionId]);

    // CRITICAL: Only use interaction.response_message if it matches the current interaction
    // This prevents showing stale content from a previous interaction while waiting for new data
    const interactionMatchesCurrent = interaction?.id === currentInteractionId;
    const safeResponseMessage = interactionMatchesCurrent ? interaction?.response_message : undefined;
    const message = safeResponseMessage || lastKnownMessage || "";

    return {
        // Use interaction message if available, otherwise fall back to preserved message
        // This prevents blank screen when streaming context clears during completion
        message,
        status: interaction?.state || "",
        isComplete:
            interaction?.state ===
            TypesInteractionState.InteractionStateComplete,
        stepInfos: stepInfos.get(sessionId) || [],
    };
};

export default useLiveInteraction;
