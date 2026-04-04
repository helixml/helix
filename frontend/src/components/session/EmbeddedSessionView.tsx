import React, {
  useEffect,
  useLayoutEffect,
  useRef,
  useMemo,
  useCallback,
  forwardRef,
  useImperativeHandle,
  useState,
} from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import CircularProgress from "@mui/material/CircularProgress";
import Button from "@mui/material/Button";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import { useQueryClient } from "@tanstack/react-query";

// DEBUG: Set to true to show scroll debug overlay
const DEBUG_SCROLL = false;

// Number of interactions to render initially (and per "load more" click)
const INTERACTIONS_TO_RENDER = 20;

import Interaction from "./Interaction";
import InteractionLiveStream from "./InteractionLiveStream";

import useAccount from "../../hooks/useAccount";
import useApi from "../../hooks/useApi";
import {
  useGetSession,
  useListSessionSteps,
  useListInteractions,
  GET_SESSION_QUERY_KEY,
  LIST_INTERACTIONS_QUERY_KEY,
} from "../../services/sessionService";
import { useStreaming } from "../../contexts/streaming";
import { TypesInteraction, TypesInteractionState } from "../../api/api";
import useLightTheme from "../../hooks/useLightTheme";
import { SESSION_TYPE_TEXT } from "../../types";

interface EmbeddedSessionViewProps {
  sessionId: string;
  onScrollToBottom?: () => void;
}

export interface EmbeddedSessionViewHandle {
  scrollToBottom: () => void;
}

/**
 * EmbeddedSessionView - A lightweight session message thread viewer
 *
 * Simple sticky-scroll behavior:
 * - If you're at the bottom, stay at the bottom as content grows
 * - If you scroll up, stay where you are
 * - If you scroll back to bottom, resume auto-scroll
 */
const EmbeddedSessionView = forwardRef<
  EmbeddedSessionViewHandle,
  EmbeddedSessionViewProps
