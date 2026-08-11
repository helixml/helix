import React, { FC, useState, useEffect, useMemo } from "react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import ReplayIcon from "@mui/icons-material/Replay";
import TerminalWindow from "../widgets/TerminalWindow";
import ClickLink from "../widgets/ClickLink";
import Row from "../widgets/Row";
import Cell from "../widgets/Cell";
import Markdown from "./Markdown";
import WorkLog from "./WorkLog";
import ActivitySummary from "./ActivitySummary";
import { SessionPlanProgress } from "./PlanProgress";
import { getInteractionDurationMs } from "./interactionDuration";
import ImageLightbox, { LightboxImage } from "./ImageLightbox";
import ElicitationCardContainer from "./ElicitationCardContainer";
import type { ElicitationPayload } from "./ElicitationCard";

/**
 * A structured response entry from the Go API.
 * Preserves the type and ordering of each entry as Zed originally had them.
 */
export interface ResponseEntry {
  type: "text" | "tool_call" | "plan" | "elicitation";
  content: string;
  message_id: string;
  tool_name?: string;
  tool_status?: string;
  /** Present for type === "elicitation": a question the agent asked the user. */
  elicitation?: ElicitationPayload;
}

interface TextActivitySegment {
  type: "text";
  entry: ResponseEntry;
  index: number;
  renderThinking: boolean;
  renderContent: boolean;
}

interface ToolActivitySegment {
  type: "tools";
  index: number;
  entries: Array<{
    id: string;
    toolName: string;
    status: string;
    body: string;
  }>;
}

interface ElicitationActivitySegment {
  type: "elicitation";
  entry: ResponseEntry;
  index: number;
}

type ActivitySegment =
  | TextActivitySegment
  | ToolActivitySegment
  | ElicitationActivitySegment;

const hasThinking = (content: string) => /<(?:think|thinking)>/i.test(content);

const hasVisibleAssistantText = (content: string) => content
  .replace(/<think>[\s\S]*?<\/think>/gi, "")
  .replace(/<thinking>[\s\S]*?<\/thinking>/gi, "")
  .replace(/<(?:think|thinking)>[\s\S]*$/gi, "")
  .trim()
  .length > 0;

const toolActivityEntry = (
  entry: ResponseEntry,
  index: number,
  responseEntryCount: number,
  isStreaming: boolean,
) => ({
  id: `${entry.message_id || "tool-call"}-${index}`,
  toolName: entry.tool_name || "Tool Call",
  status:
    entry.tool_status ||
    (index === responseEntryCount - 1 && isStreaming ? "Running" : "Completed"),
  body: entry.content || "",
});

export function buildActivityTimeline(
  responseEntries: ResponseEntry[],
  isStreaming: boolean,
): { activitySegments: ActivitySegment[]; finalTextIndex: number | undefined } {
  let finalTextIndex: number | undefined;
  if (!isStreaming) {
    for (let index = responseEntries.length - 1; index >= 0; index -= 1) {
      if (
        responseEntries[index].type === "text" &&
        hasVisibleAssistantText(responseEntries[index].content)
      ) {
        finalTextIndex = index;
        break;
      }
    }
  }

  const activitySegments: ActivitySegment[] = [];
  let currentToolSegment: ToolActivitySegment | undefined;

  responseEntries.forEach((entry, index) => {
    // A question the agent asked is never folded into a collapsed tool run — the
    // whole point is that the user sees it and can answer it.
    if (entry.type === "elicitation") {
      currentToolSegment = undefined;
      activitySegments.push({ type: "elicitation", entry, index });
      return;
    }

    if (entry.type === "tool_call") {
      const toolEntry = toolActivityEntry(
        entry,
        index,
        responseEntries.length,
        isStreaming,
      );

      if (currentToolSegment) {
        currentToolSegment.entries.push(toolEntry);
        currentToolSegment.index = index;
      } else {
        currentToolSegment = { type: "tools", index, entries: [toolEntry] };
        activitySegments.push(currentToolSegment);
      }
      return;
    }

    if (entry.type === "plan") {
      currentToolSegment = undefined;
      return;
    }

    // Any text entry ends the current tool run, even when its thinking content
    // is hidden while streaming.
    currentToolSegment = undefined;

    if (index === finalTextIndex) {
      if (hasThinking(entry.content)) {
        activitySegments.push({
          type: "text",
          entry,
          index,
          renderThinking: true,
          renderContent: false,
        });
      }
      return;
    }

    const renderThinking = !isStreaming && hasThinking(entry.content);
    const renderContent = hasVisibleAssistantText(entry.content);
    if (renderThinking || renderContent) {
      activitySegments.push({
        type: "text",
        entry,
        index,
        renderThinking,
        renderContent,
      });
    }
  });

  return {
    activitySegments,
    finalTextIndex,
  };
}
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import RefreshIcon from "@mui/icons-material/Refresh";
import TextField from "@mui/material/TextField";
import CopyButtonWithCheck from "./CopyButtonWithCheck";
import InteractionDebugCopyButton from "./InteractionDebugCopyButton";
import MessageReceivedTimestamp from "./MessageReceivedTimestamp";
import ToolStepsWidget from "./ToolStepsWidget";

