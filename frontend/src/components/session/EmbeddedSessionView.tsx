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
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import KeyboardDoubleArrowDownIcon from "@mui/icons-material/KeyboardDoubleArrowDown";
import { useQueryClient } from "@tanstack/react-query";
import {
  AUTO_SCROLL_NEAR_BOTTOM_PX,
} from "../../hooks/useAutoScrollPreference";

// Number of interactions to render initially (and per "load more" click).
// Keep this low — long-running agent sessions can have interactions with
// hundreds of entries, each rendered as a Markdown component.
const INTERACTIONS_TO_RENDER = 5;

// Keys that scroll the transcript without producing a wheel or pointer event.
const SCROLL_KEYS = new Set([
  "ArrowUp",
  "ArrowDown",
  "PageUp",
  "PageDown",
  "Home",
  "End",
  " ",
]);

// How long after a pointer release touch momentum may still be scrolling.
const POINTER_SCROLL_SETTLE_MS = 400;

import Interaction from "./Interaction";
import InteractionLiveStream from "./InteractionLiveStream";
import PausedBanner from "./PausedBanner";
import ForkBadge from "./ForkBadge";

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
import { getChatColors } from "./chatStyles";
import ChatTurnNavigator from "./ChatTurnNavigator";
import {
  ChatTurnNavigatorItem,
  compactChatTurnPreview,
  resolveChatTurnAssistantPreview,
} from "./ChatTurnNavigator.logic";
import { splitSystemPrefix } from "./CollapsibleSystemPrefix";
import { isSandboxOffline } from "../external-agent/sandboxState";

interface EmbeddedSessionViewProps {
  sessionId: string;
  onScrollToBottom?: () => void;
  enableInteractionDebugCopy?: boolean;
}

export interface EmbeddedSessionViewHandle {
  scrollToBottom: () => void;
}

/**
 * EmbeddedSessionView - session message thread viewer.
 *
 * Auto-scroll model:
 *
 *   - New content follows the viewport while it is at the bottom.
 *   - Scrolling above the bottom pauses follow mode immediately.
 *   - Returning to the bottom resumes follow mode automatically.
 *   - If content arrives while paused, a "Jump to latest" pill appears.
 */
const EmbeddedSessionView = forwardRef<
  EmbeddedSessionViewHandle,
  EmbeddedSessionViewProps
