import React, { FC, useMemo } from "react";
import InteractionContainer from "./InteractionContainer";
import InteractionInference from "./InteractionInference";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import EditIcon from "@mui/icons-material/Edit";
import CopyButtonWithCheck from "./CopyButtonWithCheck";

import useAccount from "../../hooks/useAccount";

import { IServerConfig } from "../../types";

import {
    TypesSession,
    TypesInteraction,
    TypesInteractionState,
} from "../../api/api";

// Prop comparison function for React.memo
const areEqual = (prevProps: InteractionProps, nextProps: InteractionProps) => {
    // Compare serverConfig
    if (
        prevProps.serverConfig?.filestore_prefix !==
        nextProps.serverConfig?.filestore_prefix
    ) {
        return false;
    }

    // Compare interaction
    if (
        prevProps.interaction?.id !== nextProps.interaction?.id ||
        prevProps.interaction?.prompt_message !==
            nextProps.interaction?.prompt_message ||
        prevProps.interaction?.prompt_message_content !==
            nextProps.interaction?.prompt_message_content ||
        prevProps.interaction?.display_message !==
            nextProps.interaction?.display_message ||
        prevProps.interaction?.response_message !==
            nextProps.interaction?.response_message ||
        prevProps.interaction?.error !== nextProps.interaction?.error ||
        prevProps.interaction?.state !== nextProps.interaction?.state
    ) {
        return false;
    }

    // Compare session
    if (
        prevProps.session?.id !== nextProps.session?.id ||
        prevProps.session?.type !== nextProps.session?.type ||
        prevProps.session?.mode !== nextProps.session?.mode
    ) {
        return false;
    }

    // Compare other props
    if (prevProps.highlightAllFiles !== nextProps.highlightAllFiles) {
        return false;
    }

    // Compare function references
    if (
        prevProps.onReloadSession !== nextProps.onReloadSession ||
        prevProps.onAddDocuments !== nextProps.onAddDocuments ||
        prevProps.onRegenerate !== nextProps.onRegenerate ||
        prevProps.onFilterDocument !== nextProps.onFilterDocument
    ) {
        return false;
    }

    return true;
};

interface InteractionProps {
    serverConfig: IServerConfig;
    interaction: TypesInteraction;
    session: TypesSession;
    highlightAllFiles: boolean;
    onReloadSession: () => Promise<any>;
    onAddDocuments?: () => void;
    onFilterDocument?: (docId: string) => void;
    headerButtons?: React.ReactNode;
    children?: React.ReactNode;
    isLastInteraction: boolean;
    isOwner: boolean;
    isAdmin: boolean;
    scrollToBottom?: () => void;
    appID?: string | null;
    onHandleFilterDocument?: (docId: string) => void;
    session_id: string;
    onRegenerate?: (interactionID: string, message: string) => void;
    sessionSteps?: any[];
}

