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
                // Removed excessive debug logging
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
                // Removed excessive debug logging
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

    // Debug: Targeted logging for blank screen flickering issue
    if (interaction?.state === "waiting" && !result.message) {
        console.log("🔍 BLANK_SCREEN: In waiting state with no message", {
            sessionId,
            interactionState: interaction?.state,
            hasStreamingContext: currentResponses.has(sessionId),
            streamingMessage: currentResponses.get(sessionId)?.response_message ? "EXISTS" : "MISSING",
            lastKnownMessage: lastKnownMessage ? "PRESERVED" : "MISSING"
        });
    }
    
    // Log when we're using preserved message during completion
    if (interaction?.state === "complete" && !interaction?.response_message && lastKnownMessage) {
        console.log("🔄 USING_PRESERVED: Using preserved message during completion", {
            sessionId,
            preservedMessageLength: lastKnownMessage.length,
            interactionHasMessage: !!interaction?.response_message
        });
    }
    
    // Log state transitions that might cause flickering
    if (interaction?.state === "complete" && result.message) {
        console.log("✅ COMPLETE: Interaction finished with message", {
            sessionId,
            messageLength: result.message.length,
            messageSource: interaction?.response_message ? "INTERACTION" : "PRESERVED",
            isStale: result.isStale
        });
    }

    // Minimal logging for critical state changes only
    useEffect(() => {
        if (interaction?.state === "complete" && !result.message) {
            console.log("🚨 HOOK_CRITICAL: Complete state but no message", {
                sessionId,
                preservedMessageExists: !!lastKnownMessage
            });
        }
    }, [sessionId, interaction?.state, result.message, lastKnownMessage]);

    return result;
};

export default useLiveInteraction;
