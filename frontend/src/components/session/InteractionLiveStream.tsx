import React, {
    FC,
    useEffect,
    useState,
    useMemo,
    useCallback,
    useRef,
    useLayoutEffect,
} from "react";

// Throttle interval for scroll updates during streaming (ms)
const SCROLL_THROTTLE_MS = 200;
import Typography from "@mui/material/Typography";
import Box from "@mui/material/Box";
import { useTheme } from "@mui/material/styles";
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
    const { message, status, stepInfos, isComplete } =
        useLiveInteraction(session_id, interaction);

    // Removed excessive debug logging

    // Add state to track if we're still in streaming mode or completed
    const [isActivelyStreaming, setIsActivelyStreaming] = useState(true);

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
        // Removed excessive debug logging
        setIsActivelyStreaming(true);
    }, [interaction?.id]);

    // Effect to detect completion from the server (WebSocket)
    useEffect(() => {
        if (isComplete && isActivelyStreaming) {
            setIsActivelyStreaming(false);
        }
    }, [isComplete, isActivelyStreaming]);

    useEffect(() => {
        if (!message) return;
        if (!onMessageChange) return;
        onMessageChange(message);
    }, [message, onMessageChange]);

    // Throttle scroll updates during streaming to reduce CPU usage
    const scrollThrottleRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const pendingScrollRef = useRef(false);

    useEffect(() => {
        if (!message || !onMessageUpdate) return;

        // Throttle scroll updates
        if (!scrollThrottleRef.current) {
            onMessageUpdate();
            scrollThrottleRef.current = setTimeout(() => {
                scrollThrottleRef.current = null;
                if (pendingScrollRef.current) {
                    pendingScrollRef.current = false;
                    onMessageUpdate();
                }
            }, SCROLL_THROTTLE_MS);
        } else {
            pendingScrollRef.current = true;
        }
    }, [message, onMessageUpdate]);

    // Cleanup throttle on unmount
    useEffect(() => {
        return () => {
            if (scrollThrottleRef.current) {
                clearTimeout(scrollThrottleRef.current);
            }
        };
    }, []);

    if (!serverConfig || !serverConfig.filestore_prefix) {
        return null;
    }



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
