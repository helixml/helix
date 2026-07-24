import React, { FC, useMemo } from "react";
import InteractionContainer from "./InteractionContainer";
import InteractionInference from "./InteractionInference";
import Box from "@mui/material/Box";
import Alert from "@mui/material/Alert";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import EditIcon from "@mui/icons-material/Edit";
import CopyButtonWithCheck from "./CopyButtonWithCheck";
import InteractionDebugCopyButton from "./InteractionDebugCopyButton";
import CollapsibleSystemPrefix, {
  splitSystemPrefix,
} from "./CollapsibleSystemPrefix";

import useAccount from "../../hooks/useAccount";

import { TypesServerConfigForFrontend } from "../../api/api";

import {
  TypesSession,
  TypesInteraction,
  TypesInteractionState,
} from "../../api/api";

/**
 * Inline divider rendered in place of a normal user/assistant turn for
 * synthetic fork_seed interactions. The seed's prompt_message is a
 * human-readable summary ("Session forked from ses_X at turn N");
 * response_message holds the parent's serialized transcript, hidden
 * behind a disclosure for users who want to verify what was sent.
 */
const ForkSeedDivider: FC<{ interaction: TypesInteraction }> = ({
  interaction,
}) => {
  const [expanded, setExpanded] = React.useState(false);
  const transcript = interaction.response_message || "";
  return (
    <Box sx={{ my: 3 }}>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1,
          color: "text.secondary",
        }}
      >
        <Box sx={{ flex: 1, borderTop: "1px dashed", borderColor: "divider" }} />
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            gap: 0.25,
            px: 1,
          }}
        >
          <Box
            sx={{
              fontSize: "0.75rem",
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: 0.5,
            }}
          >
            {interaction.prompt_message || "Forked from prior session"}
          </Box>
          {transcript && (
            <Box
              component="button"
              type="button"
              onClick={() => setExpanded((v) => !v)}
              sx={{
                background: "transparent",
                border: "none",
                color: "primary.main",
                fontSize: "0.7rem",
                cursor: "pointer",
                p: 0,
                "&:hover": { textDecoration: "underline" },
              }}
            >
              {expanded ? "Hide transcript" : `Show transcript (${transcript.length.toLocaleString()} chars)`}
            </Box>
          )}
        </Box>
        <Box sx={{ flex: 1, borderTop: "1px dashed", borderColor: "divider" }} />
      </Box>
      {expanded && transcript && (
        <Box
          sx={{
            mt: 1,
            p: 1.5,
            border: "1px dashed",
            borderColor: "divider",
            borderRadius: 1,
            backgroundColor: "action.hover",
            fontSize: "0.75rem",
            fontFamily: "monospace",
            whiteSpace: "pre-wrap",
            wordBreak: "break-word",
            maxHeight: 400,
            overflowY: "auto",
          }}
        >
          {transcript}
        </Box>
      )}
    </Box>
  );
};

