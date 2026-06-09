import React, {
  useEffect,
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
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import VerticalAlignBottomIcon from "@mui/icons-material/VerticalAlignBottom";
import KeyboardDoubleArrowDownIcon from "@mui/icons-material/KeyboardDoubleArrowDown";
import PauseIcon from "@mui/icons-material/Pause";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAutoScrollPreference,
  AUTO_SCROLL_NEAR_BOTTOM_PX,
  USER_SCROLL_UNLOCK_PX,
} from "../../hooks/useAutoScrollPreference";

// Timeout (ms) after which a fresh wheel event is treated as a new gesture
// and the upward-scroll accumulator resets. Stops "scroll up 60px, read,
// then scroll up another 60px" from cumulatively crossing the threshold.
const USER_SCROLL_GESTURE_TIMEOUT_MS = 500;

// Number of interactions to render initially (and per "load more" click).
// Keep this low — long-running agent sessions can have interactions with
// hundreds of entries, each rendered as a Markdown component.
const INTERACTIONS_TO_RENDER = 5;

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
  // When true, force the (otherwise persisted, globally-shared)
  // auto-scroll preference ON when this view mounts. Surfaces that want
  // the transcript to follow live output by default — e.g. the helix-org
  // worker detail chat — opt in so a previously-toggled-OFF value doesn't
  // leave the chat opening paused.
  autoScrollOnMount?: boolean;
}

export interface EmbeddedSessionViewHandle {
  scrollToBottom: () => void;
}

/**
 * EmbeddedSessionView - session message thread viewer.
 *
 * Auto-scroll model (deliberately simple — see commit history for prior
 * sticky-scroll attempts that were too race-prone to be reliable):
 *
 *   - A single global preference (`helix.autoScroll`, default ON) controls
 *     whether new content auto-scrolls the chat to the bottom.
 *   - When ON: every content-height *growth* (driven by ResizeObserver on
 *     the inner content Box) is followed by a scroll to bottom. Renders that don't
 *     grow content (3s polls, WS keepalives, identical re-renders) do
 *     no scroll work — `scrollToBottom()` is a no-op when
 *     `container.scrollHeight === lastScrolledHeightRef.current`.
 *   - When OFF: no auto-scroll. If new content lands below the viewport,
 *     a "Jump to latest" pill appears; clicking it scrolls to bottom and
 *     re-enables the preference.
 *   - Auto-scroll flips OFF in two ways: (a) the toggle button (bottom
 *     right), and (b) explicit user scroll-up — if the user wheels or
 *     finger-drags upward by a cumulative ≥ USER_SCROLL_UNLOCK_PX (100px)
 *     within a single gesture. We listen to `wheel` and `touchmove`
 *     directly (not `scrollTop` deltas) so programmatic scrolls and
 *     content reflow above the viewport cannot trip the unlock.
 */
const EmbeddedSessionView = forwardRef<
  EmbeddedSessionViewHandle,
  EmbeddedSessionViewProps