import { ThumbsUp, ThumbsDown, Download, FileText, Paperclip } from "lucide-react";

import ExportDocument from "../export/ExportDocument";
import ToPDF from "../export/ToPDF";

import useAccount from "../../hooks/useAccount";
import useRouter from "../../hooks/useRouter";
import { useUpdateInteractionFeedback } from "../../services/interactionsService";


import { TypesServerConfigForFrontend } from "../../api/api";

import {
  TypesFeedback,
  TypesInteraction,
  TypesInteractionState,
  TypesSession,
} from "../../api/api";
import {
  ChatWorkspaceAttachment,
  workspaceAttachmentURL,
} from "../common/chatAttachments";

/**
 * Renders a message that may contain tool call blocks.
 *
 * If structured `responseEntries` are provided (from the Go API's ResponseEntries
 * field), renders each entry with the correct component in the correct order.
 * Otherwise falls back to regex parsing of the flat text (for old interactions).
 */
export const MessageWithToolCalls: FC<{
  text: string;
  responseEntries?: ResponseEntry[];
  session: TypesSession;
  getFileURL: (url: string) => string;
  showBlinker: boolean;
  isStreaming: boolean;
  onFilterDocument?: (docId: string) => void;
  compactThinking?: boolean;
  durationMs?: number;
  activityStartedAt?: number;
  showActivitySummary?: boolean;
  includeTaskChecklist?: boolean;
}> = ({
  text,
  responseEntries,
  session,
  getFileURL,
  showBlinker,
  isStreaming,
  onFilterDocument,
  compactThinking = false,
  durationMs = 0,
  activityStartedAt,
  showActivitySummary = true,
  includeTaskChecklist = false,
}) => {
  if (!showActivitySummary) {
    return (
      <Markdown
        text={text}
        session={session}
        getFileURL={getFileURL}
        showBlinker={showBlinker}
        isStreaming={isStreaming}
        onFilterDocument={onFilterDocument}
        compactThinking={compactThinking}
      />
    );
  }

  const planProgress = (
    <SessionPlanProgress
      responseEntries={responseEntries}
      session={session}
      includeTaskChecklist={includeTaskChecklist}
    />
  );

  // Structured path: use response_entries from the Go API (preserves type + order)
  if (responseEntries && responseEntries.length > 0) {
    const { activitySegments, finalTextIndex } = buildActivityTimeline(
      responseEntries,
      isStreaming,
    );
    // Questions render outside the collapsible activity summary. Burying a blocked
    // turn's question inside a collapsed "activity" section would defeat the feature.
    const questionSegments = activitySegments.filter(
      (segment): segment is ElicitationActivitySegment => segment.type === "elicitation",
    );
    const timelineSegments = activitySegments.filter(
      (segment) => segment.type !== "elicitation",
    );
    const hasActivity = timelineSegments.length > 0;
    const questions = questionSegments.map((segment) =>
      segment.entry.elicitation ? (
        <ElicitationCardContainer
          key={`elicitation-${segment.entry.elicitation.id}`}
          sessionId={session.id || ""}
          elicitation={segment.entry.elicitation}
        />
      ) : null,
    );
    const activity = timelineSegments.map((segment, segmentIndex) => {
      if (segment.type === "tools") {
        return <WorkLog key={`tool-run-${segmentIndex}`} entries={segment.entries} />;
      }
      if (segment.type === "elicitation") {
        return null;
      }

      return (
        <Markdown
          key={`activity-text-${segment.index}`}
          text={segment.entry.content}
          session={session}
          getFileURL={getFileURL}
          showBlinker={false}
          isStreaming={isStreaming && segment.index === responseEntries.length - 1}
          onFilterDocument={onFilterDocument}
          compactThinking={compactThinking}
          renderThinkingWidget={segment.renderThinking}
          renderContent={segment.renderContent}
        />
      );
    });
    const finalEntry = finalTextIndex === undefined ? undefined : responseEntries[finalTextIndex];
    const finalContent = finalEntry ? (
      <Markdown
        text={finalEntry.content}
        session={session}
        getFileURL={getFileURL}
        showBlinker={false}
        isStreaming={false}
        onFilterDocument={onFilterDocument}
        compactThinking={compactThinking}
        renderThinkingWidget={false}
      />
    ) : null;

    return isStreaming ? (
      <>
        {activity}
        {planProgress}
        <ActivitySummary
          durationMs={durationMs}
          hasActivity={false}
          isStreaming
          startedAt={activityStartedAt}
        />
        {questions}
      </>
    ) : (
      <>
        <ActivitySummary
          durationMs={durationMs}
          hasActivity={hasActivity}
          isStreaming={false}
          startedAt={activityStartedAt}
        >
          {activity}
        </ActivitySummary>
        {questions}
        {planProgress}
        {finalContent}
      </>
    );
  }

  // Plain markdown for text-only interactions
  const plainHasThinking = hasThinking(text);
  const finalContent = (
    <Markdown
      text={text}
      session={session}
      getFileURL={getFileURL}
      showBlinker={false}
      isStreaming={isStreaming}
      onFilterDocument={onFilterDocument}
      compactThinking={compactThinking}
      renderThinkingWidget={false}
    />
  );
  const activityContent = plainHasThinking ? (
    <Markdown
      text={text}
      session={session}
      getFileURL={getFileURL}
      showBlinker={false}
      isStreaming={false}
      onFilterDocument={onFilterDocument}
      compactThinking={compactThinking}
      renderThinkingWidget
      renderContent={false}
    />
  ) : null;

  return isStreaming ? (
    <>
      {finalContent}
      {planProgress}
      <ActivitySummary
        durationMs={durationMs}
        hasActivity={plainHasThinking}
        isStreaming
        startedAt={activityStartedAt}
      >
        {activityContent}
      </ActivitySummary>
    </>
  ) : (
    <>
      {finalContent}
      <ActivitySummary
        durationMs={durationMs}
        hasActivity={plainHasThinking}
        isStreaming={false}
        startedAt={activityStartedAt}
      >
        {activityContent}
      </ActivitySummary>
      {planProgress}
    </>
  );
};

