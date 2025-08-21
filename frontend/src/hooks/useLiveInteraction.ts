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

    // Debug logging for blank screen issue
    console.log("useLiveInteraction Debug:", {
        sessionId,
        initialInteractionId: initialInteraction?.id,
        currentInteractionId: interaction?.id,
        hasCurrentResponse: currentResponses.has(sessionId),
        currentResponseMessage:
            currentResponses
                .get(sessionId)
                ?.response_message?.substring(0, 50) + "..." || "NO MESSAGE",
        stepInfosForSession: stepInfos.get(sessionId)?.length || 0,
        isStale,
        recentTimestamp: new Date(recentTimestamp).toISOString(),
    });

    const isAppTryHelixDomain = useMemo(() => {
        return window.location.hostname === "app.helix.ml";
    }, []);

    useEffect(() => {
        if (sessionId) {
            const currentResponse = currentResponses.get(sessionId);
            if (currentResponse) {
                console.log(
                    "useLiveInteraction: Updating interaction with current response:",
                    {
                        sessionId,
                        currentResponseMessage:
                            currentResponse.response_message?.substring(0, 50) +
                                "..." || "NO MESSAGE",
                        currentResponseState: currentResponse.state,
                    },
                );
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
                setRecentTimestamp(Date.now());
                // Reset stale state when we get an update
                if (isStale) {
                    setIsStale(false);
                }
            } else {
                console.log(
                    "useLiveInteraction: No current response found for session:",
                    sessionId,
                );
                // If no streaming context but we have an initial interaction in waiting state,
                // keep using the initial interaction data
                if (
                    initialInteraction &&
                    initialInteraction.state === "waiting"
                ) {
                    setInteraction(initialInteraction);
                }
            }
        }
    }, [sessionId, currentResponses, isStale, initialInteraction]);

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

    const result = {
        message: interaction?.response_message || "",
        status: interaction?.state || "",
        isComplete:
            interaction?.state ===
            TypesInteractionState.InteractionStateComplete,
        isStale,
        stepInfos: stepInfos.get(sessionId) || [],
    };

    // Debug: Show if we're in waiting state with no message
    if (interaction?.state === "waiting" && !result.message) {
        console.log(
            "useLiveInteraction: In waiting state but no message - this causes blank screen",
            {
                sessionId,
                interactionState: interaction?.state,
                hasStreamingContext: currentResponses.has(sessionId),
                streamingResponseMessage:
                    currentResponses.get(sessionId)?.response_message,
            },
        );
    }

    console.log("useLiveInteraction: Returning result:", {
        sessionId,
        hasMessage: !!result.message,
        messageLength: result.message.length,
        status: result.status,
        isComplete: result.isComplete,
        isStale: result.isStale,
        stepInfosCount: result.stepInfos.length,
    });

    return result;
};

export default useLiveInteraction;
