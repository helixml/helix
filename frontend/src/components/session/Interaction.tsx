import React, { FC, useMemo } from "react";
import InteractionContainer from "./InteractionContainer";
import InteractionInference from "./InteractionInference";
import Box from "@mui/material/Box";
import Stack from "@mui/material/Stack";
import Alert from "@mui/material/Alert";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import EditIcon from "@mui/icons-material/Edit";
import CopyButtonWithCheck from "./CopyButtonWithCheck";
import InteractionDebugCopyButton from "./InteractionDebugCopyButton";
import CollapsibleSystemPrefix, {
  splitSystemPrefix,
} from "./CollapsibleSystemPrefix";
import ChangedFilesCard from "./ChangedFilesCard";
import { parseMessageWithAttachments } from "../common/chatAttachments";
import { resolveChatTurnAssistantPreview } from "./ChatTurnNavigator.logic";
import { workspaceReviewMessageCopyText } from "./workspaceReviewMessage";

import useAccount from "../../hooks/useAccount";

import { TypesServerConfigForFrontend } from "../../api/api";

import {
  TypesSession,
  TypesInteraction,
  TypesInteractionState,
} from "../../api/api";

const getInteractionUserMessage = (interaction?: TypesInteraction) => {
  if (interaction?.prompt_message) return interaction.prompt_message;

  const textPart = interaction?.prompt_message_content?.parts?.find(
    (part): part is { text: string } =>
      typeof part === "object" &&
      part !== null &&
      "text" in part &&
      typeof part.text === "string",
  );

  return textPart ? interaction?.display_message || textPart.text : "";
};

/**
 * Inline divider rendered in place of a normal user/assistant turn for
 * synthetic fork_seed interactions. The disclosure keeps both the seeded
 * transcript and its associated synthetic handoff out of the normal chat.
 */
