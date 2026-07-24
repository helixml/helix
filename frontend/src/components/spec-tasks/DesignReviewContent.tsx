/**
 * DesignReviewContent - Core spec review UI component
 *
 * Displays the spec review documents (requirements, technical design,
 * implementation plan) with inline commenting functionality.
 *
 * This is a clean content component without any floating window logic.
 * Used by both SpecTaskReviewPage and the Workspace view.
 */

import React, {
  useState,
  useEffect,
  useRef,
  useMemo,
  useCallback,
} from "react";
import {
  Box,
  Tabs,
  Tab,
  Typography,
  Chip,
  IconButton,
  CircularProgress,
  Alert,
  Paper,
  Tooltip,
  Badge,
  ToggleButtonGroup,
  ToggleButton,
  GlobalStyles,
} from "@mui/material";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import Description from "@mui/icons-material/Description";
import { GitBranch } from "lucide-react";
import CommentIcon from "@mui/icons-material/Comment";
import AddCommentIcon from "@mui/icons-material/AddComment";
import ShareIcon from "@mui/icons-material/Share";
import CheckIcon from "@mui/icons-material/Check";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneLight } from "react-syntax-highlighter/dist/esm/styles/prism";
import { useQueryClient } from "@tanstack/react-query";
import ReconnectingWebSocket from "reconnecting-websocket";
import { applyPatch } from "../../utils/patchUtils";
import {
  useDesignReview,
  useDesignReviewComments,
  useSubmitReview,
  useCreateComment,
  useResolveComment,
  getUnresolvedCount,
  designReviewKeys,
  useCommentQueueStatus,
} from "../../services/designReviewService";
import useSnackbar from "../../hooks/useSnackbar";
import useApi from "../../hooks/useApi";
import useAccount from "../../hooks/useAccount";
import { useOAuthFlow } from "../../hooks/useOAuthFlow";
import { useListOAuthProviders } from "../../services/oauthProvidersService";
import { findOAuthProviderForType, vcsScopesForProvider } from "../../utils/oauthProviders";
import InlineCommentBubble from "./InlineCommentBubble";
import InlineCommentForm from "./InlineCommentForm";
import CommentLogSidebar from "./CommentLogSidebar";
import ReviewActionFooter from "./ReviewActionFooter";
import ReviewSubmitDialog from "./ReviewSubmitDialog";
import RejectDesignDialog from "./RejectDesignDialog";
import { useSpecTask, useArchiveSpecTask } from "../../services/specTaskService";
import { TypesSpecTaskStatus } from "../../api/api";

type DocumentType = "requirements" | "technical_design" | "implementation_plan";

interface DesignReviewContentProps {
  specTaskId: string;
  reviewId: string;
  onClose: () => void;
  onImplementationStarted?: () => void;
  initialTab?: DocumentType;
  /** Hide the title in header - use when embedded in a page with its own breadcrumbs */
  hideTitle?: boolean;
  /** If provided, renders a "← Back to task" tab as the first tab in the tab strip */
  onBack?: () => void;
}

const DOCUMENT_LABELS = {
  requirements: "Requirements Specification",
  technical_design: "Technical Design",
  implementation_plan: "Implementation Plan",
};

type NormMapEntry = { node: Text; offset: number };

// Collapse all whitespace runs (including newlines across DOM nodes) to a single
// space while building a parallel map from each normalized character back to its
// source text node + raw offset. A comment's quoted_text is captured from
// rendered text, so a cross-block selection contains newlines that the DOM's
// textContent concatenation does not — normalizing both sides lets us match and
// still recover an exact Range for positioning.
function buildNormalizedTextMap(root: HTMLElement): {
  text: string;
  map: NormMapEntry[];
} {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
  let text = "";
  const map: NormMapEntry[] = [];
  let prevWasSpace = false;
  let node: Node | null;
  while ((node = walker.nextNode())) {
    const raw = node.textContent || "";
    for (let i = 0; i < raw.length; i++) {
      if (/\s/.test(raw[i])) {
        if (prevWasSpace) continue;
        text += " ";
        map.push({ node: node as Text, offset: i });
        prevWasSpace = true;
      } else {
        text += raw[i];
        map.push({ node: node as Text, offset: i });
        prevWasSpace = false;
      }
    }
  }
  return { text, map };
}

const normalizeQuote = (s: string): string => s.replace(/\s+/g, " ").trim();

// Normalized-text character offset of a selection point (node, rawOffset) within
// root. Returns null if the point is not inside a text node of root. Used to
// remember which occurrence of a repeated phrase a comment was made against.
function normalizedOffsetOfPoint(
  root: HTMLElement,
  targetNode: Node,
  targetOffset: number,
): number | null {
  if (targetNode.nodeType !== Node.TEXT_NODE) return null;
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
  let normLen = 0;
  let prevWasSpace = false;
  let node: Node | null;
  while ((node = walker.nextNode())) {
    const raw = node.textContent || "";
    const limit =
      node === targetNode ? Math.min(targetOffset, raw.length) : raw.length;
    for (let i = 0; i < limit; i++) {
      if (/\s/.test(raw[i])) {
        if (prevWasSpace) continue;
        normLen++;
        prevWasSpace = true;
      } else {
        normLen++;
        prevWasSpace = false;
      }
    }
    if (node === targetNode) return normLen;
  }
  return null;
}