>(({ sessionId, onScrollToBottom, autoScrollOnMount }, ref) => {
  const account = useAccount();
  const api = useApi();
  const lightTheme = useLightTheme();
  const containerRef = useRef<HTMLDivElement>(null);
  const queryClient = useQueryClient();
  const { NewInference } = useStreaming();

  // Global on/off preference for auto-scroll. Default ON.
  const [autoScroll, setAutoScroll] = useAutoScrollPreference();
  const autoScrollRef = useRef(autoScroll);
  useEffect(() => {
    autoScrollRef.current = autoScroll;
  }, [autoScroll]);

  // Opt-in surfaces (autoScrollOnMount) want the transcript to follow
  // live output regardless of a previously-persisted OFF value. Force
  // the preference ON once on mount. Runs once — after that the toggle
  // and scroll-up unlock behave normally.
  useEffect(() => {
    if (autoScrollOnMount && !autoScrollRef.current) {
      autoScrollRef.current = true;
      setAutoScroll(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // True when auto-scroll is OFF and new content has landed below the viewport.
  // Drives the "Jump to latest" pill.
  const [hasNewBelow, setHasNewBelow] = useState(false);

  // Last scrollHeight we wrote scrollTop to. Used to short-circuit
  // scrollToBottom() when nothing has actually grown since the last write —
  // otherwise InteractionLiveStream's onMessageUpdate (which fires on every
  // message/responseEntries reference change, throttled but ungated) would
  // trigger a redundant scroll write per polling interval and per WS update.
  const lastScrolledHeightRef = useRef(0);

  // Cumulative upward user-scroll distance (px) within the current gesture.
  // Flipped to autoScroll=OFF when this crosses USER_SCROLL_UNLOCK_PX.
  const upwardAccumRef = useRef(0);
  // Timestamp of the last wheel event, used to detect a fresh gesture and
  // reset the accumulator after a quiet gap.
  const lastWheelTsRef = useRef(0);
  // Touch state for tracking finger-drag direction.
  const touchStartYRef = useRef<number | null>(null);
  const lastTouchYRef = useRef<number | null>(null);

  // Pagination state: track which page we've loaded up to (page 0 = newest)
  const [oldestPageLoaded, setOldestPageLoaded] = useState(0);
  // Store older interactions loaded via pagination (newest first, so prepend older pages)
  const [olderInteractions, setOlderInteractions] = useState<TypesInteraction[]>([]);
  // Loading state for fetching older interactions
  const [isLoadingOlder, setIsLoadingOlder] = useState(false);

  // Returns true if the viewport is "near enough" the bottom that we treat
  // it as caught up (used to hide the jump-to-latest pill).
  const isNearBottom = useCallback(() => {
    const container = containerRef.current;
    if (!container) return true;
    const { scrollTop, scrollHeight, clientHeight } = container;
    return scrollTop + clientHeight >= scrollHeight - AUTO_SCROLL_NEAR_BOTTOM_PX;
  }, []);

  // Only used to clear the pill when the user scrolls back to the bottom.
  // We deliberately do NOT track "is the user at the bottom" for auto-scroll
  // decisions — auto-scroll is purely driven by the preference toggle.
  const handleScroll = useCallback(() => {
    if (autoScrollRef.current) return;
    if (isNearBottom()) setHasNewBelow(false);
  }, [isNearBottom]);

  // Scroll to bottom. Respects the auto-scroll preference unless `force` is set
  // (force is only used for initial mount, session change, and the
  // jump-to-latest pill click). Non-force calls also short-circuit when
  // scrollHeight hasn't changed since the last write — see lastScrolledHeightRef.
  const scrollToBottom = useCallback(
    (force = false) => {
      const container = containerRef.current;
      if (!container) return;
      if (!force && !autoScrollRef.current) return;
      if (!force && container.scrollHeight === lastScrolledHeightRef.current) return;
      container.scrollTop = container.scrollHeight;
      lastScrolledHeightRef.current = container.scrollHeight;
      setHasNewBelow(false);
      onScrollToBottom?.();
    },
    [onScrollToBottom],
  );

  // Click handler for the jump-to-latest pill: jump and re-enable auto-scroll.
  const handleJumpToLatest = useCallback(() => {
    setAutoScroll(true);
    autoScrollRef.current = true;
    scrollToBottom(true);
  }, [scrollToBottom, setAutoScroll]);

  // Expose scrollToBottom via ref for parent components
  useImperativeHandle(
    ref,
    () => ({
      scrollToBottom,
    }),
    [scrollToBottom],
  );

  // Fetch session data with auto-refresh.
  // Always poll session metadata at 3s, regardless of WS state.
  //
  // Earlier this was gated on `!wsConnected` to avoid HTTP polls racing
  // with WS-delivered data — but the WS only delivers interaction-related
  // events. The session's own metadata (in particular
  // `config.external_agent_status`) is never broadcast over the WS, so
  // suppressing polling left that field stale, breaking the
  // `useSandboxState` hook used by `ExternalAgentDesktopViewer` to render
  // the "Starting Desktop..." spinner during boot. See incident
  // 2026-04-25 with ses_01kq0ba2708rawbsfqv2hyyxp2.
  //
  // We've also confirmed the original race concern is mitigated by
  // `streaming.tsx:296-308`, which explicitly preserves the existing
  // `config` when applying WS-delivered session updates. So polling can't
  // overwrite a fresher WS value because the WS never updates `config` in
  // the first place.
  const { data: sessionResponse, refetch: refetchSession } = useGetSession(
    sessionId,
    {
      enabled: !!sessionId,
      refetchInterval: 3000,
      skipInteractions: true,
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
    { enabled: !!sessionId, refetchInterval: 3000 }
  );
  const paginatedData = paginatedInteractionsResponse?.data;

  // Fetch session steps
  const { data: sessionSteps } = useListSessionSteps(sessionId, {
    enabled: !!sessionId,
  });

  // The inner content Box is observed by ResizeObserver so we only react
  // to *actual* content size changes, not every React re-render. Using a
  // state-mirrored callback ref (NOT a plain useRef) so the ResizeObserver
  // useEffect can re-run when the element actually mounts — necessary
  // because EmbeddedSessionView has early returns (loading state when
  // !session, empty state when no interactions) that render before the
  // JSX containing this ref. A plain useRef + stable-deps useEffect runs
  // once with `current === null` and never re-runs when the content
  // finally mounts; the callback-ref state flip forces the re-run.
  const [contentEl, setContentEl] = useState<HTMLDivElement | null>(null);
  const setContentRef = useCallback((el: HTMLDivElement | null) => {
    setContentEl(el);
  }, []);
  // Last observed content height. 0 until the first ResizeObserver callback.
  const lastContentHeightRef = useRef(0);
  // True once we've forced an initial scroll-to-bottom for this session.
  // Reset on session change.
  const hasInitiallyScrolled = useRef(false);

  // Reset state and clear stale cache when sessionId changes.
  const prevSessionIdRef = useRef(sessionId);
  useEffect(() => {
    if (sessionId !== prevSessionIdRef.current) {
      const oldSessionId = prevSessionIdRef.current;
      prevSessionIdRef.current = sessionId;

      hasInitiallyScrolled.current = false;
      lastContentHeightRef.current = 0;
      lastScrolledHeightRef.current = 0;
      upwardAccumRef.current = 0;
      lastWheelTsRef.current = 0;
      touchStartYRef.current = null;
      lastTouchYRef.current = null;
      setHasNewBelow(false);
      setOldestPageLoaded(0);
      setOlderInteractions([]);

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

  // Force-scroll to the bottom on first content render for a session, even if
  // auto-scroll is OFF — opening a session should land you on the latest
  // message.
  useEffect(() => {
    if (
      paginatedData?.interactions &&
      paginatedData.interactions.length > 0 &&
      !hasInitiallyScrolled.current
    ) {
      hasInitiallyScrolled.current = true;
      // setTimeout (vs RAF) gives markdown / code highlighting time to render
      // so the scroll lands on truly-final content.
      setTimeout(() => {
        scrollToBottom(true);
      }, 100);
    }
  }, [paginatedData?.interactions?.length, scrollToBottom]);

  // ResizeObserver-driven auto-scroll: only fires when the content's actual
  // size changes. Renders that don't grow content (e.g., the 3s React Query
  // poll returning identical data) do no scroll work at all.
  //
  // The dep array includes `contentEl` so the effect re-runs when the
  // content element first mounts after a loading-state early return.
  useEffect(() => {
    const container = containerRef.current;
    if (!container || !contentEl) return;

    const observer = new ResizeObserver((entries) => {
      const newHeight = entries[0]?.contentRect.height ?? 0;
      const prevHeight = lastContentHeightRef.current;
      lastContentHeightRef.current = newHeight;

      // First measurement after mount/session-reset: just record it. The
      // initial-scroll effect handles getting us to the bottom.
      if (prevHeight === 0) return;
      // Only react to growth; shrinking (e.g., a tool call collapsing) shouldn't
      // yank the viewport.
      if (newHeight <= prevHeight) return;

      if (autoScrollRef.current) {
        container.scrollTop = container.scrollHeight;
        lastScrolledHeightRef.current = container.scrollHeight;
        setHasNewBelow(false);
      } else if (!isNearBottom()) {
        setHasNewBelow(true);
      }
    });

    observer.observe(contentEl);
    return () => observer.disconnect();
  }, [contentEl, isNearBottom]);

  // Detect explicit user scroll-up and flip auto-scroll OFF when the user
  // accumulates >= USER_SCROLL_UNLOCK_PX upward within a single gesture.
  //
  // We listen to wheel and touch input events directly — NOT to scrollTop
  // deltas — because:
  //   * wheel/touchmove only fire from real user input; programmatic
  //     scrollTop writes don't synthesize them.
  //   * content reflow above the viewport (image loads, syntax highlight)
  //     shifts scrollTop without any input event.
  // This sidesteps the three race surfaces that killed the previous
  // sticky-scroll attempt (see commit 42c3a5112 for the autopsy).
  //
  // Listeners are wired via React's synthetic event props on the container
  // JSX (NOT a useEffect) because the component renders early-return
  // loading/empty states before the container exists. A useEffect with a
  // stable dep array would run once with `containerRef.current === null`
  // and never re-attach when the container later mounts. React's prop
  // wiring guarantees attachment whenever the container actually renders.
  const triggerUnlock = useCallback(() => {
    setAutoScroll(false);
    autoScrollRef.current = false;
    upwardAccumRef.current = 0;
  }, [setAutoScroll]);

  const handleWheel = useCallback(
    (e: React.WheelEvent<HTMLDivElement>) => {
      if (!autoScrollRef.current) return;
      const now = performance.now();
      // New gesture if the previous wheel event was long enough ago.
      if (now - lastWheelTsRef.current > USER_SCROLL_GESTURE_TIMEOUT_MS) {
        upwardAccumRef.current = 0;
      }
      lastWheelTsRef.current = now;
      if (e.deltaY < 0) {
        upwardAccumRef.current += -e.deltaY;
        if (upwardAccumRef.current >= USER_SCROLL_UNLOCK_PX) triggerUnlock();
      } else if (e.deltaY > 0) {
        // Direction change resets the accumulator — scrolling down clearly
        // signals the user does NOT want to disengage auto-scroll.
        upwardAccumRef.current = 0;
      }
    },
    [triggerUnlock],
  );

  const handleTouchStart = useCallback((e: React.TouchEvent<HTMLDivElement>) => {
    if (!autoScrollRef.current) return;
    const t = e.touches[0];
    if (!t) return;
    touchStartYRef.current = t.clientY;
    lastTouchYRef.current = t.clientY;
    upwardAccumRef.current = 0;
  }, []);

  const handleTouchMove = useCallback(
    (e: React.TouchEvent<HTMLDivElement>) => {
      if (!autoScrollRef.current) return;
      const t = e.touches[0];
      if (!t || lastTouchYRef.current === null) return;
      // Finger moving DOWN the screen (clientY increases) scrolls the
      // content UP — that's the gesture we treat as "user wants to read
      // history."
      const dy = t.clientY - lastTouchYRef.current;
      lastTouchYRef.current = t.clientY;
      if (dy > 0) {
        upwardAccumRef.current += dy;
        if (upwardAccumRef.current >= USER_SCROLL_UNLOCK_PX) triggerUnlock();
      } else if (dy < 0) {
        upwardAccumRef.current = 0;
      }
    },
    [triggerUnlock],
  );

  const handleTouchEnd = useCallback(() => {
    touchStartYRef.current = null;
    lastTouchYRef.current = null;
    upwardAccumRef.current = 0;
  }, []);

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
  const totalPages = paginatedData?.totalPages || 1;
  const totalCount = paginatedData?.totalCount || 0;
  const hasOlderInteractions = oldestPageLoaded < totalPages - 1;
  const remainingOlderCount = Math.max(0, totalCount - totalInteractions);

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
      sx={{
        flex: 1,
        minHeight: 0,
        position: "relative",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <Box
        ref={containerRef}
        onScroll={handleScroll}
        onWheel={handleWheel}
        onTouchStart={handleTouchStart}
        onTouchMove={handleTouchMove}
        onTouchEnd={handleTouchEnd}
        onTouchCancel={handleTouchEnd}
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
        <Box
          ref={setContentRef}
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
              {isLoadingOlder ? "Loading..." : `Show ${remainingOlderCount} older messages`}
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

      {/* Auto-scroll toggle (bottom-right) — stark filled/outlined treatment
          so the on/off state is visible at a glance. */}
      <Tooltip
        title={
          autoScroll
            ? "Auto-scroll is on. Click to pause."
            : "Auto-scroll is paused. Click to resume."
        }
        placement="left"
      >
        <IconButton
          size="small"
          onClick={() => {
            const next = !autoScroll;
            setAutoScroll(next);
            autoScrollRef.current = next;
            if (next) scrollToBottom(true);
          }}
          aria-label={autoScroll ? "Pause auto-scroll" : "Resume auto-scroll"}
          aria-pressed={autoScroll}
          sx={{
            position: "absolute",
            bottom: 8,
            right: 12,
            zIndex: 2,
            transition: "background-color 0.15s, color 0.15s, box-shadow 0.15s, opacity 0.15s",
            ...(autoScroll
              ? {
                  // ON: filled, primary, prominent
                  backgroundColor: "primary.main",
                  color: "primary.contrastText",
                  boxShadow: 2,
                  border: "none",
                  "&:hover": {
                    backgroundColor: "primary.dark",
                  },
                }
              : {
                  // OFF: outlined ghost, dimmed
                  backgroundColor: "background.paper",
                  color: "text.secondary",
                  border: 1,
                  borderColor: "divider",
                  boxShadow: "none",
                  opacity: 0.65,
                  "&:hover": {
                    backgroundColor: "action.hover",
                    opacity: 1,
                  },
                }),
          }}
        >
          {autoScroll ? (
            <VerticalAlignBottomIcon fontSize="small" />
          ) : (
            <PauseIcon fontSize="small" />
          )}
        </IconButton>
      </Tooltip>

      {/* Jump-to-latest pill (bottom-center, only when auto-scroll OFF and
          new content has arrived below the viewport) */}
      {!autoScroll && hasNewBelow && (
        <Button
          variant="contained"
          size="small"
          startIcon={<KeyboardDoubleArrowDownIcon />}
          onClick={handleJumpToLatest}
          sx={{
            position: "absolute",
            bottom: 12,
            left: "50%",
            transform: "translateX(-50%)",
            zIndex: 3,
            textTransform: "none",
            borderRadius: 999,
            boxShadow: 3,
          }}
        >
          Jump to latest
        </Button>
      )}
    </Box>
  );
});

export default EmbeddedSessionView;