>(({ sessionId, onScrollToBottom }, ref) => {
  const account = useAccount();
  const api = useApi();
  const lightTheme = useLightTheme();
  const containerRef = useRef<HTMLDivElement>(null);
  const queryClient = useQueryClient();
  const { NewInference, wsConnected } = useStreaming();

  // Simple scroll state: are we currently at the bottom?
  // This is the ONLY state we need for sticky scroll behavior
  const isAtBottomRef = useRef(true);

  // Pagination state: track which page we've loaded up to (page 0 = newest)
  const [oldestPageLoaded, setOldestPageLoaded] = useState(0);
  // Store older interactions loaded via pagination (newest first, so prepend older pages)
  const [olderInteractions, setOlderInteractions] = useState<TypesInteraction[]>([]);
  // Loading state for fetching older interactions
  const [isLoadingOlder, setIsLoadingOlder] = useState(false);
  // Guard: when true, scroll events are from programmatic scrollTo, not user interaction.
  // Prevents the scroll handler from unsetting isAtBottom during auto-scroll.
  const isProgrammaticScrollRef = useRef(false);
  const SCROLL_THRESHOLD = 50;

  // DEBUG: State for debug overlay
  const [debugInfo, setDebugInfo] = useState({
    isAtBottom: true,
    scrollTop: 0,
    scrollHeight: 0,
    clientHeight: 0,
    lastEvent: "init",
    mutationCount: 0,
    scrollCount: 0,
  });

  // Check if currently at bottom
  const checkIsAtBottom = useCallback(() => {
    const container = containerRef.current;
    if (!container) return true;
    const { scrollTop, scrollHeight, clientHeight } = container;
    return scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD;
  }, []);

  // Update isAtBottom on every scroll event
  const handleScroll = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;

    // Skip if this scroll was triggered by our own programmatic scrollTo
    if (isProgrammaticScrollRef.current) return;

    isAtBottomRef.current = checkIsAtBottom();

    if (DEBUG_SCROLL) {
      setDebugInfo((prev) => ({
        ...prev,
        isAtBottom: isAtBottomRef.current,
        scrollTop: Math.round(container.scrollTop),
        scrollHeight: container.scrollHeight,
        clientHeight: container.clientHeight,
        lastEvent: "scroll",
        scrollCount: prev.scrollCount + 1,
      }));
    }
  }, [checkIsAtBottom]);

  // iOS Safari fix: Also track via native scroll event listener
  // React's onScroll might not fire correctly on iOS
  // Depends on session so it re-runs after the container is rendered
  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      if (DEBUG_SCROLL) {
        setDebugInfo((prev) => ({ ...prev, lastEvent: "NO CONTAINER REF" }));
      }
      return;
    }

    // Debug: mark that we attached the listener
    if (DEBUG_SCROLL) {
      setDebugInfo((prev) => ({ ...prev, lastEvent: "listener-attached" }));
    }

    const onNativeScroll = () => {
      if (isProgrammaticScrollRef.current) return;
      isAtBottomRef.current = checkIsAtBottom();

      if (DEBUG_SCROLL) {
        setDebugInfo((prev) => ({
          ...prev,
          isAtBottom: isAtBottomRef.current,
          scrollTop: Math.round(container.scrollTop),
          scrollHeight: container.scrollHeight,
          clientHeight: container.clientHeight,
          lastEvent: "native-scroll",
          scrollCount: prev.scrollCount + 1,
        }));
      }
    };

    // Also listen for wheel events directly (Magic Keyboard trackpad)
    const onWheel = () => {
      // Small delay to let scroll position update
      requestAnimationFrame(() => {
        isAtBottomRef.current = checkIsAtBottom();
        if (DEBUG_SCROLL) {
          setDebugInfo((prev) => ({
            ...prev,
            isAtBottom: isAtBottomRef.current,
            scrollTop: Math.round(container.scrollTop),
            lastEvent: "wheel",
            scrollCount: prev.scrollCount + 1,
          }));
        }
      });
    };

    // Touch events for iOS - check if container is even receiving touch input
    const onTouchMove = () => {
      requestAnimationFrame(() => {
        isAtBottomRef.current = checkIsAtBottom();
        if (DEBUG_SCROLL) {
          setDebugInfo((prev) => ({
            ...prev,
            isAtBottom: isAtBottomRef.current,
            scrollTop: Math.round(container.scrollTop),
            lastEvent: `touch(h=${container.scrollHeight},st=${Math.round(container.scrollTop)})`,
            scrollCount: prev.scrollCount + 1,
          }));
        }
      });
    };

    // Use passive: true for better scroll performance
    container.addEventListener("scroll", onNativeScroll, { passive: true });
    container.addEventListener("wheel", onWheel, { passive: true });
    container.addEventListener("touchmove", onTouchMove, { passive: true });

    return () => {
      container.removeEventListener("scroll", onNativeScroll);
      container.removeEventListener("wheel", onWheel);
      container.removeEventListener("touchmove", onTouchMove);
    };
  }, [checkIsAtBottom]);

  // Scroll to bottom - only scrolls if we're already at the bottom (sticky scroll)
  // Pass force=true to scroll regardless of current position
  const scrollToBottom = useCallback(
    (force = false) => {
      const container = containerRef.current;
      if (!container) return;

      // Only scroll if we're already at the bottom (or forced)
      // This implements "sticky scroll" - stay at bottom if you were there
      if (!force && !isAtBottomRef.current) {
        if (DEBUG_SCROLL) {
          setDebugInfo((prev) => ({
            ...prev,
            lastEvent: `SCROLL_BLOCKED (not at bottom)`,
          }));
        }
        return;
      }

      // DEBUG: Show what triggered the scroll
      if (DEBUG_SCROLL) {
        setDebugInfo((prev) => ({
          ...prev,
          lastEvent: `SCROLL_TO_BOTTOM (${force ? "forced" : "sticky"})`,
        }));
      }

      isProgrammaticScrollRef.current = true;
      container.scrollTop = container.scrollHeight;
      isAtBottomRef.current = true;
      // Clear the guard after the scroll event has fired
      requestAnimationFrame(() => {
        isProgrammaticScrollRef.current = false;
      });
      onScrollToBottom?.();
    },
    [onScrollToBottom],
  );

  // Expose scrollToBottom via ref for parent components
  useImperativeHandle(
    ref,
    () => ({
      scrollToBottom,
    }),
    [scrollToBottom],
  );

  // Fetch session data with auto-refresh.
  // When the WebSocket is connected it is the authoritative real-time source,
  // so we suppress the poll to prevent stale HTTP responses from racing with
  // and overwriting fresh WebSocket-delivered data. Polling resumes automatically
  // whenever the WebSocket drops (network hiccup, server restart, etc.).
  const { data: sessionResponse, refetch: refetchSession } = useGetSession(
    sessionId,
    {
      enabled: !!sessionId,
      refetchInterval: wsConnected ? false : 3000,
    },
  );

  const session = sessionResponse?.data;

  // Fetch paginated interactions (newest first via order=desc)
  // Page 0 = newest interactions, higher pages = older interactions
  const { data: paginatedInteractionsResponse } = useListInteractions(
    sessionId,
    0, // Always fetch page 0 (newest) - older pages fetched on demand
    INTERACTIONS_TO_RENDER,
    'desc',
    { enabled: !!sessionId }
  );
  const paginatedData = paginatedInteractionsResponse?.data;

  // Fetch session steps
  const { data: sessionSteps } = useListSessionSteps(sessionId, {
    enabled: !!sessionId,
  });

  // Track if we're streaming (last interaction is in waiting state)
  // Use paginatedData for the most recent interaction state
  const isStreaming = useMemo(() => {
    const interactions = paginatedData?.interactions;
    if (!interactions || interactions.length === 0) return false;
    // paginatedData.interactions is newest-first, so [0] is the most recent
    const lastInteraction = interactions[0];
    return (
      lastInteraction.state === TypesInteractionState.InteractionStateWaiting
    );
  }, [paginatedData?.interactions]);

  // Scroll to bottom on initial load
  const hasInitiallyScrolled = useRef(false);

  // Reset scroll state and clear stale cache when sessionId changes
  const prevSessionIdRef = useRef(sessionId);
  useEffect(() => {
    if (sessionId !== prevSessionIdRef.current) {
      const oldSessionId = prevSessionIdRef.current;
      prevSessionIdRef.current = sessionId;

      // Reset scroll tracking so the new session scrolls to bottom on load
      hasInitiallyScrolled.current = false;
      isAtBottomRef.current = true;
      prevScrollHeightRef.current = 0;
      // Reset pagination state for new session
      setOldestPageLoaded(0);
      setOlderInteractions([]);

      // Remove old session's React Query cache to prevent flash of stale content
      if (oldSessionId) {
        queryClient.removeQueries({
          queryKey: GET_SESSION_QUERY_KEY(oldSessionId),
        });
        queryClient.removeQueries({
          queryKey: LIST_INTERACTIONS_QUERY_KEY(oldSessionId),
        });
      }
    }
  }, [sessionId, queryClient]);

  useEffect(() => {
    if (
      paginatedData?.interactions &&
      paginatedData.interactions.length > 0 &&
      !hasInitiallyScrolled.current
    ) {
      hasInitiallyScrolled.current = true;
      // Use setTimeout to ensure content is fully rendered (RAF may be too early)
      setTimeout(() => {
        scrollToBottom(true); // Force scroll on initial load
      }, 100);
    }
  }, [paginatedData?.interactions?.length, scrollToBottom]);

  // Track previous streaming state to detect when streaming ends
  const prevIsStreamingRef = useRef(isStreaming);

  // Scroll to bottom when streaming ends - critical for reliable scroll behavior
  useEffect(() => {
    const wasStreaming = prevIsStreamingRef.current;
    prevIsStreamingRef.current = isStreaming;

    // When streaming transitions from true to false, scroll to show final content
    if (wasStreaming && !isStreaming) {
      // Wait for final content to render before scrolling
      setTimeout(() => {
        scrollToBottom(true);
      }, 100);
    }
  }, [isStreaming, scrollToBottom]);

  // Maintain sticky scroll when session data updates (e.g., from polling or WebSocket)
  // Use useLayoutEffect to run synchronously after DOM mutations but before browser paint
  // This ensures we scroll before any scroll events could fire from layout reflow
  const prevScrollHeightRef = useRef(0);

  useLayoutEffect(() => {
    const container = containerRef.current;
    if (!container || !session?.interactions) return;

    const prevScrollHeight = prevScrollHeightRef.current;
    const currentScrollHeight = container.scrollHeight;

    // If content height changed and we were at bottom, scroll to bottom
    if (currentScrollHeight !== prevScrollHeight && prevScrollHeight > 0) {
      // Check isAtBottomRef BEFORE the scroll - this is the value from before the DOM update
      if (isAtBottomRef.current) {
        isProgrammaticScrollRef.current = true;
        container.scrollTop = currentScrollHeight;
        requestAnimationFrame(() => {
          isProgrammaticScrollRef.current = false;
        });
      }
    }

    prevScrollHeightRef.current = currentScrollHeight;
  });

  // DISABLED: MutationObserver was causing constant scroll jumps
  // because scroll events weren't being detected on iOS Safari,
  // so isAtBottomRef stayed true and every mutation scrolled to bottom.
  //
  // Instead, we only scroll on:
  // 1. Initial load
  // 2. When interaction count increases (new message)
  // 3. When InteractionLiveStream calls onMessageUpdate during streaming
  //
  // This is less "smooth" for streaming but more reliable.

  // Reload session handler
  const handleReloadSession = useCallback(async () => {
    await refetchSession();
    return session;
  }, [refetchSession, session]);

  // Regenerate handler
  const handleRegenerate = useCallback(
    async (interactionID: string, message: string) => {
      if (!session) return;

      await NewInference({
        message: message,
        sessionId: sessionId,
        type: SESSION_TYPE_TEXT,
      });

      scrollToBottom();
    },
    [session, sessionId, NewInference, scrollToBottom],
  );

  // Handler for loading older interactions via API pagination
  const handleLoadOlder = useCallback(async () => {
    const container = containerRef.current;
    if (!container || isLoadingOlder) return;

    // Save scroll position before expanding
    const prevScrollHeight = container.scrollHeight;

    setIsLoadingOlder(true);
    try {
      const nextPage = oldestPageLoaded + 1;
      const apiClient = api.getApiClient();
      const response = await apiClient.v1SessionsInteractionsDetail(sessionId, {
        page: nextPage,
        per_page: INTERACTIONS_TO_RENDER,
        order: 'desc',
      });

      const newInteractions = response.data?.interactions || [];
      if (newInteractions.length > 0) {
        // Prepend older interactions (they come newest-first within the page,
        // so we need to reverse then prepend)
        setOlderInteractions(prev => [...newInteractions.reverse(), ...prev]);
        setOldestPageLoaded(nextPage);
      }
    } finally {
      setIsLoadingOlder(false);
    }

    // After state update, restore scroll position so viewport doesn't jump
    requestAnimationFrame(() => {
      if (containerRef.current) {
        const newScrollHeight = containerRef.current.scrollHeight;
        containerRef.current.scrollTop += newScrollHeight - prevScrollHeight;
      }
    });
  }, [api, sessionId, oldestPageLoaded, isLoadingOlder]);

  // Compute which interactions to render using paginated data
  // paginatedData.interactions are newest-first (page 0), we reverse for display (oldest first)
  // olderInteractions are already in oldest-first order from handleLoadOlder
  // NOTE: These useMemos MUST be before any early returns to maintain consistent hook order
  const newestInteractions = useMemo(() => {
    const interactions = paginatedData?.interactions || [];
    // Reverse to get oldest-first order for display
    return [...interactions].reverse();
  }, [paginatedData?.interactions]);

  // Combine older (loaded via pagination) + newest (from initial fetch)
  const visibleInteractions = useMemo(() => {
    return [...olderInteractions, ...newestInteractions];
  }, [olderInteractions, newestInteractions]);

  const totalInteractions = visibleInteractions.length;

  // Check if there are more pages to load
  const totalCount = paginatedData?.totalCount || 0;
  const totalPages = paginatedData?.totalPages || 1;
  const hasOlderInteractions = oldestPageLoaded < totalPages - 1;

  const isOwner = account.user?.id === session?.owner;

  // Show loading state while fetching session
  if (!session) {
    return (
      <Box
        sx={{
          flex: 1,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexDirection: "column",
          gap: 2,
        }}
      >
        <CircularProgress size={32} />
        <Typography variant="body2" color="text.secondary">
          Loading session...
        </Typography>
      </Box>
    );
  }

  // Show empty state if no interactions (check paginated data, not session.interactions)
  if (totalInteractions === 0 && !paginatedData?.interactions?.length) {
    return (
      <Box
        sx={{
          flex: 1,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <Typography variant="body2" color="text.secondary">
          No messages yet. Send a message to start the conversation.
        </Typography>
      </Box>
    );
  }

  return (
    <Box
      ref={containerRef}
      onScroll={handleScroll}
      sx={{
        // Use height: 0 + flex: 1 to force this to be the scrollable container
        // Without height: 0, the container may expand to fit content on iOS
        height: 0,
        flex: 1,
        overflow: "auto",
        display: "flex",
        flexDirection: "column",
        minHeight: 0,
        position: "relative",
        // Enable momentum scrolling on iOS
        WebkitOverflowScrolling: "touch",
        ...lightTheme.scrollbar,
      }}
    >
      {/* DEBUG OVERLAY */}
      {DEBUG_SCROLL && (
        <Box
          sx={{
            position: "sticky",
            top: 0,
            left: 0,
            right: 0,
            zIndex: 9999,
            backgroundColor: "rgba(0, 0, 0, 0.85)",
            color: "#0f0",
            fontFamily: "monospace",
            fontSize: "11px",
            padding: "4px 8px",
            pointerEvents: "none",
          }}
        >
          <div>
            atBottom: {debugInfo.isAtBottom ? "YES" : "NO"} | scrollTop:{" "}
            {debugInfo.scrollTop}
          </div>
          <div>
            scrollH: {debugInfo.scrollHeight} | clientH:{" "}
            {debugInfo.clientHeight}
          </div>
          <div>last: {debugInfo.lastEvent}</div>
          <div>
            mutations: {debugInfo.mutationCount} | scrolls:{" "}
            {debugInfo.scrollCount}
          </div>
        </Box>
      )}
      <Box
        sx={{
          width: "100%",
          maxWidth: 700,
          mx: "auto",
          px: 2,
          py: 2,
          display: "flex",
          flexDirection: "column",
          gap: 2,
          // Ensure content can shrink on narrow screens
          minWidth: 0,
          boxSizing: "border-box",
        }}
      >
        {/* Show "Load older" button when there are more interactions */}
        {hasOlderInteractions && (
          <Button
            variant="text"
            size="small"
            startIcon={isLoadingOlder ? <CircularProgress size={16} /> : <ExpandLessIcon />}
            onClick={handleLoadOlder}
            disabled={isLoadingOlder}
            sx={{
              alignSelf: "center",
              color: "text.secondary",
              textTransform: "none",
              mb: 1,
            }}
          >
            {isLoadingOlder ? 'Loading...' : `Show ${INTERACTIONS_TO_RENDER} older messages`}
          </Button>
        )}
        {visibleInteractions.map((interaction, index) => {
          const isLastInteraction = index === totalInteractions - 1;
          const isLive =
            isLastInteraction &&
            interaction.state === TypesInteractionState.InteractionStateWaiting;
          return (
            <Interaction
              key={interaction.id}
              serverConfig={account.serverConfig}
              interaction={interaction}
              session={session}
              highlightAllFiles={false}
              onReloadSession={handleReloadSession}
              onRegenerate={handleRegenerate}
              isLastInteraction={isLastInteraction}
              isOwner={isOwner}
              isAdmin={account.admin}
              scrollToBottom={scrollToBottom}
              session_id={sessionId}
              sessionSteps={sessionSteps?.data || []}
            >
              {isLive && (isOwner || account.admin) && (
                <InteractionLiveStream
                  session_id={sessionId}
                  interaction={interaction}
                  session={session}
                  serverConfig={account.serverConfig}
                  onMessageUpdate={scrollToBottom}
                />
              )}
            </Interaction>
          );
        })}
      </Box>
    </Box>
  );
});

export default EmbeddedSessionView;