export default function DesignReviewContent({
  specTaskId,
  reviewId,
  onClose,
  onImplementationStarted,
  initialTab = "requirements",
  hideTitle = false,
  onBack,
}: DesignReviewContentProps) {
  const snackbar = useSnackbar();
  const api = useApi();

  // Narrow layout: comment bubbles stack in-flow below the document instead of
  // floating in the document column's right gutter.
  //
  // The side-positioned bubble is absolutely placed at left:820px (width 300px)
  // relative to the 800px document column, which is centred (mx:auto) inside the
  // document area. Its right edge therefore lands at
  // (contentWidth - 800) / 2 + 1120, which only stays within bounds when the
  // document content area is at least ~1440px wide. Below that the bubble — and
  // the Resolve button in its header — is pushed off the right edge, forcing a
  // horizontal scroll to reach it.
  //
  // We measure the *actual* document-area width (documentRef.clientWidth), not
  // the window, because this component renders both standalone
  // (SpecTaskReviewPage) and embedded (workspace TabsView), where the
  // surrounding chrome differs at the same window size. Empirically the side
  // bubble stops overflowing once the document area is ~1440px wide; we add a
  // small gutter margin on top of that.
  const SIDE_PANEL_MIN_DOC_AREA_WIDTH = 1460;
  const [docAreaWidth, setDocAreaWidth] = useState<number | null>(null);
  // Default to the stacked layout until measured, so the bubble never flashes
  // off-screen on first paint.
  const isNarrowViewport =
    docAreaWidth === null || docAreaWidth < SIDE_PANEL_MIN_DOC_AREA_WIDTH;

  // Review state
  const [activeTab, setActiveTab] = useState<DocumentType>(initialTab);
  const [showCommentForm, setShowCommentForm] = useState(false);
  const [selectedText, setSelectedText] = useState("");
  const [commentText, setCommentText] = useState("");
  const [commentFormPosition, setCommentFormPosition] = useState({
    x: 0,
    y: 0,
  });
  const [overallComment, setOverallComment] = useState("");
  const [showSubmitDialog, setShowSubmitDialog] = useState(false);
  const [submitDecision, setSubmitDecision] = useState<
    "approve" | "request_changes"
  >("approve");
  const [startingImplementation, setStartingImplementation] = useState(false);
  const [showCommentLog, setShowCommentLog] = useState(false);
  const [viewedTabs, setViewedTabs] = useState<Set<DocumentType>>(
    new Set(["requirements"]),
  );
  const viewedContentRef = useRef<Map<DocumentType, string>>(new Map());
  const [showRejectDialog, setShowRejectDialog] = useState(false);
  const [rejectReason, setRejectReason] = useState("");
  const archiveMutation = useArchiveSpecTask();
  const [shareLinkCopied, setShareLinkCopied] = useState(false);
  const [commentPositions, setCommentPositions] = useState<Map<string, number>>(
    new Map(),
  );
  // Comments whose quoted_text could not be located in the rendered document.
  // They are still shown (fail safe) but flagged as unanchored.
  const [unlocatedCommentIds, setUnlocatedCommentIds] = useState<Set<string>>(
    new Set(),
  );
  // Normalized character offset of the current selection start, stored with the
  // comment so a repeated phrase re-anchors to the occurrence that was selected.
  const [selectedOffset, setSelectedOffset] = useState<number | null>(null);
  // Track when we just created a comment - enables queue polling immediately without waiting for comments refresh
  const [awaitingCommentResponse, setAwaitingCommentResponse] = useState(false);

  // Refs for positioning
  const documentRef = useRef<HTMLDivElement>(null);
  const markdownRef = useRef<HTMLDivElement>(null);
  const commentRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  // Ref to the in-progress new-comment form so we can measure its rendered
  // height and feed it into the bubble-stacking algorithm.
  const commentFormRef = useRef<HTMLDivElement | null>(null);
  // Bump to force re-stacking after the form mounts/unmounts and we have its
  // measured height (offsetHeight isn't reactive on its own).
  const [commentFormMeasureTick, setCommentFormMeasureTick] = useState(0);

  // Track the rendered width of the scrollable document area so we can decide
  // between the side-gutter (wide) and stacked (narrow) comment layouts based
  // on real available space rather than the window size. Re-attached whenever
  // the main content (re)mounts.
  useEffect(() => {
    const el = documentRef.current;
    if (!el) return;
    const measure = () => setDocAreaWidth(el.clientWidth);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  });

  // Refs and state for highlight preservation and hover button
  const savedRangeRef = useRef<Range | null>(null);
  const savedHighlightRangeRef = useRef<Range | null>(null);
  const hoveredElementRef = useRef<Element | null>(null);
  const [hoverButtonPosition, setHoverButtonPosition] = useState<{
    x: number;
    y: number;
    elementText: string;
  } | null>(null);

  const { data: task } = useSpecTask(specTaskId, {
    enabled: !!specTaskId,
  });
  const unfinishedDependencies = useMemo(() => {
    const dependencies = task?.depends_on || [];
    return dependencies.filter((dependency) => {
      const dependencyStatus = dependency.status || "";
      const isCompleted =
        (dependencyStatus as string) === "done" || (dependencyStatus as string) === "completed";
      return !dependency.archived && !isCompleted;
    });
  }, [task?.depends_on]);
  const blockingDependency = unfinishedDependencies[0];

  // First fetch comments to know if we should poll for review updates
  const { data: commentsData, isLoading: commentsLoading } =
    useDesignReviewComments(specTaskId, reviewId, {
      refetchInterval: 5000,
    });

  // Check if there are comments awaiting agent responses
  const hasAwaitingComments = useMemo(() => {
    return (commentsData?.comments || []).some(
      (c) => c.request_id && !c.agent_response,
    );
  }, [commentsData]);

  // Fetch review data
  const { data: reviewData, isLoading: reviewLoading } = useDesignReview(
    specTaskId,
    reviewId,
    {
      refetchInterval: hasAwaitingComments ? 3000 : 5000,
    },
  );

  const submitReviewMutation = useSubmitReview(specTaskId, reviewId);
  const createCommentMutation = useCreateComment(specTaskId, reviewId);
  const resolveCommentMutation = useResolveComment(specTaskId, reviewId);

  // Open the matching repository provider when approval requires OAuth.
  const { startOAuthFlow } = useOAuthFlow();
  const { data: oauthProviders } = useListOAuthProviders();

  // Get queue status for streaming
  // Enable polling immediately when we create a comment (awaitingCommentResponse)
  // OR when we detect awaiting comments from the comments query (hasAwaitingComments)
  const shouldPollQueueStatus = awaitingCommentResponse || hasAwaitingComments;
  const { data: queueStatus } = useCommentQueueStatus(specTaskId, reviewId, {
    enabled: shouldPollQueueStatus,
  });

  // Clear awaitingCommentResponse when comments data confirms no more pending comments
  // This handles edge cases like timeouts or missed WebSocket messages
  useEffect(() => {
    if (awaitingCommentResponse && !hasAwaitingComments && commentsData) {
      // Comments data has refreshed and shows no pending comments - clear the flag
      setAwaitingCommentResponse(false);
    }
  }, [awaitingCommentResponse, hasAwaitingComments, commentsData]);

  // Apply DOM highlight when comment form opens, to preserve the visual selection
  // (browser clears native selection when the form's TextField auto-focuses)
  useEffect(() => {
    if (showCommentForm && savedRangeRef.current) {
      applyHighlight(savedRangeRef.current);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showCommentForm]);

  // Track streaming agent response
  const [streamingResponse, setStreamingResponse] = useState<{
    commentId: string;
    content: string;
    entries: Array<{ type: 'text' | 'tool_call'; content: string; message_id: string; tool_name?: string; tool_status?: string }>;
    isComplete?: boolean; // true = done streaming, keep content visible until cache refreshes
  } | null>(null);
  const account = useAccount();
  const queryClient = useQueryClient();

  const review = reviewData?.review;
  const allComments = commentsData?.comments || [];

  // Refs to access latest values inside WebSocket messageHandler (avoids stale closures)
  const allCommentsRef = useRef(allComments);
  const queueStatusRef = useRef(queueStatus);
  useEffect(() => {
    allCommentsRef.current = allComments;
  }, [allComments]);
  useEffect(() => {
    queueStatusRef.current = queueStatus;
  }, [queueStatus]);

  // Get planning session ID from spec task (more reliable than waiting for queue status)
  const planningSessionId = task?.planning_session_id;

  const activeDocComments = useMemo(
    () => allComments.filter((c) => c.document_type === activeTab),
    [allComments, activeTab],
  );
  const unresolvedCount = getUnresolvedCount(allComments);

  const ALL_TABS: DocumentType[] = ["requirements", "technical_design", "implementation_plan"];
  const allTabsViewed = ALL_TABS.every((t) => viewedTabs.has(t));

  // Memoize document content
  const documentContent = useMemo(() => {
    if (!review) return "";
    switch (activeTab) {
      case "requirements":
        return (
          review.requirements_spec ||
          "# No requirements specification available"
        );
      case "technical_design":
        return review.technical_design || "# No technical design available";
      case "implementation_plan":
        return (
          review.implementation_plan || "# No implementation plan available"
        );
    }
  }, [review, activeTab]);

  // Get comment counts per document type
  const getCommentCount = (docType: DocumentType) => {
    return allComments.filter((c) => c.document_type === docType && !c.resolved)
      .length;
  };

  const getTabContent = (tab: DocumentType): string => {
    if (!review) return "";
    switch (tab) {
      case "requirements":
        return review.requirements_spec || "";
      case "technical_design":
        return review.technical_design || "";
      case "implementation_plan":
        return review.implementation_plan || "";
    }
  };

  // Snapshot content on initial mount for the default tab
  useEffect(() => {
    if (review && !viewedContentRef.current.has("requirements")) {
      viewedContentRef.current.set("requirements", getTabContent("requirements"));
    }
  }, [review]);

  // Invalidate viewed tabs when content changes. The active tab is exempt:
  // the user is currently looking at it, so we refresh its snapshot in place
  // rather than flagging it unread.
  useEffect(() => {
    if (!review) return;
    const tabs: DocumentType[] = ["requirements", "technical_design", "implementation_plan"];
    const invalidated: DocumentType[] = [];
    for (const tab of tabs) {
      const snapshot = viewedContentRef.current.get(tab);
      if (snapshot === undefined) continue;
      if (snapshot === getTabContent(tab)) continue;

      if (tab === activeTab) {
        viewedContentRef.current.set(tab, getTabContent(tab));
        continue;
      }
      invalidated.push(tab);
      viewedContentRef.current.delete(tab);
    }
    if (invalidated.length > 0) {
      setViewedTabs((prev) => {
        const next = new Set(prev);
        for (const tab of invalidated) next.delete(tab);
        return next;
      });
    }
  }, [review?.requirements_spec, review?.technical_design, review?.implementation_plan, activeTab]);

  // Handle tab change
  const handleTabChange = (newTab: DocumentType) => {
    setActiveTab(newTab);
    setViewedTabs((prev) => new Set(prev).add(newTab));
    viewedContentRef.current.set(newTab, getTabContent(newTab));
    if (documentRef.current) {
      documentRef.current.scrollTop = 0;
    }
  };

  // Jump to the next unread tab in canonical order, wrapping past the end.
  const handleNextDocument = () => {
    const startIdx = ALL_TABS.indexOf(activeTab);
    for (let i = 1; i <= ALL_TABS.length; i++) {
      const candidate = ALL_TABS[(startIdx + i) % ALL_TABS.length];
      if (!viewedTabs.has(candidate)) {
        handleTabChange(candidate);
        return;
      }
    }
  };

  // Handle share link
  const handleShareLink = () => {
    // Unauthenticated public viewer — the /api/v1 prefix is required, otherwise
    // the link hits the SPA and forces an OIDC login.
    const shareUrl = `${window.location.origin}/api/v1/spec-tasks/${specTaskId}/view`;
    navigator.clipboard.writeText(shareUrl);
    setShareLinkCopied(true);
    setTimeout(() => setShareLinkCopied(false), 2000);
    snackbar.success("Share link copied to clipboard");
  };

  // Separate comments with quoted_text (inline) vs without (general)
  const inlineComments = useMemo(
    () => activeDocComments.filter((c) => c.quoted_text && !c.resolved),
    [activeDocComments],
  );

  // WebSocket subscription for real-time agent responses
  // Always subscribe when viewing a spec task - that way we're already connected when comments are created
  useEffect(() => {
    // [DRWS-DEBUG] Log subscription decision
    // With BFF auth, session cookie is automatically sent with WebSocket connections
    console.log("[DRWS-DEBUG] Subscription check:", {
      planningSessionId,
      hasUser: !!account.user,
      willSubscribe: !!(planningSessionId && account.user),
    });

    if (!planningSessionId || !account.user) {
      console.log(
        "[DRWS-DEBUG] Not subscribing - missing planningSessionId or user",
      );
      return;
    }

    const sessionId = planningSessionId;
    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsHost = window.location.host;
    const url = `${wsProtocol}//${wsHost}/api/v1/ws/user?session_id=${sessionId}`;

    console.log("[DRWS-DEBUG] Creating WebSocket connection to:", url);
    const rws = new ReconnectingWebSocket(url);

    rws.addEventListener("open", () => {
      console.log("[DRWS-DEBUG] WebSocket CONNECTED");
    });

    rws.addEventListener("error", (err) => {
      console.error("[DRWS-DEBUG] WebSocket ERROR:", err);
    });

    rws.addEventListener("close", () => {
      console.log("[DRWS-DEBUG] WebSocket CLOSED");
    });

    let accumulatedResponse = "";
    // Track per-entry streaming content with type metadata
    type StreamEntry = { type: 'text' | 'tool_call'; content: string; message_id: string; tool_name?: string; tool_status?: string };
    let streamEntries: StreamEntry[] = [];

    const messageHandler = (event: MessageEvent) => {
      try {
        const parsedData = JSON.parse(event.data);

        console.log(
          "[DRWS-DEBUG] WebSocket message received, type:",
          parsedData.type,
        );

        // Handle interaction_patch events (entry-based streaming from Go server)
        if (
          parsedData.type === "interaction_patch" &&
          parsedData.entry_patches
        ) {
          const entryPatches = parsedData.entry_patches as Array<{
            index: number;
            patch: string;
            patch_offset: number;
            total_length: number;
            type?: string;
            tool_name?: string;
            tool_status?: string;
          }>;
          const entryCount = parsedData.entry_count as number;

          // Grow array if new entries appeared
          while (streamEntries.length < entryCount) {
            streamEntries.push({ type: 'text', content: '', message_id: String(streamEntries.length) });
          }
          // Apply per-entry patches and capture type metadata
          for (const ep of entryPatches) {
            if (ep.index < streamEntries.length) {
              streamEntries[ep.index].content = applyPatch(
                streamEntries[ep.index].content,
                ep.patch_offset,
                ep.patch,
                ep.total_length,
              );
              if (ep.type) streamEntries[ep.index].type = ep.type as 'text' | 'tool_call';
              if (ep.tool_name) streamEntries[ep.index].tool_name = ep.tool_name;
              if (ep.tool_status) streamEntries[ep.index].tool_status = ep.tool_status;
            }
          }
          // Join text entries for flat content fallback
          accumulatedResponse = streamEntries.filter(e => e.content).map(e => e.content).join("\n\n");

          console.log(
            "[DRWS-DEBUG] interaction_patch received, entry_count:",
            entryCount,
            "reconstructed length:",
            accumulatedResponse.length,
          );

          // Find the comment that's currently being processed
          const currentQueueStatus = queueStatusRef.current;
          const currentComments = allCommentsRef.current;

          const targetCommentId =
            currentQueueStatus?.current_comment_id ||
            currentComments.find((c) => c.request_id && !c.agent_response)
              ?.id ||
            [...currentComments]
              .reverse()
              .find((c) => !c.agent_response && !c.resolved)?.id;

          if (targetCommentId) {
            setStreamingResponse({
              commentId: targetCommentId,
              content: accumulatedResponse,
              entries: [...streamEntries],
            });
          }
        }

        // Handle session_update events (full interaction updates)
        if (
          parsedData.type === "session_update" &&
          parsedData.session?.interactions
        ) {
          const lastInteraction =
            parsedData.session.interactions[
              parsedData.session.interactions.length - 1
            ];

          console.log(
            "[DRWS-DEBUG] session_update with interactions, last interaction:",
            {
              hasResponseMessage: !!lastInteraction?.response_message,
              state: lastInteraction?.state,
              responseLength: lastInteraction?.response_message?.length,
            },
          );

          if (lastInteraction?.response_message) {
            accumulatedResponse = lastInteraction.response_message;

            // Find the comment that's currently being processed
            // Use refs to get latest values (not stale closure values)
            const currentQueueStatus = queueStatusRef.current;
            const currentComments = allCommentsRef.current;

            console.log("[DRWS-DEBUG] Looking for target comment:", {
              queueStatusCurrentCommentId:
                currentQueueStatus?.current_comment_id,
              commentsCount: currentComments.length,
              commentsWithRequestId: currentComments
                .filter((c) => c.request_id && !c.agent_response)
                .map((c) => c.id),
              commentsWithoutResponse: currentComments
                .filter((c) => !c.agent_response && !c.resolved)
                .map((c) => c.id),
            });

            // Priority: queue status (most reliable), then find from comments list
            const targetCommentId =
              currentQueueStatus?.current_comment_id ||
              currentComments.find((c) => c.request_id && !c.agent_response)
                ?.id ||
              // Fallback: most recent comment without a response
              [...currentComments]
                .reverse()
                .find((c) => !c.agent_response && !c.resolved)?.id;

            console.log("[DRWS-DEBUG] Target comment ID:", targetCommentId);

            if (targetCommentId) {
              console.log(
                "[DRWS-DEBUG] Setting streaming response for comment:",
                targetCommentId,
                "length:",
                accumulatedResponse.length,
              );
              setStreamingResponse({
                commentId: targetCommentId,
                content: accumulatedResponse,
                entries: [...streamEntries],
              });
            } else {
              console.warn(
                "[DRWS-DEBUG] No target comment found - cannot attribute response!",
              );
            }

            if (lastInteraction.state === "complete") {
              console.log(
                "[DRWS-DEBUG] Interaction complete - invalidating queries and marking stream complete",
              );
              // Invalidate both comments AND review detail (which contains the design doc content)
              // The agent may have updated the design doc via git push in response to the comment
              queryClient.invalidateQueries({
                queryKey: designReviewKeys.comments(specTaskId, reviewId),
              });
              queryClient.invalidateQueries({
                queryKey: designReviewKeys.detail(specTaskId, reviewId),
              });
              // Mark as complete rather than clearing immediately — keeps the response content
              // visible on comment 1 while the React Query cache refreshes. The next comment's
              // streaming events will naturally overwrite this with the new comment's data.
              setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null);
              // Reset entry tracking for next streaming response
              streamEntries = [];
            }
          }
        }

        // Handle interaction_update events (sent on completion)
        if (
          parsedData.type === "interaction_update" &&
          parsedData.interaction
        ) {
          const interaction = parsedData.interaction;
          console.log("[DRWS-DEBUG] interaction_update received:", {
            state: interaction.state,
            responseLength: interaction.response_message?.length,
          });

          if (interaction.response_message) {
            accumulatedResponse = interaction.response_message;
            // Use structured entries from the wire if available, else flat text entry
            if (interaction.response_entries?.length) {
              streamEntries = interaction.response_entries;
            } else {
              streamEntries = [{ type: 'text', content: interaction.response_message, message_id: '0' }];
            }

            const currentQueueStatus = queueStatusRef.current;
            const currentComments = allCommentsRef.current;

            const targetCommentId =
              currentQueueStatus?.current_comment_id ||
              currentComments.find((c) => c.request_id && !c.agent_response)
                ?.id ||
              [...currentComments]
                .reverse()
                .find((c) => !c.agent_response && !c.resolved)?.id;

            if (targetCommentId) {
              setStreamingResponse({
                commentId: targetCommentId,
                content: accumulatedResponse,
                entries: [...streamEntries],
              });
            }
          }

          if (interaction.state === "complete") {
            console.log(
              "[DRWS-DEBUG] interaction_update complete - invalidating queries",
            );
            queryClient.invalidateQueries({
              queryKey: designReviewKeys.comments(specTaskId, reviewId),
            });
            queryClient.invalidateQueries({
              queryKey: designReviewKeys.detail(specTaskId, reviewId),
            });
            // Mark as complete rather than clearing immediately (see session_update handler above)
            setStreamingResponse(prev => prev ? { ...prev, isComplete: true } : null);
            streamEntries = [];
          }
        }
      } catch (error) {
        console.error("[DRWS-DEBUG] Error parsing WebSocket message:", error);
      }
    };

    rws.addEventListener("message", messageHandler);

    return () => {
      console.log("[DRWS-DEBUG] Cleaning up WebSocket subscription");
      rws.removeEventListener("message", messageHandler);
      rws.close();
    };
  }, [planningSessionId, specTaskId, reviewId, account.user]);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyPress = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      if (target.tagName === "INPUT" || target.tagName === "TEXTAREA") {
        return;
      }

      switch (e.key.toLowerCase()) {
        case "c":
          if (!e.ctrlKey && !e.metaKey) {
            setShowCommentForm((prev) => !prev);
            e.preventDefault();
          }
          break;
        case "escape":
          if (showCommentForm) {
            removeHighlight();
            setShowCommentForm(false);
            e.preventDefault();
          } else if (showSubmitDialog) {
            setShowSubmitDialog(false);
            e.preventDefault();
          }
          break;
        case "1":
        case "2":
        case "3":
          const tabs: DocumentType[] = [
            "requirements",
            "technical_design",
            "implementation_plan",
          ];
          const tabIndex = parseInt(e.key) - 1;
          if (tabIndex >= 0 && tabIndex < tabs.length) {
            setActiveTab(tabs[tabIndex]);
            e.preventDefault();
          }
          break;
      }
    };

    window.addEventListener("keydown", handleKeyPress);
    return () => window.removeEventListener("keydown", handleKeyPress);
  }, [showCommentForm, showSubmitDialog]);

  // Recalculate comment positions
  const inlineCommentIds = useMemo(
    () => inlineComments.map((c) => c.id).join(","),
    [inlineComments],
  );

  const positionRetryRef = useRef(0);
  const maxPositionRetries = 5;

  // Sentinel id used to represent the in-progress new-comment form inside
  // the bubble-stacking algorithm so the form participates in collision
  // resolution alongside existing comment bubbles.
  const NEW_COMMENT_FORM_KEY = "__new_comment_form__";

  // Stable callback so passing it as a ref prop doesn't cause repeated
  // null/node toggling and infinite re-renders.
  const handleCommentFormRef = useCallback((el: HTMLDivElement | null) => {
    commentFormRef.current = el;
    // The form just mounted/unmounted — trigger a re-stack so the
    // algorithm can pick up the real measured height.
    setCommentFormMeasureTick((t) => t + 1);
  }, []);

  useEffect(() => {
    // On narrow viewports bubbles render inline (position: relative) and
    // the form is a bottom-sheet (position: fixed). Stacking math is
    // irrelevant in that mode.
    const formActive =
      !isNarrowViewport && showCommentForm && !!selectedText;

    if (
      !documentRef.current ||
      (inlineComments.length === 0 && !formActive) ||
      !documentContent
    ) {
      setCommentPositions((prev) => (prev.size === 0 ? prev : new Map()));
      setUnlocatedCommentIds((prev) => (prev.size === 0 ? prev : new Set()));
      positionRetryRef.current = 0;
      return;
    }

    const calculatePositions = (retryCount: number) => {
      if (!documentRef.current?.textContent) {
        return false;
      }

      const positions: Array<{ id: string; baseY: number; height: number }> =
        [];
      let hasInvalidPositions = false;

      // Ordinal of each comment among those sharing identical quoted_text, so a
      // repeated phrase anchors to distinct occurrences in creation order when
      // no stored offset is available.
      const sameTextSeen = new Map<string, number>();
      const unlocated = new Set<string>();

      inlineComments.forEach((comment) => {
        if (!comment.quoted_text) return;

        const ordinal = sameTextSeen.get(comment.quoted_text) ?? 0;
        sameTextSeen.set(comment.quoted_text, ordinal + 1);

        const hasOffset =
          typeof comment.start_offset === "number" && comment.start_offset > 0;
        const baseY = findQuotedTextPosition(
          comment.quoted_text,
          hasOffset ? comment.start_offset : undefined,
          ordinal,
        );

        const ref = commentRefs.current.get(comment.id!);
        const height = ref?.offsetHeight || 250;

        if (baseY === null) {
          // Not found — retry a few times in case the DOM is still settling.
          if (retryCount < maxPositionRetries) {
            hasInvalidPositions = true;
            return;
          }
          // Retries exhausted: keep the comment visible (fail safe) by stacking
          // it at the top of the gutter and flagging it as unanchored.
          unlocated.add(comment.id!);
          positions.push({ id: comment.id!, baseY: 0, height });
          return;
        }

        positions.push({ id: comment.id!, baseY, height });
      });

      setUnlocatedCommentIds((prev) => {
        if (prev.size === unlocated.size) {
          let same = true;
          for (const id of unlocated) {
            if (!prev.has(id)) {
              same = false;
              break;
            }
          }
          if (same) return prev;
        }
        return unlocated;
      });

      if (formActive) {
        // 220 is a sensible default matching the form's typical rendered
        // height with a 3-line TextField; the real value replaces it once
        // the form has mounted and the ref callback bumps the tick.
        const formHeight = commentFormRef.current?.offsetHeight || 220;
        positions.push({
          id: NEW_COMMENT_FORM_KEY,
          baseY: commentFormPosition.y,
          height: formHeight,
        });
      }

      // Sort top-to-bottom so the item with the higher anchor wins its
      // preferred slot; later items get pushed down to avoid overlap.
      positions.sort((a, b) => a.baseY - b.baseY);

      if (hasInvalidPositions && retryCount < maxPositionRetries) {
        return false;
      }

      const newPositions = new Map<string, number>();
      const minGap = 10;

      positions.forEach((comment, index) => {
        let adjustedY = comment.baseY;

        let hasOverlap = true;
        while (hasOverlap) {
          hasOverlap = false;

          for (let i = 0; i < index; i++) {
            const other = positions[i];
            const otherY = newPositions.get(other.id)!;
            const otherBottom = otherY + other.height;
            const thisBottom = adjustedY + comment.height;

            if (
              !(
                adjustedY >= otherBottom + minGap ||
                thisBottom <= otherY - minGap
              )
            ) {
              adjustedY = otherBottom + minGap;
              hasOverlap = true;
              break;
            }
          }
        }

        newPositions.set(comment.id, adjustedY);
      });

      setCommentPositions((prev) => {
        if (prev.size !== newPositions.size) return newPositions;
        for (const [id, pos] of newPositions) {
          if (prev.get(id) !== pos) return newPositions;
        }
        return prev;
      });

      return true;
    };

    const scheduleCalculation = (retryCount: number) => {
      const delay = 100 * Math.pow(2, retryCount);
      const timeoutId = setTimeout(() => {
        requestAnimationFrame(() => {
          const success = calculatePositions(retryCount);
          if (!success && retryCount < maxPositionRetries) {
            scheduleCalculation(retryCount + 1);
          }
        });
      }, delay);
      return timeoutId;
    };

    positionRetryRef.current = 0;
    const timeoutId = scheduleCalculation(0);

    return () => clearTimeout(timeoutId);
  }, [
    inlineCommentIds,
    activeTab,
    documentContent,
    showCommentForm,
    selectedText,
    commentFormPosition.y,
    commentFormMeasureTick,
    isNarrowViewport,
  ]);

  // Find the Y position of quoted text within the rendered markdown.
  //
  // Matching runs over a whitespace-normalized copy of the *markdown* element
  // (markdownRef) only — never documentRef, which also contains the comment
  // bubbles and would produce false matches. Normalization lets cross-block
  // selections (which captured newlines) still match. When the same phrase
  // occurs multiple times, `targetOffset` (the stored offset of the selection)
  // wins; otherwise the `occurrenceIndex`-th occurrence is used so multiple
  // comments on the same text spread across distinct occurrences.
  const findQuotedTextPosition = (
    quotedText: string,
    targetOffset?: number | null,
    occurrenceIndex?: number,
  ): number | null => {
    if (!markdownRef.current || !documentRef.current) return null;

    const { text, map } = buildNormalizedTextMap(markdownRef.current);
    const q = normalizeQuote(quotedText);
    if (!q) return null;

    const starts: number[] = [];
    let idx = text.indexOf(q);
    while (idx !== -1) {
      starts.push(idx);
      idx = text.indexOf(q, idx + 1);
    }
    if (starts.length === 0) return null;

    let chosen = starts[0];
    if (targetOffset !== undefined && targetOffset !== null) {
      chosen = starts.reduce(
        (best, s) =>
          Math.abs(s - targetOffset) < Math.abs(best - targetOffset)
            ? s
            : best,
        starts[0],
      );
    } else if (
      occurrenceIndex !== undefined &&
      occurrenceIndex < starts.length
    ) {
      chosen = starts[occurrenceIndex];
    }

    const startEntry = map[chosen];
    const endEntry = map[chosen + q.length - 1];
    if (!startEntry || !endEntry) return null;

    try {
      const range = document.createRange();
      range.setStart(startEntry.node, startEntry.offset);
      range.setEnd(endEntry.node, endEntry.offset + 1);

      const rect = range.getBoundingClientRect();
      const containerRect = documentRef.current.getBoundingClientRect();

      if (rect.top === 0 && rect.bottom === 0 && rect.height === 0) {
        return null;
      }

      return rect.top - containerRect.top + documentRef.current.scrollTop;
    } catch {
      return null;
    }
  };

  const applyHighlight = (range: Range) => {
    try {
      // Clear any existing highlight
      CSS.highlights.delete("comment-highlight");

      // Create highlight from the range - no DOM modification
      const highlight = new Highlight(range);
      CSS.highlights.set("comment-highlight", highlight);

      // Store range for cleanup (not a DOM node)
      savedHighlightRangeRef.current = range;
    } catch {
      // Fallback: skip visual highlight, comment form still opens
      savedHighlightRangeRef.current = null;
    }
  };

  const removeHighlight = () => {
    CSS.highlights.delete("comment-highlight");
    savedHighlightRangeRef.current = null;
  };

  const handleTextSelection = (isTouch: boolean = false) => {
    const processSelection = () => {
      const selection = window.getSelection();
      const text = selection?.toString().trim();
      if (text && text.length > 0 && selection.rangeCount > 0) {
        const range = selection.getRangeAt(0);
        const selectionContainer = range.commonAncestorContainer;

        let node: Node | null = selectionContainer;
        let isInMarkdown = false;
        while (node) {
          if (node === markdownRef.current) {
            isInMarkdown = true;
            break;
          }
          node = node.parentNode;
        }

        if (!isInMarkdown) {
          return;
        }

        const rect = range.getBoundingClientRect();
        const containerRect = documentRef.current?.getBoundingClientRect();

        if (containerRect) {
          const scrollTop = documentRef.current?.scrollTop || 0;
          const yPosition = rect.top - containerRect.top + scrollTop;

          // Clear stale highlight before applying new selection
          removeHighlight();

          savedRangeRef.current = range.cloneRange();
          setSelectedText(text);
          // Remember which occurrence was selected so the comment re-anchors
          // there even if the phrase repeats.
          setSelectedOffset(
            markdownRef.current
              ? normalizedOffsetOfPoint(
                  markdownRef.current,
                  range.startContainer,
                  range.startOffset,
                )
              : null,
          );
          setCommentFormPosition({ x: 0, y: yPosition });
          setShowCommentForm(true);
          // Apply highlight immediately — the useEffect won't re-fire
          // if showCommentForm was already true
          applyHighlight(range.cloneRange());
        }
      }
    };

    // On touch devices, iOS may not have finalized the selection yet on touchend
    // Add a small delay to allow the selection to be set
    if (isTouch) {
      setTimeout(processSelection, 50);
    } else {
      processSelection();
    }
  };

  const handleCreateComment = async () => {
    if (!commentText.trim()) {
      snackbar.error("Comment text is required");
      return;
    }

    try {
      const normalizedLen = selectedText
        ? normalizeQuote(selectedText).length
        : 0;
      await createCommentMutation.mutateAsync({
        document_type: activeTab,
        quoted_text: selectedText || undefined,
        start_offset:
          selectedOffset !== null && selectedOffset >= 0
            ? selectedOffset
            : undefined,
        end_offset:
          selectedOffset !== null && selectedOffset >= 0
            ? selectedOffset + normalizedLen
            : undefined,
        comment_text: commentText,
      });

      snackbar.success("Comment added successfully");
      removeHighlight();
      setCommentText("");
      setSelectedText("");
      setSelectedOffset(null);
      setShowCommentForm(false);
    } catch (error: any) {
      snackbar.error(`Failed to add comment: ${error.message}`);
    }
  };

  const handleResolveComment = async (commentId: string) => {
    try {
      await resolveCommentMutation.mutateAsync(commentId);
      snackbar.success("Comment resolved");
    } catch (error: any) {
      snackbar.error(`Failed to resolve comment: ${error.message}`);
    }
  };

  const handleSubmitReview = async () => {
    try {
      await submitReviewMutation.mutateAsync({
        decision: submitDecision,
        overall_comment: overallComment || undefined,
      });

      if (submitDecision === "approve") {
        snackbar.success("Design approved! Agent starting implementation...");
        setShowSubmitDialog(false);

        if (onImplementationStarted) {
          onImplementationStarted();
        }

        onClose();
      } else {
        snackbar.success("Changes requested. Agent will be notified.");
        setShowSubmitDialog(false);
        onClose();
      }
    } catch (error: any) {
      // Open the matching provider connection flow on OAuth enforcement.
      const respData = error?.response?.data;
      if (respData?.error === "oauth_required") {
        setShowSubmitDialog(false);
        const providerType = respData?.provider_type === "gitlab" ? "gitlab" : "github";
        const providerName = providerType === "gitlab" ? "GitLab" : "GitHub";
        const oauthProvider = findOAuthProviderForType(oauthProviders, providerType);
        if (oauthProvider?.id) {
          snackbar.info(`Connect ${providerName} to approve this design.`);
          startOAuthFlow({
            providerId: oauthProvider.id,
            scopes: vcsScopesForProvider(oauthProvider.type, oauthProvider.name),
            onSuccess: () => {
              snackbar.success(
                `${providerName} connected. Click Approve again to submit.`,
              );
            },
            onError: (oauthError) => {
              snackbar.error(`${providerName} connection failed: ${oauthError}`);
            },
          });
        } else {
          // No GitHub provider is configured system-wide. The backend's
          // error message is PR-centric and actionless for this user, so
          // override it with admin-direction guidance.
          snackbar.error(
            `${providerName} OAuth is not configured on this Helix instance. Ask your administrator to set it up before approving designs.`,
          );
        }
        return;
      }
      snackbar.error(`Failed to submit review: ${error.message}`);
    }
  };

  const handleRejectDesign = async () => {
    try {
      await archiveMutation.mutateAsync({ taskId: specTaskId, archived: true });
      snackbar.success("Design rejected - spec task archived");
      setShowRejectDialog(false);
      onClose();
    } catch (error: any) {
      snackbar.error(`Failed to reject design: ${error.message}`);
    }
  };

  const handleStartImplementation = async () => {
    setStartingImplementation(true);
    try {
      const apiClient = api.getApiClient();
      const response =
        await apiClient.v1SpecTasksApproveImplementationCreate(specTaskId);
      const data = response.data as any;

      snackbar.success(`Implementation started on branch: ${data.branch_name}`);

      if (data.pr_template_url) {
        window.open(data.pr_template_url, "_blank");
      }

      if (onImplementationStarted) {
        onImplementationStarted();
      }

      onClose();
    } catch (error: any) {
      snackbar.error(`Failed to start implementation: ${error.message}`);
    } finally {
      setStartingImplementation(false);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case "approved":
        return "success";
      case "changes_requested":
        return "error";
      case "in_review":
        return "warning";
      case "pending":
        return "info";
      case "superseded":
        return "default";
      default:
        return "default";
    }
  };

  if (reviewLoading || commentsLoading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="400px"
      >
        <CircularProgress />
      </Box>
    );
  }

  if (!review) {
    return (
      <Box p={4}>
        <Alert severity="error">Review not found</Alert>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <GlobalStyles styles={{ "::highlight(comment-highlight)": { backgroundColor: "rgba(25, 118, 210, 0.4)" } }} />
      {/* Main Content Area */}
      <Box display="flex" flex={1} overflow="hidden">
        {/* Document Viewer */}
        <Box flex={1} display="flex" flexDirection="column" overflow="hidden">
          {/* Compact single-line header: Tabs on left, git info + actions on right */}
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              borderBottom: 1,
              borderColor: "divider",
              bgcolor: "background.default",
              minHeight: 48,
            }}
          >
            {/* Tabs on the left */}
            {onBack && (
              <>
                {/* Desktop: Chat/Spec toggle matching the issue detail view */}
                <ToggleButtonGroup
                  value="spec"
                  exclusive
                  onChange={(_, val) => { if (val === "chat") onBack(); }}
                  size="small"
                  sx={{
                    display: { xs: 'none', sm: 'flex' },
                    flexShrink: 0,
                    alignSelf: 'center',
                    ml: 3,
                    mr: 1,
                    "& .MuiToggleButton-root": {
                      px: 1.25,
                      py: 0.25,
                      fontSize: "0.8rem",
                      fontWeight: 500,
                      textTransform: "none",
                      border: "1px solid",
                      borderColor: "divider",
                      color: "text.secondary",
                      "&.Mui-selected": {
                        color: "text.primary",
                        backgroundColor: "action.selected",
                      },
                    },
                  }}
                >
                  <ToggleButton value="chat">Chat</ToggleButton>
                  <ToggleButton value="spec">
                    <Description sx={{ fontSize: 14, mr: 0.5 }} />
                    Spec
                  </ToggleButton>
                </ToggleButtonGroup>
                {/* Mobile: just an arrow icon */}
                <IconButton
                  onClick={onBack}
                  size="small"
                  sx={{ display: { xs: 'flex', sm: 'none' }, ml: 0.5, mr: 0.5 }}
                >
                  <ArrowBackIcon sx={{ fontSize: 18 }} />
                </IconButton>
              </>
            )}
            <Tabs
              value={activeTab}
              onChange={(_, value) => handleTabChange(value)}
              variant="scrollable"
              scrollButtons="auto"
              sx={{
                minHeight: 48,
                "& .MuiTab-root": {
                  minHeight: 48,
                  py: 0,
                  textTransform: "uppercase",
                  fontSize: "0.75rem",
                  fontWeight: 600,
                  letterSpacing: "0.5px",
                },
              }}
            >
              {ALL_TABS.map((tab) => (
                <Tab
                  key={tab}
                  label={
                    <Box display="flex" alignItems="center" gap={0.5}>
                      {DOCUMENT_LABELS[tab]}
                      {!viewedTabs.has(tab) && (
                        <Box
                          sx={{
                            width: 8,
                            height: 8,
                            borderRadius: "50%",
                            bgcolor: "warning.main",
                            flexShrink: 0,
                          }}
                        />
                      )}
                      {getCommentCount(tab) > 0 && (
                        <Chip
                          label={getCommentCount(tab)}
                          size="small"
                          color="warning"
                          sx={{
                            height: 16,
                            minWidth: 16,
                            fontSize: "0.65rem",
                            "& .MuiChip-label": { px: 0.5 },
                          }}
                        />
                      )}
                    </Box>
                  }
                  value={tab}
                />
              ))}
            </Tabs>

            {/* Git info and actions on the right */}
            <Box display="flex" alignItems="center" gap={1.5} pr={2}>
              <Box sx={{ display: { xs: "none", sm: "flex" } }}>
                <Tooltip title={`Commit: ${review.git_commit_hash}`}>
                  <Chip
                    icon={<GitBranch size={14} />}
                    label={`${review.git_branch} @ ${review.git_commit_hash.substring(0, 7)}`}
                    size="small"
                    variant="outlined"
                    sx={{ height: 24, fontSize: "0.7rem" }}
                  />
                </Tooltip>
              </Box>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ whiteSpace: "nowrap", display: { xs: "none", sm: "block" } }}
              >
                {new Date(review.git_pushed_at).toLocaleString()}
              </Typography>

              <Tooltip
                title={shareLinkCopied ? "Link copied!" : "Copy shareable link"}
              >
                <IconButton
                  size="small"
                  onClick={handleShareLink}
                  sx={{ p: 0.5 }}
                >
                  {shareLinkCopied ? (
                    <CheckIcon color="success" fontSize="small" />
                  ) : (
                    <ShareIcon fontSize="small" />
                  )}
                </IconButton>
              </Tooltip>

              <Tooltip title="Comment log">
                <IconButton
                  size="small"
                  onClick={() => setShowCommentLog(!showCommentLog)}
                  sx={{ p: 0.5 }}
                >
                  <Badge
                    badgeContent={activeDocComments.length}
                    color="primary"
                  >
                    <CommentIcon fontSize="small" />
                  </Badge>
                </IconButton>
              </Tooltip>
            </Box>
          </Box>

          <Box
            ref={documentRef}
            flex={1}
            overflow="auto"
            p={2}
            onMouseLeave={() => {
              hoveredElementRef.current = null;
              setHoverButtonPosition(null);
            }}
            onMouseMove={(e) => {
              if (!hoverButtonPosition) return;
              const containerRect = (e.currentTarget as HTMLElement).getBoundingClientRect();
              const mouseX = e.clientX - containerRect.left;
              const buttonRightEdge = containerRect.width / 2 + 400 + 4 + 28;
              if (mouseX > buttonRightEdge) {
                setHoverButtonPosition(null);
                hoveredElementRef.current = null;
              }
            }}
            sx={{
              bgcolor: "background.default",
              position: "relative",
            }}
          >
            {/* Hover button for adding comment without text selection */}
            {hoverButtonPosition && !showCommentForm && !isNarrowViewport && (
              <Tooltip title="Add comment" placement="top">
                <IconButton
                  size="small"
                  onClick={() => {
                    if (hoveredElementRef.current) {
                      const range = document.createRange();
                      range.selectNodeContents(hoveredElementRef.current);
                      savedRangeRef.current = range;
                    }
                    setSelectedText(hoverButtonPosition.elementText);
                    // Whole-block selection has no precise start point; fall
                    // back to occurrence-ordinal anchoring.
                    setSelectedOffset(null);
                    setCommentFormPosition({ x: 0, y: hoverButtonPosition.y });
                    setHoverButtonPosition(null);
                    setShowCommentForm(true);
                  }}
                  sx={{
                    position: "absolute",
                    top: hoverButtonPosition.y,
                    left: "calc(50% + 400px + 4px)",
                    zIndex: 15,
                    bgcolor: "#1976d2",
                    color: "#fff",
                    width: 28,
                    height: 28,
                    "&:hover": { bgcolor: "#1565c0" },
                  }}
                >
                  <AddCommentIcon sx={{ fontSize: 14 }} />
                </IconButton>
              </Tooltip>
            )}

            {/* Document content */}
            <Box
              onMouseDown={() => { if (!showCommentForm) removeHighlight(); }}
              onMouseUp={() => handleTextSelection(false)}
              onTouchEnd={() => handleTextSelection(true)}
              onMouseMove={(e) => {
                if (showCommentForm || isNarrowViewport) return;
                const target = e.target as Node;
                for (const bubble of commentRefs.current.values()) {
                  if (bubble.contains(target)) {
                    if (hoverButtonPosition) {
                      setHoverButtonPosition(null);
                      hoveredElementRef.current = null;
                    }
                    return;
                  }
                }
                const blockTags = new Set(["P", "LI", "H1", "H2", "H3", "H4", "BLOCKQUOTE", "PRE"]);
                let node: Node | null = target;
                while (node && node !== markdownRef.current) {
                  if (node.nodeType === Node.ELEMENT_NODE && blockTags.has((node as Element).tagName)) {
                    const el = node as Element;
                    if (el === hoveredElementRef.current) return;
                    hoveredElementRef.current = el;
                    const rect = el.getBoundingClientRect();
                    const containerRect = documentRef.current?.getBoundingClientRect();
                    if (containerRect) {
                      const scrollTop = documentRef.current?.scrollTop || 0;
                      const y = rect.top - containerRect.top + scrollTop;
                      setHoverButtonPosition({ x: 0, y, elementText: (el as HTMLElement).innerText.trim() });
                    }
                    return;
                  }
                  node = node.parentNode;
                }
              }}
              sx={{
                maxWidth: "800px",
                minWidth: "400px",
                mx: "auto",
                position: "relative",
                "& .markdown-body": {
                  bgcolor: "background.paper",
                  px: 2.5,
                  py: 1.5,
                  borderRadius: 1,
                  boxShadow: "0 2px 8px rgba(0,0,0,0.06)",
                  fontSize: "14px",
                  lineHeight: 1.6,
                  color: "text.primary",

                  "& h1": {
                    fontSize: "1.5rem",
                    fontWeight: 600,
                    color: "text.primary",
                    marginTop: 0,
                    marginBottom: "0.75rem",
                    lineHeight: 1.3,
                    borderBottom: 1,
                    borderColor: "divider",
                    paddingBottom: "0.5rem",
                    "&:first-of-type": {
                      marginTop: 0,
                    },
                  },
                  "& h2": {
                    fontSize: "1.25rem",
                    fontWeight: 600,
                    color: "text.primary",
                    marginTop: "1.25rem",
                    marginBottom: "0.5rem",
                    lineHeight: 1.3,
                  },
                  "& h3": {
                    fontSize: "1.1rem",
                    fontWeight: 600,
                    color: "text.primary",
                    marginTop: "1rem",
                    marginBottom: "0.4rem",
                  },
                  "& p": {
                    marginBottom: "0.75rem",
                  },
                  "& ul, & ol": {
                    marginBottom: "0.75rem",
                    paddingLeft: "1.5rem",
                  },
                  "& li": {
                    marginBottom: "0.25rem",
                  },
                  "& blockquote": {
                    borderLeft: "3px solid",
                    borderColor: "divider",
                    paddingLeft: "1rem",
                    marginLeft: 0,
                    fontStyle: "italic",
                    color: "text.secondary",
                  },
                  "& code": {
                    fontFamily: "Monaco, Consolas, monospace",
                    fontSize: "0.85em",
                    bgcolor: "action.hover",
                    padding: "1px 4px",
                    borderRadius: "3px",
                    border: 1,
                    borderColor: "divider",
                  },
                  "& pre": {
                    marginBottom: "0.75rem",
                    borderRadius: "4px",
                    overflow: "auto",
                  },
                  "& a": {
                    color: "#00d5ff",
                    textDecoration: "none",
                    "&:hover": {
                      textDecoration: "underline",
                    },
                    "&:visited": {
                      color: "#00d5ff",
                    },
                  },
                  "&::selection": {
                    bgcolor: "#b3d7ff",
                    color: "#000",
                  },
                  cursor: "text",
                  "& p, & li, & h1, & h2, & h3, & h4": {
                    cursor: "text",
                    transition: "background-color 0.15s ease",
                    "&:hover": {
                      backgroundColor: "rgba(59, 130, 246, 0.03)",
                    },
                  },
                },
              }}
            >
              <Paper ref={markdownRef} className="markdown-body" elevation={2}>
                <ReactMarkdown
                  remarkPlugins={[remarkGfm]}
                  components={{
                    code({ node, inline, className, children, ref, ...props }: any) {
                      const match = /language-(\w+)/.exec(className || "");
                      return !inline && match ? (
                        <SyntaxHighlighter
                          style={oneLight as any}
                          language={match[1]}
                          PreTag="div"
                          customStyle={{
                            borderRadius: "4px",
                            border: "1px solid #e0e0e0",
                            fontSize: "14px",
                            // Prevent code blocks from capturing vertical scroll
                            // clip doesn't create a scroll container like auto does
                            overflowX: "auto",
                            overflowY: "clip",
                          }}
                          {...props}
                        >
                          {String(children).replace(/\n$/, "")}
                        </SyntaxHighlighter>
                      ) : (
                        <code className={className} {...props}>
                          {children}
                        </code>
                      );
                    },
                  }}
                >
                  {documentContent}
                </ReactMarkdown>
              </Paper>

              {/* Inline Comments Overlay */}
              {inlineComments.map((comment) => {
                if (!comment.quoted_text) return null;
                const yPos = commentPositions.get(comment.id!);
                if (yPos === undefined) return null;

                const isCurrentlyStreaming =
                  streamingResponse?.commentId === comment.id;

                return (
                  <InlineCommentBubble
                    key={comment.id}
                    comment={comment}
                    yPos={yPos}
                    onResolve={handleResolveComment}
                    streamingResponse={
                      isCurrentlyStreaming
                        ? streamingResponse.content
                        : undefined
                    }
                    streamingEntries={
                      isCurrentlyStreaming
                        ? streamingResponse.entries
                        : undefined
                    }
                    isStreamingComplete={
                      isCurrentlyStreaming
                        ? !!streamingResponse.isComplete
                        : undefined
                    }
                    commentRef={(el) => {
                      if (el) {
                        commentRefs.current.set(comment.id!, el);
                      } else {
                        commentRefs.current.delete(comment.id!);
                      }
                    }}
                    isNarrowViewport={isNarrowViewport}
                    unlocated={unlocatedCommentIds.has(comment.id!)}
                  />
                );
              })}

              {/* New Comment Form (Inline) */}
              <InlineCommentForm
                show={showCommentForm}
                yPos={
                  commentPositions.get(NEW_COMMENT_FORM_KEY) ??
                  commentFormPosition.y
                }
                selectedText={selectedText}
                commentText={commentText}
                onCommentChange={setCommentText}
                onCreate={handleCreateComment}
                onCancel={() => {
                  removeHighlight();
                  setShowCommentForm(false);
                  setCommentText("");
                  setSelectedText("");
                  setSelectedOffset(null);
                }}
                isNarrowViewport={isNarrowViewport}
                isSubmitting={createCommentMutation.isPending}
                outerRef={handleCommentFormRef}
              />
            </Box>
          </Box>
        </Box>

        {/* Comment Log Sidebar */}
        <CommentLogSidebar
          show={showCommentLog}
          comments={activeDocComments}
          onResolveComment={handleResolveComment}
          streamingResponse={streamingResponse}
        />
      </Box>

      {/* Review Actions Footer */}
      {review && task?.status !== TypesSpecTaskStatus.TaskStatusDone && (
        <ReviewActionFooter
          reviewStatus={review.status}
          unresolvedCount={unresolvedCount}
          startingImplementation={startingImplementation}
          implementationStarted={
            task?.status === TypesSpecTaskStatus.TaskStatusSpecApproved ||
            task?.status === TypesSpecTaskStatus.TaskStatusImplementation ||
            task?.status ===
              TypesSpecTaskStatus.TaskStatusImplementationQueued ||
            task?.status ===
              TypesSpecTaskStatus.TaskStatusImplementationReview ||
            task?.status === TypesSpecTaskStatus.TaskStatusPullRequest
          }
          isBlockedByDependencies={unfinishedDependencies.length > 0}
          blockedReason={
            blockingDependency
              ? `Depends on: #${blockingDependency.task_number ? String(blockingDependency.task_number).padStart(6, "0") : blockingDependency.id?.slice(0, 8)}`
              : ""
          }
          onApprove={() => {
            setSubmitDecision("approve");
            setShowSubmitDialog(true);
          }}
          onRequestChanges={() => {
            setSubmitDecision("request_changes");
            setShowSubmitDialog(true);
          }}
          allTabsViewed={allTabsViewed}
          hasNextDocument={!allTabsViewed}
          onNextDocument={handleNextDocument}
          onReject={() => setShowRejectDialog(true)}
          onStartImplementation={handleStartImplementation}
        />
      )}

      {/* Dialogs */}
      <ReviewSubmitDialog
        open={showSubmitDialog}
        onClose={() => setShowSubmitDialog(false)}
        decision={submitDecision}
        overallComment={overallComment}
        onCommentChange={setOverallComment}
        onSubmit={handleSubmitReview}
        isSubmitting={submitReviewMutation.isPending}
      />

      <RejectDesignDialog
        open={showRejectDialog}
        onClose={() => setShowRejectDialog(false)}
        reason={rejectReason}
        onReasonChange={setRejectReason}
        onReject={handleRejectDesign}
        isSubmitting={archiveMutation.isPending}
      />
    </Box>
  );
}