export const Interaction: FC<InteractionProps> = ({
    serverConfig,
    interaction,
    session,
    onFilterDocument,
    headerButtons,
    children,
    isLastInteraction,
    onRegenerate,
    sessionSteps = [],
}) => {    
    // Memoize computed values
    const displayData = useMemo(() => {
        let userMessage: string = "";
        let assistantMessage: string = "";
        let imageURLs: string[] = [];
        let isLoading =
            interaction.state == TypesInteractionState.InteractionStateWaiting;

        // Removed excessive debug logging

        // Extract user message from prompt_message, display_message, or prompt_message_content.parts
        if (interaction?.prompt_message) {
            userMessage = interaction.prompt_message
            console.log('xxxx user msg', userMessage)
        } else if (interaction?.prompt_message_content?.parts?.length) {
            const textPart = interaction.prompt_message_content.parts.find(
                (part): part is { text: string } =>
                    typeof part === "object" &&
                    part !== null &&
                    "text" in part &&
                    typeof part.text === "string"
            );
            if (textPart) {
                userMessage = interaction.display_message || textPart.text;
                console.log('xxxx', userMessage)
            }
        }

        // Extract assistant response from response_message
        if (interaction?.response_message) {
            assistantMessage = interaction.response_message;
        }

        // Check for images in content
        if (interaction?.prompt_message_content?.parts) {
            interaction.prompt_message_content.parts.forEach((part) => {
                if (
                    typeof part === "object" &&
                    part !== null &&
                    "type" in part &&
                    part.type === "image_url" &&
                    "image_url" in part &&
                    part.image_url?.url
                ) {
                    imageURLs.push(part.image_url.url);
                }
            });
        }

        return {
            userMessage,
            assistantMessage,
            imageURLs,
            isLoading,
        };
    }, [interaction, session]);

    const { userMessage, assistantMessage, imageURLs, isLoading } = displayData;

    const [isEditing, setIsEditing] = React.useState(false);
    const [editedMessage, setEditedMessage] = React.useState(userMessage || "");
    const [isHovering, setIsHovering] = React.useState(false);

    const isLive =
        interaction.state == TypesInteractionState.InteractionStateWaiting;

    if (!serverConfig || !serverConfig.filestore_prefix) return null;

    const handleEditClick = () => setIsEditing(true);
    const handleCancel = () => {
        setEditedMessage(userMessage || "");
        setIsEditing(false);
    };
    const handleSave = () => {
        if (onRegenerate && editedMessage !== userMessage) {
            onRegenerate(interaction.id || "", editedMessage);
        }
        setIsEditing(false);
    };

    return (
        <Box
            sx={{
                mb: 2,
                display: "flex",
                flexDirection: "column",
                gap: 1,
            }}
            onMouseEnter={() => setIsHovering(true)}
            onMouseLeave={() => setIsHovering(false)}
        >
            {/* User Message Container */}            
            {userMessage && (
                <Box
                    sx={{
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "flex-end",
                    }}
                >
                    <InteractionContainer
                        buttons={headerButtons}
                        background={true}
                        align="right"
                        border={true}
                        isAssistant={false}
                    >
                        <InteractionInference
                            serverConfig={serverConfig}
                            session={session}
                            interaction={interaction}
                            imageURLs={imageURLs}
                            message={userMessage}
                            error={interaction?.error}
                            upgrade={false}
                            isFromAssistant={false}
                            onFilterDocument={onFilterDocument}
                            onRegenerate={onRegenerate}
                            isEditing={isEditing}
                            editedMessage={editedMessage}
                            setEditedMessage={setEditedMessage}
                            handleCancel={handleCancel}
                            handleSave={handleSave}
                            isLastInteraction={isLastInteraction}
                            sessionSteps={sessionSteps}
                        />
                    </InteractionContainer>
                    {/* Edit button floating below and right-aligned, only for user messages, not editing, and message present */}
                    {!isEditing && userMessage && (
                        <Box
                            sx={{
                                width: "100%",
                                display: "flex",
                                justifyContent: "flex-end",
                                mt: 0.5,
                                gap: 0.5,
                                opacity: isHovering ? 1 : 0,
                                pointerEvents: isHovering ? "auto" : "none",
                                transition: "opacity 0.2s ease-in-out",
                            }}
                        >
                            <CopyButtonWithCheck
                                text={userMessage}
                                alwaysVisible={isHovering}
                            />
                            <Tooltip title="Edit">
                                <IconButton
                                    onClick={handleEditClick}
                                    size="small"
                                    sx={(theme) => ({
                                        color:
                                            theme.palette.mode === "light"
                                                ? "#888"
                                                : "#bbb",
                                        "&:hover": {
                                            color:
                                                theme.palette.mode === "light"
                                                    ? "#000"
                                                    : "#fff",
                                        },
                                    })}
                                    aria-label="edit"
                                >
                                    <EditIcon sx={{ fontSize: 20 }} />
                                </IconButton>
                            </Tooltip>
                        </Box>
                    )}
                </Box>
            )}

            {/* Assistant Response Container */}
            {(assistantMessage || isLive) && (
                <Box
                    sx={{
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "flex-start",
                    }}
                >
                    <InteractionContainer
                        buttons={headerButtons}
                        background={false}
                        align="left"
                        border={false}
                        isAssistant={true}
                    >
                        {/* Show live stream if interaction is waiting */}
                        {isLive ? (
                            children
                        ) : (
                            <InteractionInference
                                serverConfig={serverConfig}
                                session={session}
                                interaction={interaction}
                                imageURLs={[]}
                                message={assistantMessage}
                                error={interaction?.error}
                                upgrade={false}
                                isFromAssistant={true}
                                onFilterDocument={onFilterDocument}
                                onRegenerate={onRegenerate}
                                isEditing={false}
                                editedMessage=""
                                setEditedMessage={() => {}}
                                handleCancel={() => {}}
                                handleSave={() => {}}
                                isLastInteraction={isLastInteraction}
                                sessionSteps={sessionSteps}
                            />
                        )}
                    </InteractionContainer>
                </Box>
            )}
        </Box>
    );
};

export default React.memo(Interaction, areEqual);