>(({ sessionId, onScrollToBottom, enableInteractionDebugCopy }, ref) => {
  const account = useAccount();
  const api = useApi();
  const lightTheme = useLightTheme();
  const containerRef = useRef<HTMLDivElement>(null);
  const [scrollContainerEl, setScrollContainerEl] = useState<HTMLDivElement | null>(null);
  const setScrollContainerRef = useCallback((el: HTMLDivElement | null) => {
    containerRef.current = el;
    setScrollContainerEl(el);
  }, []);
  const queryClient = useQueryClient();
  const { NewInference } = useStreaming();

  // Whether content growth should keep the viewport pinned to the bottom.
  // This is derived from user scroll intent, not a persisted preference.
  // Layout changes can move the bottom without the user scrolling (notably
  // when RobustPromptInput expands to show its queue), so scroll position
  // alone is not sufficient to decide that follow mode should stop.
  const shouldFollowLatestRef = useRef(true);
  const isPointerScrollingRef = useRef(false);

  // True when new content has landed below a viewport that is away from the
  // latest message. Drives the "Jump to latest" pill.
  const [hasNewBelow, setHasNewBelow] = useState(false);

  // Last scrollHeight we wrote scrollTop to. Used to short-circuit
  // scrollToBottom() when nothing has actually grown since the last write —
  // otherwise InteractionLiveStream's onMessageUpdate (which fires on every
  // message/responseEntries reference change, throttled but ungated) would
  // trigger a redundant scroll write per polling interval and per WS update.
  const lastScrolledHeightRef = useRef(0);

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

  // Returning to the bottom always restores follow mode. Moving away only
  // pauses while there is explicit user input; flex/layout changes can also
  // emit scroll events and must not be mistaken for user navigation.
  const handleScroll = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;

    if (isNearBottom()) {
      shouldFollowLatestRef.current = true;
      setHasNewBelow(false);
    } else if (isPointerScrollingRef.current) {
      shouldFollowLatestRef.current = false;
    }
  }, [isNearBottom]);

  const handleWheel = useCallback((event: React.WheelEvent<HTMLDivElement>) => {
    if (event.deltaY < 0) {
      shouldFollowLatestRef.current = false;
    }
  }, []);

  const handlePointerDown = useCallback(() => {
    isPointerScrollingRef.current = true;
  }, []);

  const handlePointerUp = useCallback(() => {
    // Touch momentum keeps emitting scroll events after the finger lifts, so
    // hold the intent open long enough for the fling to be attributed to the
    // user rather than to a layout change.
    window.setTimeout(() => {
      isPointerScrollingRef.current = false;
    }, POINTER_SCROLL_SETTLE_MS);
  }, []);

  // Keyboard scrolling emits no wheel or pointer events, so without this the
  // viewport would snap back to the bottom as soon as new content arrived.
  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (SCROLL_KEYS.has(event.key)) {
      isPointerScrollingRef.current = true;
    }
  }, []);

  // Scroll to bottom. `force` is used for initial mount, session changes, and
  // the jump-to-latest pill. Other callers only follow when the user is at the
  // bottom. Non-force calls also short-circuit when scrollHeight hasn't changed
  // since the last write.
  const scrollToBottom = useCallback(
    (force = false) => {
      const container = containerRef.current;
      if (!container) return;
      if (!force && !shouldFollowLatestRef.current) return;
      if (
        !force &&
        container.scrollHeight === lastScrolledHeightRef.current &&
        isNearBottom()
      ) return;
      container.scrollTop = container.scrollHeight;
      lastScrolledHeightRef.current = container.scrollHeight;
      shouldFollowLatestRef.current = true;
      setHasNewBelow(false);
      onScrollToBottom?.();
    },
    [isNearBottom, onScrollToBottom],
  );

  // Click handler for the jump-to-latest pill: jump and re-enable auto-scroll.
  const handleJumpToLatest = useCallback(() => {
    shouldFollowLatestRef.current = true;
    scrollToBottom(true);
  }, [scrollToBottom]);

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
  const { data: sessionResponse, refetch: refetchSession, error: sessionError } = useGetSession(
    sessionId,
    {
      enabled: !!sessionId,
      // Stop polling once the session errors — a 403/404 won't fix itself
      // by re-asking every 3s, and the perpetual poll is what made a
      // forbidden session hang the view forever instead of showing why.
      refetchInterval: (query: any) => (query.state.error ? false : 3000),
      skipInteractions: true,
    },
  );

  const session = sessionResponse?.data;
  // HTTP status of a failed session fetch, if any. Drives the error state
  // below so a forbidden / missing session degrades gracefully instead of
  // spinning on "Loading session…" forever.
  const sessionErrorStatus = (sessionError as any)?.response?.status as number | undefined;

  // Whether this session's sandbox has stopped. Derived from the session we
  // already poll above rather than via useSandboxState, so we don't open a
  // second query against the same row with different parameters.
  const agentOffline = useMemo(
    () => isSandboxOffline(session?.config),
    [session?.config],
  );

  // Fetch paginated interactions (newest first via order=desc)
  // Page 0 = newest interactions, higher pages = older interactions.
  // Disabled once the session fetch has errored (403/404) — there's no
  // point polling interactions for a session we can't read, and leaving
  // it on kept 403ing every 3s in the background after the session poll
  // had already stopped.
  const { data: paginatedInteractionsResponse } = useListInteractions(
    sessionId,
    0, // Always fetch page 0 (newest) - older pages fetched on demand
    INTERACTIONS_TO_RENDER,
    'desc',
    { enabled: !!sessionId && !sessionErrorStatus, refetchInterval: 3000 }
  );
  const paginatedData = paginatedInteractionsResponse?.data;

  // Fetch session steps (also gated on a readable session — see above).
  const { data: sessionSteps } = useListSessionSteps(sessionId, {
    enabled: !!sessionId && !sessionErrorStatus,
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

  // Keep a followed transcript pinned when its viewport changes height. The
  // composer queue is outside this scroll container, so opening it shrinks the
  // viewport without changing the transcript's scrollHeight.
  useEffect(() => {
    if (!scrollContainerEl) return;

    let previousHeight = scrollContainerEl.clientHeight;
    const observer = new ResizeObserver((entries) => {
      const nextHeight = entries[0]?.contentRect.height ?? scrollContainerEl.clientHeight;
      if (nextHeight === previousHeight) return;
      previousHeight = nextHeight;

      if (!shouldFollowLatestRef.current) return;
      scrollContainerEl.scrollTop = scrollContainerEl.scrollHeight;
      lastScrolledHeightRef.current = scrollContainerEl.scrollHeight;
      setHasNewBelow(false);
    });

    observer.observe(scrollContainerEl);
    return () => observer.disconnect();
  }, [scrollContainerEl]);

  // Reset state and clear stale cache when sessionId changes.
  const prevSessionIdRef = useRef(sessionId);
  useEffect(() => {
    if (sessionId !== prevSessionIdRef.current) {
      const oldSessionId = prevSessionIdRef.current;
      prevSessionIdRef.current = sessionId;

      hasInitiallyScrolled.current = false;
      lastContentHeightRef.current = 0;
      lastScrolledHeightRef.current = 0;
      shouldFollowLatestRef.current = true;
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

  // Opening a session should land on the latest message regardless of the
  // previous session's scroll position.
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

      // Disclosure bodies grow below their clicked header. Do not treat that
      // operator action as new chat output and yank the viewport to the bottom.
      if (container.dataset.preserveDisclosureExpansion === "true") {
        delete container.dataset.preserveDisclosureExpansion;
        return;
      }

      if (shouldFollowLatestRef.current) {
        container.scrollTop = container.scrollHeight;
        lastScrolledHeightRef.current = container.scrollHeight;
        shouldFollowLatestRef.current = true;
        setHasNewBelow(false);
      } else if (!isNearBottom()) {
        setHasNewBelow(true);
      }
    });

    observer.observe(contentEl);
    return () => observer.disconnect();
  }, [contentEl, isNearBottom]);

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

  const navigatorItems = useMemo<ChatTurnNavigatorItem[]>(() => {
    return visibleInteractions.flatMap((interaction) => {
      if (!interaction.id || interaction.trigger === "fork_seed" || interaction.trigger === "fork_handoff") return [];
      const contentText = interaction.prompt_message_content?.parts?.find(
        (part): part is { text: string } =>
          typeof part === "object" &&
          part !== null &&
          "text" in part &&
          typeof part.text === "string",
      )?.text;
      const rawUserText = interaction.display_message || interaction.prompt_message || contentText;
      const splitUserText = splitSystemPrefix(rawUserText || "");
      const userText = compactChatTurnPreview(
        splitUserText.prefix
          ? splitUserText.userText ||
            (splitUserText.kind === "approval"
              ? "Spec approved · Implementation instructions"
              : splitUserText.label || "Planning Instructions")
          : rawUserText,
      );
      if (!userText) return [];
      return [{
        id: interaction.id,
        userText,
        assistantText: resolveChatTurnAssistantPreview(
          interaction.response_message,
          interaction.response_entries as unknown as Array<{ type?: string; content?: string }>,
        ),
      }];
    });
  }, [visibleInteractions]);

  const totalInteractions = visibleInteractions.length;

  // Check if there are more pages to load
  const totalPages = paginatedData?.totalPages || 1;
  const totalCount = paginatedData?.totalCount || 0;
  const hasOlderInteractions = oldestPageLoaded < totalPages - 1;
  const remainingOlderCount = Math.max(0, totalCount - totalInteractions);

  const isOwner = account.user?.id === session?.owner;

  const handleNavigateToTurn = useCallback((item: ChatTurnNavigatorItem) => {
    const container = containerRef.current;
    if (!container) return;
    const target = Array.from(
      container.querySelectorAll<HTMLElement>("[data-chat-turn]"),
    ).find((element) => element.dataset.chatTurn === item.id);
    if (!target) return;

    shouldFollowLatestRef.current = false;
    const viewport = container.getBoundingClientRect();
    const targetRect = target.getBoundingClientRect();
    container.scrollTo({
      top: container.scrollTop + targetRect.top - viewport.top - 24,
      behavior: "smooth",
    });
    const targetIndex = navigatorItems.findIndex((candidate) => candidate.id === item.id);
    setHasNewBelow(targetIndex >= 0 && targetIndex < navigatorItems.length - 1);
  }, [navigatorItems]);

  // Show loading state while fetching session
  // Error state — a failed session fetch (no data) degrades to a clear
  // message instead of a perpetual "Loading session…" spinner. The poll is
  // already stopped (refetchInterval returns false on error), so this is
  // terminal until the inputs change.
  if (!session && sessionErrorStatus) {
    const message =
      sessionErrorStatus === 403
        ? "You don't have access to this conversation."
        : sessionErrorStatus === 404
          ? "This conversation is no longer available."
          : "This conversation couldn't be loaded.";
    return (
      <Box
        sx={{
          flex: 1,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexDirection: "column",
          gap: 1,
          p: 3,
          textAlign: "center",
        }}
      >
        <Typography variant="body2" color="text.secondary">
          {message}
        </Typography>
      </Box>
    );
  }

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
        minWidth: 0,
        width: "100%",
        position: "relative",
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
        backgroundColor: (theme) => getChatColors(theme).canvas,
      }}
    >
      {session && (
        <>
          <PausedBanner session={session} />
          {session.config?.parent_session_id && (
            <Box sx={{ px: 2, pt: 1.5, display: "flex", justifyContent: "flex-start" }}>
              <ForkBadge session={session} />
            </Box>
          )}
        </>
      )}
      <Box
        ref={setScrollContainerRef}
        data-session-scroll-container
        onScroll={handleScroll}
        onWheel={handleWheel}
        onKeyDown={handleKeyDown}
        onPointerDown={handlePointerDown}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        sx={{
          // Use height: 0 + flex: 1 to force this to be the scrollable container
          // Without height: 0, the container may expand to fit content on iOS
          height: 0,
          flex: 1,
          overflowY: "auto",
          overflowX: "hidden",
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
            maxWidth: 768,
            mx: "auto",
            pl: { xs: 1.5, sm: 2.5 },
            pr: { xs: 1.5, sm: 2.5 },
            "@media (pointer: fine)": {
              pl: 4.5,
            },
            py: 2.5,
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
                nextInteraction={visibleInteractions[index + 1]}
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
                enableDebugCopy={enableInteractionDebugCopy}
              >
                {isLive && (isOwner || account.admin) && (
                  <InteractionLiveStream
                    session_id={sessionId}
                    interaction={interaction}
                    session={session}
                    agentOffline={agentOffline}
                    serverConfig={account.serverConfig}
                    onMessageUpdate={scrollToBottom}
                  />
                )}
              </Interaction>
            );
          })}
        </Box>
      </Box>

      <ChatTurnNavigator
        items={navigatorItems}
        scrollContainer={scrollContainerEl}
        onSelect={handleNavigateToTurn}
      />

      {/* Jump-to-latest pill (bottom-center, only when content arrived while
          the user was reading above the latest message) */}
      {hasNewBelow && (
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
            px: 1.5,
            py: 0.65,
            minHeight: 32,
            fontWeight: 600,
            backgroundColor: lightTheme.isDark ? "#000000" : "#ffffff",
            color: lightTheme.isDark ? "#ffffff" : "#111111",
            border: `1px solid ${lightTheme.isDark ? "rgba(255,255,255,0.72)" : "rgba(0,0,0,0.62)"}`,
            boxShadow: lightTheme.isDark
              ? "0 2px 10px rgba(0,0,0,0.45)"
              : "0 2px 10px rgba(0,0,0,0.18)",
            "&:hover": {
              backgroundColor: lightTheme.isDark ? "#111111" : "#f3f3f3",
              borderColor: lightTheme.isDark ? "#ffffff" : "#000000",
              boxShadow: lightTheme.isDark
                ? "0 3px 12px rgba(0,0,0,0.55)"
                : "0 3px 12px rgba(0,0,0,0.24)",
            },
          }}
        >
          Jump to latest
        </Button>
      )}
    </Box>
  );
});

export default EmbeddedSessionView;