// Prop comparison function for React.memo
const areEqual = (prevProps: InteractionProps, nextProps: InteractionProps) => {
  if (prevProps.enableDebugCopy !== nextProps.enableDebugCopy) {
    return false;
  }

  // Debug-enabled surfaces must keep the raw objects current because the
  // copied bundle includes fields the transcript itself does not render
  // (usage, runner, structured tool calls, model and routing metadata).
  if (
    nextProps.enableDebugCopy &&
    (prevProps.interaction !== nextProps.interaction ||
      prevProps.session !== nextProps.session ||
      prevProps.sessionSteps !== nextProps.sessionSteps)
  ) {
    return false;
  }

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
  serverConfig: TypesServerConfigForFrontend;
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
  enableDebugCopy?: boolean;
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
  enableDebugCopy = false,
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
      userMessage = interaction.prompt_message;
    } else if (interaction?.prompt_message_content?.parts?.length) {
      const textPart = interaction.prompt_message_content.parts.find(
        (part): part is { text: string } =>
          typeof part === "object" &&
          part !== null &&
          "text" in part &&
          typeof part.text === "string",
      );
      if (textPart) {
        userMessage = interaction.display_message || textPart.text;
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

    const split = splitSystemPrefix(userMessage);

    return {
      userMessage,
      assistantMessage,
      imageURLs,
      isLoading,
      systemPrefix: split.prefix,
      userMessageBody: split.userText,
      systemPrefixLabel: split.label,
      systemPrefixKind: split.kind,
    };
  }, [interaction, session]);

  const {
    userMessage,
    assistantMessage,
    imageURLs,
    isLoading,
    systemPrefix,
    userMessageBody,
    systemPrefixLabel,
    systemPrefixKind,
  } = displayData;

  // When the whole message is system content (no user body), the user
  // bubble has nothing to show. The CollapsibleSystemPrefix carries the
  // entire message and replaces the bubble.
  const isPureSystemMessage = !!systemPrefix && userMessageBody.length === 0;

  const [isEditing, setIsEditing] = React.useState(false);
  const [editedMessage, setEditedMessage] = React.useState(userMessage || "");
  const [isHovering, setIsHovering] = React.useState(false);

  const isLive =
    interaction.state == TypesInteractionState.InteractionStateWaiting;
  const hasAgentReply =
    !!assistantMessage ||
    ((interaction as any)?.response_entries?.length ?? 0) > 0;

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

  // Synthetic fork_seed interactions are the UI-visible marker that this
  // session was created by forking another. Render as a centred divider
  // with an expandable disclosure for the raw seeded transcript instead
  // of as a normal user/assistant turn (the agent never sees the
  // prompt_message — it's a placeholder; the actual seed payload lives
  // in response_message and is injected via maybePrependTranscript).
  if (interaction.trigger === "fork_seed") {
    return <ForkSeedDivider interaction={interaction} />;
  }

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
          {systemPrefix && !isEditing && (
            <CollapsibleSystemPrefix
              prefix={systemPrefix}
              label={
                systemPrefixKind === "approval"
                  ? "Spec Approved — Implementation Instructions"
                  : systemPrefixLabel?.startsWith("Original Request")
                    ? "Planning Instructions (cloned task)"
                    : "Planning Instructions"
              }
            />
          )}
          {/*
            "Retried Nx" badge for prompts the auto-wake worker has had
            to re-send to unstick the session. Auto-wakes don't create
            separate interactions any more — they re-send the original
            prompt's content over the wire and bump auto_wake_count on
            this row, so the badge counts the retries on the original
            user message. See
            design/2026-04-25-zed-claude-async-event-flush-on-user-input.md
            and the file header of api/pkg/server/auto_wake_stuck_interactions.go
          */}
          {!isPureSystemMessage && (
            <>
              {((interaction as any)?.auto_wake_count ?? 0) > 0 && (
                <Tooltip
                  title="Helix re-sent this prompt because the agent didn't respond — likely upstream ACP buffering (claude-agent-acp #551 / agent-client-protocol #554). See the helix-side design doc 2026-04-25 for the full story."
                >
                  <Box
                    sx={(theme) => ({
                      fontSize: "11px",
                      color: theme.palette.mode === "light" ? "#888" : "#999",
                      mb: 0.5,
                      px: 1,
                      py: 0.25,
                      borderRadius: "4px",
                      backgroundColor:
                        theme.palette.mode === "light"
                          ? "rgba(0,0,0,0.04)"
                          : "rgba(255,255,255,0.06)",
                      cursor: "help",
                      userSelect: "none",
                    })}
                  >
                    {"↻ Retried " + ((interaction as any).auto_wake_count) + "× · upstream ACP buffering"}
                  </Box>
                </Tooltip>
              )}
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
                  message={systemPrefix && !isEditing ? userMessageBody : userMessage}
                  error={interaction?.error}
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
                  enableDebugCopy={false}
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
                  {enableDebugCopy && !hasAgentReply && (
                    <InteractionDebugCopyButton
                      interaction={interaction}
                      session={session}
                      sessionSteps={sessionSteps}
                      serverConfig={serverConfig}
                    />
                  )}
                  <CopyButtonWithCheck
                    text={systemPrefix ? userMessageBody : userMessage}
                    alwaysVisible={isHovering}
                  />
                  <Tooltip title="Edit">
                    <IconButton
                      onClick={handleEditClick}
                      size="small"
                      sx={(theme) => ({
                        color: theme.palette.mode === "light" ? "#888" : "#bbb",
                        "&:hover": {
                          color: theme.palette.mode === "light" ? "#000" : "#fff",
                        },
                      })}
                      aria-label="edit"
                    >
                      <EditIcon sx={{ fontSize: 20 }} />
                    </IconButton>
                  </Tooltip>
                </Box>
              )}
            </>
          )}
        </Box>
      )}

      {/* Assistant Response Container */}
      {/*
        Also mount the assistant bubble when the interaction has an error, even
        with no response content. The error alert + Retry button live on the
        assistant-side InteractionInference; without this an agent that fails
        before producing any output (common in the spec task / ACP view) would
        render no assistant bubble at all, so the Retry button never appears.
      */}
      {(assistantMessage || (interaction as any)?.response_entries?.length > 0 || isLive || interaction.error) && (
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
            {/* Show live stream if interaction is waiting AND has children (last interaction) */}
            {isLive && children ? (
              children
            ) : (
              <>
                <InteractionInference
                  serverConfig={serverConfig}
                  session={session}
                  interaction={interaction}
                  imageURLs={[]}
                  message={assistantMessage}
                  error={interaction?.error}
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
                  enableDebugCopy={enableDebugCopy}
                />
                {/* Show incomplete warning for waiting interactions that aren't actively streaming */}
                {isLive && !children && !isLastInteraction && (
                  <Alert
                    severity="warning"
                    icon={false}
                    sx={{
                      mt: 1,
                      py: 0.25,
                      px: 1.5,
                      fontSize: "0.75rem",
                      "& .MuiAlert-message": {
                        padding: "2px 0",
                      },
                    }}
                  >
                    ⚠ Incomplete interaction — the agent may have disconnected
                    before finishing
                  </Alert>
                )}
                {interaction.state === TypesInteractionState.InteractionStateInterrupted && (
                  <Alert
                    severity="info"
                    icon={false}
                    sx={{
                      mt: 1,
                      py: 0.25,
                      px: 1.5,
                      fontSize: "0.75rem",
                      "& .MuiAlert-message": {
                        padding: "2px 0",
                      },
                    }}
                  >
                    Interrupted
                  </Alert>
                )}
              </>
            )}
          </InteractionContainer>
        </Box>
      )}
    </Box>
  );
};

export default React.memo(Interaction, areEqual);
