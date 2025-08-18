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
import LoadingSpinner from "../widgets/LoadingSpinner";
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
                position: "relative",
                padding: "24px",
                fontSize: "18px",
                fontWeight: 600,
                color: theme.palette.mode === "light" ? "#333" : "#fff",
                background:
                    theme.palette.mode === "light"
                        ? "linear-gradient(135deg, #667eea 0%, #764ba2 100%)"
                        : "linear-gradient(135deg, #1e3c72 0%, #2a5298 100%)",
                borderRadius: "16px",
                margin: "16px 0",
                textAlign: "center",
                boxShadow: "0 8px 32px rgba(0,0,0,0.3)",
                overflow: "hidden",
                "&::before": {
                    content: '""',
                    position: "absolute",
                    top: 0,
                    left: 0,
                    right: 0,
                    bottom: 0,
                    background:
                        "linear-gradient(45deg, #ff0000, #ff7300, #fffb00, #48ff00, #00ffd5, #002bff, #7a00ff, #ff00c8, #ff0000)",
                    backgroundSize: "400% 400%",
                    borderRadius: "16px",
                    padding: "2px",
                    mask: "linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)",
                    maskComposite: "xor",
                    WebkitMask:
                        "linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)",
                    WebkitMaskComposite: "xor",
                    animation: "rainbow-glow 3s ease-in-out infinite",
                },
                "@keyframes rainbow-glow": {
                    "0%, 100%": {
                        backgroundPosition: "0% 50%",
                    },
                    "50%": {
                        backgroundPosition: "100% 50%",
                    },
                },
                "&::after": {
                    content: '""',
                    position: "absolute",
                    top: "50%",
                    left: "50%",
                    width: "200%",
                    height: "200%",
                    background:
                        "conic-gradient(from 0deg, transparent, rgba(255,255,255,0.1), transparent)",
                    transform: "translate(-50%, -50%)",
                    animation: "spin 4s linear infinite",
                    pointerEvents: "none",
                },
                "@keyframes spin": {
                    "0%": {
                        transform: "translate(-50%, -50%) rotate(0deg)",
                    },
                    "100%": {
                        transform: "translate(-50%, -50%) rotate(360deg)",
                    },
                },
                "& .thinking-text": {
                    position: "relative",
                    zIndex: 1,
                    background:
                        "linear-gradient(45deg, #ff6b6b, #4ecdc4, #45b7d1, #96ceb4, #feca57, #ff9ff3, #54a0ff)",
                    backgroundSize: "400% 400%",
                    WebkitBackgroundClip: "text",
                    WebkitTextFillColor: "transparent",
                    backgroundClip: "text",
                    animation: "rainbow-text 2s ease-in-out infinite alternate",
                },
                "@keyframes rainbow-text": {
                    "0%": {
                        backgroundPosition: "0% 50%",
                    },
                    "100%": {
                        backgroundPosition: "100% 50%",
                    },
                },
            }}
        >
            <span className="thinking-text">🤔 Thinking...</span>
        </Box>
    );
};

export default React.memo(InteractionLiveStream);