const ForkSeedDivider: FC<{
  interaction: TypesInteraction;
  handoffInteraction?: TypesInteraction;
}> = ({
  interaction,
  handoffInteraction,
}) => {
  const [expanded, setExpanded] = React.useState(false);
  const transcript = interaction.response_message || "";
  const isAgentSwitch = interaction.prompt_message?.startsWith("Agent switched to ")
    && handoffInteraction?.trigger === "fork_handoff";
  const dividerLabel = interaction.prompt_message || "Forked from prior session";
  const handoffResponse = handoffInteraction
    ? resolveChatTurnAssistantPreview(
        handoffInteraction.response_message,
        (handoffInteraction as any).response_entries,
      )
    : null;
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
            component={isAgentSwitch ? "button" : "div"}
            type={isAgentSwitch ? "button" : undefined}
            aria-expanded={isAgentSwitch ? expanded : undefined}
            onClick={isAgentSwitch ? () => setExpanded((value) => !value) : undefined}
            sx={{
              background: "transparent",
              border: "none",
              color: "inherit",
              fontSize: "0.75rem",
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: 0.5,
              p: 0,
              cursor: isAgentSwitch ? "pointer" : "default",
              "&:hover": isAgentSwitch ? { color: "text.primary" } : undefined,
            }}
          >
            {dividerLabel}
          </Box>
          {!isAgentSwitch && transcript && (
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
      {expanded && handoffInteraction && (
        <Stack spacing={1} sx={{ mt: 1 }}>
          <Box
            sx={{
              p: 1.5,
              borderRadius: 1,
              backgroundColor: "action.hover",
              fontSize: "0.8rem",
              whiteSpace: "pre-wrap",
              wordBreak: "break-word",
            }}
          >
            {handoffInteraction.prompt_message}
          </Box>
          <Box
            sx={{
              p: 1.5,
              border: "1px solid",
              borderColor: "divider",
              borderRadius: 1,
              fontSize: "0.8rem",
              whiteSpace: "pre-wrap",
              wordBreak: "break-word",
            }}
          >
            {handoffResponse || "Waiting for the agent response…"}
          </Box>
        </Stack>
      )}
      {!isAgentSwitch && expanded && transcript && (
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

  if (
    prevProps.nextInteraction?.id !== nextProps.nextInteraction?.id ||
    prevProps.nextInteraction?.state !== nextProps.nextInteraction?.state ||
    prevProps.nextInteraction?.error !== nextProps.nextInteraction?.error ||
    getInteractionUserMessage(prevProps.nextInteraction) !==
      getInteractionUserMessage(nextProps.nextInteraction)
  ) {
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
    prevProps.interaction?.completed !== nextProps.interaction?.completed ||
    prevProps.interaction?.error !== nextProps.interaction?.error ||
    prevProps.interaction?.state !== nextProps.interaction?.state ||
    prevProps.interaction?.code_changes?.status !==
      nextProps.interaction?.code_changes?.status ||
    prevProps.interaction?.code_changes?.patch_hash !==
      nextProps.interaction?.code_changes?.patch_hash
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
  if (
    prevProps.highlightAllFiles !== nextProps.highlightAllFiles ||
    prevProps.recoveredLater !== nextProps.recoveredLater
  ) {
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
  nextInteraction?: TypesInteraction;
  /**
   * A later interaction in this session completed successfully, so any error on
   * THIS one has been overtaken by events. See lastSuccessfulInteractionIndex.
   */
  recoveredLater?: boolean;
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
  nextInteraction,
  recoveredLater = false,
}) => {
  // Memoize computed values
  const displayData = useMemo(() => {
    const userMessage = getInteractionUserMessage(interaction);
    let assistantMessage: string = "";
    let imageURLs: string[] = [];
    let isLoading =
      interaction.state == TypesInteractionState.InteractionStateWaiting;

    // Removed excessive debug logging

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

    const parsedUserMessage = parseMessageWithAttachments(userMessage);
    const split = splitSystemPrefix(parsedUserMessage.message);

    return {
      userMessage,
      assistantMessage,
      imageURLs,
      workspaceAttachments: parsedUserMessage.attachments,
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
  const workspaceReviewCopyText = workspaceReviewMessageCopyText(userMessageBody);

  const [isEditing, setIsEditing] = React.useState(false);
  const [editedMessage, setEditedMessage] = React.useState(userMessage || "");
  const [isHovering, setIsHovering] = React.useState(false);

  const isLive =
    interaction.state == TypesInteractionState.InteractionStateWaiting;
  const hasAgentReply =
    !!assistantMessage ||
    ((interaction as any)?.response_entries?.length ?? 0) > 0;
  const nextInteractionPrompt = getInteractionUserMessage(nextInteraction);
  const retrySucceeded =
    !!interaction.error &&
    nextInteraction?.state ===
      TypesInteractionState.InteractionStateComplete &&
    !nextInteraction.error &&
    nextInteractionPrompt === userMessage;
  // A retry of the SAME prompt succeeded, so the work got done and the failed
  // attempt is moot — drop it entirely.
  const visibleError = retrySucceeded ? undefined : interaction.error;
  // A LATER, different turn succeeded. The session recovered, but this turn's
  // work really was abandoned, so the error still belongs on screen — as
  // history, not as an actionable alarm. No red alert, no Retry button.
  const errorIsHistorical = !retrySucceeded && !!interaction.error && recoveredLater;

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
    return (
      <ForkSeedDivider
        interaction={interaction}
        handoffInteraction={nextInteraction}
      />
    );
  }
  if (interaction.trigger === "fork_handoff") {
    return null;
  }

  return (
    <Box
      data-chat-turn={interaction.id}
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
              tone={systemPrefixKind === "approval" ? "success" : "neutral"}
              label={
                systemPrefixKind === "approval"
                  ? "Spec approved · Implementation instructions"
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
                messageRole="user"
              >
                <InteractionInference
                  serverConfig={serverConfig}
                  session={session}
                  interaction={interaction}
                  imageURLs={imageURLs}
                  workspaceAttachments={displayData.workspaceAttachments}
                  message={isEditing ? userMessage : userMessageBody}
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
                    maxWidth: "80%",
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
                    text={
                      workspaceReviewCopyText ||
                      (systemPrefix ? userMessageBody : userMessage)
                    }
                    alwaysVisible={isHovering}
                  />
                  {!workspaceReviewCopyText && (
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
                  )}
                </Box>
              )}
            </>
          )}
        </Box>
      )}

      {/* Assistant Response Container */}
      {(assistantMessage ||
        (interaction as any)?.response_entries?.length > 0 ||
        isLive ||
        visibleError) && (
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
            messageRole="assistant"
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
                  error={visibleError}
                  errorIsHistorical={errorIsHistorical}
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
                <ChangedFilesCard
                  interaction={interaction}
                  isLatest={isLastInteraction}
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
