import React, {
    FC,
    useEffect,
    useState,
    useMemo,
    useCallback,
    useRef,
} from "react";
import Typography from "@mui/material/Typography";
import Box from "@mui/material/Box";
import { useTheme } from "@mui/material/styles";
import WaitingInQueue from "./WaitingInQueue";

import useLiveInteraction from "../../hooks/useLiveInteraction";
import Markdown from "./Markdown";
import { IServerConfig } from "../../types";
import {
    TypesInteraction,
    TypesInteractionState,
    TypesSession,
} from "../../api/api";
import ToolStepsWidget from "./ToolStepsWidget";

export const InteractionLiveStream: FC<{
    session_id: string;
    interaction: TypesInteraction;
    serverConfig?: IServerConfig;
    session: TypesSession;
    onMessageChange?: {
        (message: string): void;
    };
    onMessageUpdate?: () => void;
    onFilterDocument?: (docId: string) => void;
}> = ({
    session_id,
    serverConfig,
    session,
    interaction,
    onMessageChange,
    onMessageUpdate,
    onFilterDocument,
}) => {
    const { message, status, isStale, stepInfos, isComplete } =
        useLiveInteraction(session_id, interaction);

    // Debug logging for blank screen issue
    console.log("InteractionLiveStream Debug:", {
        session_id,
        interactionId: interaction?.id,
        interactionState: interaction?.state,
        message: message ? `${message.substring(0, 50)}...` : "NO MESSAGE",
        messageLength: message?.length || 0,
        status,
        isStale,
        stepInfosLength: stepInfos?.length || 0,
        isComplete,
        isActivelyStreaming: true, // Will be updated below
        serverConfigExists: !!serverConfig,
        filestorePrefix: serverConfig?.filestore_prefix,
    });

    // Add state to track if we're still in streaming mode or completed
    const [isActivelyStreaming, setIsActivelyStreaming] = useState(true);

    // Memoize values that don't change frequently to prevent unnecessary re-renders
    const showLoading = useMemo(
        () =>
            !message &&
            interaction.state === TypesInteractionState.InteractionStateWaiting,
        [message, status],
    );

    // Memoize the useClientURL function
    const useClientURL = useCallback(
        (url: string) => {
            if (!url) return "";
            if (!serverConfig) return "";
            return `${serverConfig.filestore_prefix}/${url}?redirect_urls=true`;
        },
        [serverConfig],
    );

    // Transform stepInfos to match ToolStepsWidget format
    const toolSteps = useMemo(
        () =>
            stepInfos.map((step, index) => ({
                id: `step-${index}`,
                name: step.name,
                icon: step.icon,
                type: step.type,
                message: step.message,
                details: {
                    arguments: {},
                },
                created: step.created || "",
            })),
        [stepInfos],
    );

    // Reset streaming state when a new interaction starts or interaction ID changes
    useEffect(() => {
        // Always reset to streaming state when interaction ID changes
        console.log(
            "InteractionLiveStream: Resetting streaming state for interaction",
            interaction?.id,
        );
        setIsActivelyStreaming(true);
    }, [interaction?.id]);

    // Effect to detect completion from the server (WebSocket)
    useEffect(() => {
        if (isComplete && isActivelyStreaming) {
            console.log(
                "InteractionLiveStream: Setting streaming to false - interaction complete",
            );
            setIsActivelyStreaming(false);
        }
    }, [isComplete, isActivelyStreaming]);

    useEffect(() => {
        if (!message) return;
        if (!onMessageChange) return;
        onMessageChange(message);
    }, [message, onMessageChange]);

    useEffect(() => {
        if (!message || !onMessageUpdate) return;
        onMessageUpdate();
    }, [message, onMessageUpdate]);

    if (!serverConfig || !serverConfig.filestore_prefix) {
        console.log(
            "InteractionLiveStream: Returning null - missing serverConfig or filestore_prefix",
        );
        return null;
    }

    console.log("InteractionLiveStream: Rendering with:", {
        showLoading,
        hasMessage: !!message,
        stepInfosCount: stepInfos.length,
        isActivelyStreaming,
    });

    return (
        <>
            {stepInfos.length > 0 && (
                <ToolStepsWidget
                    steps={toolSteps}
                    isLiveStreaming={isActivelyStreaming}
                />
            )}

            {/* Show clear text when in waiting state and no message yet */}
            {interaction.state === "waiting" && !message && <ThinkingBox />}

            {message && (
                <div>
                    <Markdown
                        text={message}
                        session={session}
                        getFileURL={useClientURL}
                        showBlinker={true}
                        isStreaming={isActivelyStreaming} // Now reactive to completion state
                        onFilterDocument={onFilterDocument}
                    />
                </div>
            )}

            {interaction.state === "waiting" && isStale && (
                <WaitingInQueue hasSubscription={false} />
            )}
        </>
    );
};

const ThinkingBox: FC = () => {
    const theme = useTheme();

    return (
        <Box
            sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                padding: "16px",
                margin: "8px 0",
                gap: "4px",
                "@keyframes bounce": {
                    "0%, 80%, 100%": {
                        transform: "scale(0.8)",
                        opacity: 0.5,
                    },
                    "40%": {
                        transform: "scale(1)",
                        opacity: 1,
                    },
                },
            }}
        >
            <Box
                sx={{
                    width: "8px",
                    height: "8px",
                    borderRadius: "50%",
                    backgroundColor:
                        theme.palette.mode === "light" ? "#666" : "#999",
                    animation: "bounce 1.4s ease-in-out infinite",
                    animationDelay: "0s",
                }}
            />
            <Box
                sx={{
                    width: "8px",
                    height: "8px",
                    borderRadius: "50%",
                    backgroundColor:
                        theme.palette.mode === "light" ? "#666" : "#999",
                    animation: "bounce 1.4s ease-in-out infinite",
                    animationDelay: "0.2s",
                }}
            />
            <Box
                sx={{
                    width: "8px",
                    height: "8px",
                    borderRadius: "50%",
                    backgroundColor:
                        theme.palette.mode === "light" ? "#666" : "#999",
                    animation: "bounce 1.4s ease-in-out infinite",
                    animationDelay: "0.4s",
                }}
            />
        </Box>
    );
};

export default React.memo(InteractionLiveStream);