export const InteractionInference: FC<{
  imageURLs?: string[];
  workspaceAttachments?: ChatWorkspaceAttachment[];
  message?: string;
  error?: string;
  serverConfig?: TypesServerConfigForFrontend;
  interaction: TypesInteraction;
  session: TypesSession;
  isFromAssistant?: boolean;
  onFilterDocument?: (docId: string) => void;
  onRegenerate?: (interactionID: string, message: string) => void;
  isEditing?: boolean;
  editedMessage?: string;
  setEditedMessage?: (msg: string) => void;
  handleCancel?: () => void;
  handleSave?: () => void;
  isLastInteraction?: boolean;
  sessionSteps?: any[];
  enableDebugCopy?: boolean;
}> = ({
  imageURLs = [],
  workspaceAttachments = [],
  message,
  error,
  serverConfig,
  interaction,
  session,
  isFromAssistant: isFromAssistant,
  onFilterDocument,
  onRegenerate,
  isEditing: externalIsEditing,
  editedMessage: externalEditedMessage,
  setEditedMessage: externalSetEditedMessage,
  handleCancel: externalHandleCancel,
  handleSave: externalHandleSave,
  isLastInteraction,
  sessionSteps = [],
  enableDebugCopy = false,
}) => {
  const account = useAccount();
  const router = useRouter();
  const [viewingError, setViewingError] = useState(false);
  const [viewingExport, setViewingExport] = useState(false);
  const [selectedImageIndex, setSelectedImageIndex] = useState<number | null>(null);
  const [userMessageExpanded, setUserMessageExpanded] = useState(false);
  const [internalIsEditing, setInternalIsEditing] = useState(false);
  const [internalEditedMessage, setInternalEditedMessage] = useState(
    message || "",
  );
  const [currentFeedback, setCurrentFeedback] = useState<
    TypesFeedback | undefined
  >(interaction.feedback);
  const isEditing =
    externalIsEditing !== undefined ? externalIsEditing : internalIsEditing;
  const editedMessage =
    externalEditedMessage !== undefined
      ? externalEditedMessage
      : internalEditedMessage;
  const setEditedMessage = externalSetEditedMessage || setInternalEditedMessage;

  const { updateFeedback } = useUpdateInteractionFeedback(
    session.id || "",
    interaction.id || "",
  );
  const handleCancel =
    externalHandleCancel ||
    (() => {
      setInternalEditedMessage(message || "");
      setInternalIsEditing(false);
    });
  const handleSave =
    externalHandleSave ||
    (() => {
      if (onRegenerate && internalEditedMessage !== message) {
        onRegenerate(interaction.id || "", internalEditedMessage);
      }
      setInternalIsEditing(false);
    });

  const handleFeedback = async (feedback: TypesFeedback) => {
    try {
      await updateFeedback({ feedback });
      setCurrentFeedback(feedback);
    } catch (error) {
      console.error("Failed to update feedback:", error);
    }
  };

  useEffect(() => {
    setCurrentFeedback(interaction.feedback);
  }, [interaction.feedback]);

  useEffect(() => {
    setUserMessageExpanded(false);
  }, [interaction.id, message]);

  // Filter tool steps for this interaction
  const toolSteps = sessionSteps
    .filter((step) => step.interaction_id === interaction.id)
    .map((step) => ({
      id: step.id || "",
      icon: step.icon || "",
      name: step.name || "",
      type: step.type || "",
      message: step.message || "",
      created: step.created || "",
      details: {
        arguments: step.details?.arguments || {},
      },
    }));

  // Derive copy text from response_entries when response_message is empty
  // (the API strips it to save bandwidth when entries exist)
  const copyText = useMemo(() => {
    if (message) return message;
    const entries = (interaction as any)?.response_entries as ResponseEntry[] | undefined;
    if (!entries || entries.length === 0) return "";
    return entries
      .filter((e: ResponseEntry) => e.type === "text")
      .map((e: ResponseEntry) => e.content)
      .join("\n\n");
  }, [message, interaction]);
  const shouldCollapseUserMessage = !isFromAssistant && !!message && (
    message.length > 600 || message.split("\n").length > 8
  );

  if (!serverConfig || !serverConfig.filestore_prefix) return null;
  if (!interaction) return null;

  const getFileURL = (url: string) => {
    if (!url) return "";
    if (!serverConfig) return "";
    if (url.startsWith("data:")) return url;
    return `${serverConfig.filestore_prefix}/${url}?redirect_urls=true`;
  };

  const contentImages: LightboxImage[] = imageURLs
    .filter(() => !!account.user)
    .map((imageURL, index) => {
      const path = imageURL.split("?")[0];
      const basename = path.split("/").pop();
      let imageName = `Image ${index + 1}`;
      if (basename && !basename.startsWith("data:")) {
        try {
          imageName = decodeURIComponent(basename);
        } catch {
          imageName = basename;
        }
      }
      return {
        src: getFileURL(imageURL),
        name: imageName,
      };
    });
  const workspaceImages: LightboxImage[] = workspaceAttachments
    .filter((attachment) => attachment.type === "image")
    .map((attachment) => ({
      src: workspaceAttachmentURL(session.id || "", attachment.path),
      name: attachment.name,
    }));
  const lightboxImages = [...workspaceImages, ...contentImages];
  const workspaceFiles = workspaceAttachments.filter((attachment) => attachment.type === "file");

  return (
    <>
      {serverConfig?.filestore_prefix && lightboxImages.length > 0 && (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: lightboxImages.length === 1
              ? "minmax(0, 1fr)"
              : "repeat(2, minmax(0, 1fr))",
            gap: 1,
            width: "min(100%, 420px)",
            mb: message ? 1.25 : 0,
          }}
        >
          {lightboxImages.map((image, index) => (
            <Box
              key={`${image.src}-${index}`}
              component="button"
              type="button"
              onClick={() => setSelectedImageIndex(index)}
              aria-label={`Preview ${image.name}`}
              sx={{
                p: 0,
                border: "1px solid",
                borderColor: "divider",
                borderRadius: 1,
                overflow: "hidden",
                background: "transparent",
                cursor: "zoom-in",
                minWidth: 0,
                lineHeight: 0,
                "&:hover img": { transform: "scale(1.02)" },
                "&:focus-visible": {
                  outline: "2px solid",
                  outlineColor: "primary.main",
                  outlineOffset: 2,
                },
              }}
            >
              <Box
                component="img"
                src={image.src}
                alt={image.name}
                sx={{
                  display: "block",
                  width: "100%",
                  height: lightboxImages.length === 1 ? "auto" : 180,
                  maxHeight: 220,
                  objectFit: "cover",
                  transition: "transform 0.15s ease",
                }}
              />
            </Box>
          ))}
        </Box>
      )}
      {workspaceFiles.length > 0 && (
        <Box
          sx={{
            display: "flex",
            flexWrap: "wrap",
            gap: 1,
            width: "min(100%, 440px)",
            mb: message ? 1.25 : 0,
          }}
        >
          {workspaceFiles.map((attachment) => {
            const isDocument = /\.(pdf|txt|md|docx?|rtf)$/i.test(attachment.name);
            return (
              <Box
                key={attachment.path}
                component="a"
                href={workspaceAttachmentURL(session.id || "", attachment.path)}
                target="_blank"
                rel="noreferrer"
                aria-label={`Open attachment ${attachment.name}`}
                sx={{
                  width: { xs: "100%", sm: 210 },
                  minWidth: 0,
                  height: 56,
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  px: 1.25,
                  border: "1px solid",
                  borderColor: "divider",
                  borderRadius: 2,
                  color: "text.primary",
                  textDecoration: "none",
                  backgroundColor: "rgba(255,255,255,0.025)",
                  "&:hover": { borderColor: "text.secondary", backgroundColor: "action.hover" },
                }}
              >
                <Box sx={{ display: "flex", flexShrink: 0, color: "text.secondary" }}>
                  {isDocument ? <FileText size={19} /> : <Paperclip size={19} />}
                </Box>
                <Box sx={{ minWidth: 0 }}>
                  <Box sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontSize: "0.8rem" }}>
                    {attachment.name}
                  </Box>
                  <Box sx={{ color: "text.secondary", fontSize: "0.68rem", lineHeight: 1.4 }}>
                    Open attachment
                  </Box>
                </Box>
              </Box>
            );
          })}
        </Box>
      )}
      {toolSteps.length > 0 && isFromAssistant && (
        <ToolStepsWidget steps={toolSteps} />
      )}
      {(message || (interaction as any)?.response_entries?.length > 0) && (
        <Box
          sx={{
            my: 0.5,
            display: "flex",
            alignItems: "flex-start",
            position: "relative",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ width: "100%" }}>
            {isEditing && onRegenerate ? (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <TextField
                  multiline
                  fullWidth
                  value={editedMessage}
                  onChange={(e) => setEditedMessage(e.target.value)}
                  sx={{
                    "& .MuiInputBase-root": {
                      backgroundColor: "rgba(255, 255, 255, 0.05)",
                      borderRadius: 1,
                    },
                  }}
                />
                <Box
                  sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}
                >
                  <Button
                    size="small"
                    onClick={handleCancel}
                    sx={{ textTransform: "none" }}
                  >
                    Cancel
                  </Button>
                  <Button
                    size="small"
                    variant="contained"
                    onClick={handleSave}
                    sx={{ textTransform: "none" }}
                  >
                    Save
                  </Button>
                </Box>
              </Box>
            ) : (
              <>
                <Box
                  sx={{
                    position: "relative",
                    "&:hover .action-buttons, &:focus-within .action-buttons": {
                      opacity: 1,
                    },
                  }}
                >
                  <Box
                    sx={{
                      position: "relative",
                      maxHeight: shouldCollapseUserMessage && !userMessageExpanded ? 176 : "none",
                      overflow: shouldCollapseUserMessage && !userMessageExpanded ? "hidden" : "visible",
                      "&::after": shouldCollapseUserMessage && !userMessageExpanded
                        ? {
                            content: '""',
                            position: "absolute",
                            left: 0,
                            right: 0,
                            bottom: 0,
                            height: 48,
                            pointerEvents: "none",
                            background: (theme) => `linear-gradient(transparent, ${theme.palette.mode === "dark" ? "#1c1c1f" : "#f0f0f2"})`,
                          }
                        : undefined,
                    }}
                  >
                    <MessageWithToolCalls
                      text={message || ""}
                      responseEntries={isFromAssistant ? (interaction as any)?.response_entries : undefined}
                      session={session}
                      getFileURL={getFileURL}
                      showBlinker={false}
                      isStreaming={false}
                      durationMs={getInteractionDurationMs(interaction)}
                      showActivitySummary={isFromAssistant}
                      includeTaskChecklist={isFromAssistant && !!isLastInteraction}
                      onFilterDocument={onFilterDocument}
                    />
                  </Box>
                  {shouldCollapseUserMessage && (
                    <Button
                      size="small"
                      onClick={() => setUserMessageExpanded((expanded) => !expanded)}
                      sx={{
                        minHeight: 24,
                        px: 0,
                        mt: 0.5,
                        textTransform: "none",
                        color: "text.secondary",
                        fontSize: "0.75rem",
                        "&:hover": { background: "transparent", color: "text.primary" },
                      }}
                    >
                      {userMessageExpanded ? "Show less" : "Show full message"}
                    </Button>
                  )}
                  {isFromAssistant && onRegenerate && (
                    <Box
                      className="action-buttons"
                      sx={{
                        display: "flex",
                        justifyContent: "left",
                        alignItems: "center",
                        mt: 1,
                        gap: 1,
                        opacity: 0,
                        transition: "opacity 0.2s ease-in-out",
                        position: "relative",
                        "&:hover, &:focus-within": {
                          opacity: 1,
                        },
                      }}
                    >
                      {enableDebugCopy && (
                        <InteractionDebugCopyButton
                          interaction={interaction}
                          session={session}
                          sessionSteps={sessionSteps}
                          serverConfig={serverConfig}
                        />
                      )}

                      <Tooltip title="Regenerate this response">
                        <IconButton
                          onClick={() =>
                            onRegenerate(
                              interaction.id || "",
                              interaction.prompt_message || "",
                            )
                          }
                          size="small"
                          className="regenerate-btn"
                          sx={(theme) => ({
                            mt: 0.5,
                            color:
                              theme.palette.mode === "light" ? "#888" : "#bbb",
                            "&:hover": {
                              color:
                                theme.palette.mode === "light"
                                  ? "#000"
                                  : "#fff",
                            },
                          })}
                          aria-label="regenerate"
                        >
                          <RefreshIcon sx={{ fontSize: 20 }} />
                        </IconButton>
                      </Tooltip>

                      <CopyButtonWithCheck
                        text={copyText}
                        alwaysVisible={isLastInteraction}
                      />

                      <Tooltip title="Export to PDF">
                        <IconButton
                          onClick={() => setViewingExport(true)}
                          size="small"
                          className="export-btn"
                          sx={(theme) => ({
                            mt: 0.5,
                            color:
                              theme.palette.mode === "light" ? "#888" : "#bbb",
                            "&:hover": {
                              color:
                                theme.palette.mode === "light"
                                  ? "#000"
                                  : "#fff",
                            },
                          })}
                          aria-label="export"
                        >
                          <Download size={16} />
                        </IconButton>
                      </Tooltip>

                      <Tooltip title="Love this">
                        <IconButton
                          onClick={() =>
                            handleFeedback(TypesFeedback.FeedbackLike)
                          }
                          size="small"
                          className="thumbs-up-btn"
                          sx={(theme) => ({
                            mt: 0.5,
                            color:
                              currentFeedback === TypesFeedback.FeedbackLike
                                ? "#4caf50"
                                : theme.palette.mode === "light"
                                  ? "#888"
                                  : "#bbb",
                            "&:hover": {
                              color:
                                currentFeedback === TypesFeedback.FeedbackLike
                                  ? "#45a049"
                                  : theme.palette.mode === "light"
                                    ? "#000"
                                    : "#fff",
                            },
                          })}
                          aria-label="thumbs up"
                        >
                          <ThumbsUp
                            size={16}
                            fill={
                              currentFeedback === TypesFeedback.FeedbackLike
                                ? "#4caf50"
                                : "none"
                            }
                          />
                        </IconButton>
                      </Tooltip>

                      <Tooltip title="Needs improvement">
                        <IconButton
                          onClick={() =>
                            handleFeedback(TypesFeedback.FeedbackDislike)
                          }
                          size="small"
                          className="thumbs-down-btn"
                          sx={(theme) => ({
                            mt: 0.5,
                            color:
                              currentFeedback === TypesFeedback.FeedbackDislike
                                ? "#f44336"
                                : theme.palette.mode === "light"
                                  ? "#888"
                                  : "#bbb",
                            "&:hover": {
                              color:
                                currentFeedback ===
                                TypesFeedback.FeedbackDislike
                                  ? "#d32f2f"
                                  : theme.palette.mode === "light"
                                    ? "#000"
                                    : "#fff",
                            },
                          })}
                          aria-label="thumbs down"
                        >
                          <ThumbsDown
                            size={16}
                            fill={
                              currentFeedback === TypesFeedback.FeedbackDislike
                                ? "#f44336"
                                : "none"
                            }
                          />
                        </IconButton>
                      </Tooltip>
                      {interaction.state ===
                        TypesInteractionState.InteractionStateComplete && (
                        <MessageReceivedTimestamp
                          completedAt={interaction.completed}
                        />
                      )}
                    </Box>
                  )}
                </Box>
              </>
            )}
          </Box>
        </Box>
      )}
      {error && (
        <Row
          sx={{
            mt: 3,
          }}
        >
          <Cell grow>
            <Alert severity="error">
              The system has encountered an error -
              <ClickLink
                sx={{
                  pl: 0.5,
                  pr: 0.5,
                }}
                onClick={() => {
                  setViewingError(true);
                }}
              >
                click here
              </ClickLink>
              to view the details.
            </Alert>
          </Cell>
          {onRegenerate && !message && (
            <Cell
              sx={{
                ml: 2,
                flexShrink: 0,
              }}
            >
              <Button
                variant="contained"
                color="secondary"
                size="small"
                endIcon={<ReplayIcon />}
                sx={{
                  minWidth: 92,
                  whiteSpace: "nowrap",
                }}
                onClick={() =>
                  onRegenerate(
                    interaction.id || "",
                    interaction.prompt_message || "",
                  )
                }
              >
                Retry
              </Button>
            </Cell>
          )}
        </Row>
      )}
      {viewingError && (
        <TerminalWindow
          open
          title="Error"
          data={error}
          onClose={() => {
            setViewingError(false);
          }}
        />
      )}
      {viewingExport && (
        <ExportDocument
          open={viewingExport}
          onClose={() => setViewingExport(false)}
        >
          <ToPDF
            markdown={message || ""}
            onClose={() => setViewingExport(false)}
            filename={`${session.name}-${interaction.id || "export"}.pdf`}
          />
        </ExportDocument>
      )}
      <ImageLightbox
        images={lightboxImages}
        initialIndex={selectedImageIndex ?? 0}
        open={selectedImageIndex !== null}
        onClose={() => setSelectedImageIndex(null)}
      />
    </>
  );
};

export default InteractionInference;
