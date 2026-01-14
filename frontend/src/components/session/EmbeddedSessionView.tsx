import React, { FC, useState, useEffect, useRef, useMemo, useCallback, forwardRef, useImperativeHandle } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'

import Interaction from './Interaction'
import InteractionLiveStream from './InteractionLiveStream'

import useAccount from '../../hooks/useAccount'
import { useGetSession, useListSessionSteps } from '../../services/sessionService'
import { useStreaming } from '../../contexts/streaming'
import { TypesInteractionState, TypesSession } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { SESSION_TYPE_TEXT } from '../../types'

interface EmbeddedSessionViewProps {
  sessionId: string
  /** Called when a new message is submitted (streaming is handled by parent) */
  onScrollToBottom?: () => void
}

export interface EmbeddedSessionViewHandle {
  scrollToBottom: () => void
}

/**
 * EmbeddedSessionView - A lightweight session message thread viewer
 *
 * This component renders the chat message thread for a session, designed to be
 * embedded in dialogs or panels where the full Session page would be too heavy.
 *
 * Unlike the full Session page, this component:
 * - Does not include the input box (handled by parent)
 * - Does not include the session toolbar
 * - Uses simpler virtualization (shows last N interactions)
 * - Auto-scrolls to bottom on new messages
 */
const EmbeddedSessionView = forwardRef<EmbeddedSessionViewHandle, EmbeddedSessionViewProps>(({
  sessionId,
  onScrollToBottom,
}, ref) => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const containerRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const { NewInference } = useStreaming()

  // Smart scroll state - use refs to avoid re-renders on scroll
  const isAtBottomRef = useRef(true) // Start at bottom
  const userScrolledUpRef = useRef(false) // Track if user explicitly scrolled up
  const lastScrollTopRef = useRef(0) // Track scroll direction
  const isResizingRef = useRef(false) // Track resize events
  const SCROLL_THRESHOLD = 50 // Pixels from bottom to consider "at bottom"

  // Check if currently at bottom
  const checkIsAtBottom = useCallback(() => {
    const container = containerRef.current
    if (!container) return true
    const { scrollTop, scrollHeight, clientHeight } = container
    return scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD
  }, [])

  // Handle scroll events to track user scroll intent
  const handleScroll = useCallback(() => {
    const container = containerRef.current
    if (!container) return

    // Ignore scroll events during resize
    if (isResizingRef.current) return

    const { scrollTop } = container
    const wasAtBottom = isAtBottomRef.current
    const isNowAtBottom = checkIsAtBottom()

    // Detect scroll direction
    const scrolledUp = scrollTop < lastScrollTopRef.current

    // If user scrolled up from the bottom, mark as user-initiated scroll up
    if (scrolledUp && wasAtBottom && !isNowAtBottom) {
      userScrolledUpRef.current = true
    }

    // If user scrolled back to bottom, re-enable auto-scroll
    if (isNowAtBottom) {
      userScrolledUpRef.current = false
    }

    isAtBottomRef.current = isNowAtBottom
    lastScrollTopRef.current = scrollTop
  }, [checkIsAtBottom])

  // Handle container resize - don't count as user scroll, but do scroll to bottom if we were at bottom
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const resizeObserver = new ResizeObserver(() => {
      isResizingRef.current = true

      // If we were at bottom before resize and user hasn't scrolled up, scroll to bottom
      if (!userScrolledUpRef.current) {
        requestAnimationFrame(() => {
          if (containerRef.current) {
            containerRef.current.scrollTop = containerRef.current.scrollHeight
            isAtBottomRef.current = true
          }
          isResizingRef.current = false
        })
      } else {
        isResizingRef.current = false
      }
    })

    resizeObserver.observe(container)
    return () => resizeObserver.disconnect()
  }, [])

  // Watch for content height changes (tool calls, streaming updates, etc.)
  // This handles the case where content grows within an existing interaction
  useEffect(() => {
    const content = contentRef.current
    if (!content) return

    const resizeObserver = new ResizeObserver(() => {
      // If user hasn't scrolled up, scroll to bottom when content grows
      if (!userScrolledUpRef.current) {
        requestAnimationFrame(() => {
          if (containerRef.current) {
            containerRef.current.scrollTop = containerRef.current.scrollHeight
            isAtBottomRef.current = true
          }
        })
      }
    })

    resizeObserver.observe(content)
    return () => resizeObserver.disconnect()
  }, [])

  // Fetch session data with auto-refresh
  const { data: sessionResponse, refetch: refetchSession } = useGetSession(sessionId, {
    enabled: !!sessionId,
    refetchInterval: 2000, // Auto-refresh every 2 seconds
  })

  const session = sessionResponse?.data

  // Debug logging
  useEffect(() => {
    console.log('[EmbeddedSessionView] Session data:', {
      sessionId,
      hasSession: !!session,
      interactionsCount: session?.interactions?.length || 0,
      interactions: session?.interactions?.map(i => ({
        id: i.id,
        state: i.state,
        prompt_message: i.prompt_message?.substring(0, 50),
        response_message: i.response_message?.substring(0, 50),
        display_message: i.display_message?.substring(0, 50),
      })),
      serverConfigFilestore: account.serverConfig?.filestore_prefix,
    })
  }, [sessionId, session, account.serverConfig])

  // Fetch session steps
  const { data: sessionSteps } = useListSessionSteps(sessionId, {
    enabled: !!sessionId,
  })

  // Track if we're streaming (last interaction is in waiting state)
  const isStreaming = useMemo(() => {
    if (!session?.interactions || session.interactions.length === 0) return false
    const lastInteraction = session.interactions[session.interactions.length - 1]
    return lastInteraction.state === TypesInteractionState.InteractionStateWaiting
  }, [session?.interactions])

  // Auto-scroll to bottom when new messages arrive or when streaming
  // Only scrolls if user hasn't explicitly scrolled up
  const scrollToBottom = useCallback((force = false) => {
    if (!containerRef.current) return

    // Don't auto-scroll if user has explicitly scrolled up (unless forced)
    if (!force && userScrolledUpRef.current) return

    // Use requestAnimationFrame to ensure DOM has updated
    requestAnimationFrame(() => {
      if (!containerRef.current) return
      containerRef.current.scrollTop = containerRef.current.scrollHeight
      isAtBottomRef.current = true
    })

    onScrollToBottom?.()
  }, [onScrollToBottom])

  // Expose scrollToBottom via ref for parent components
  useImperativeHandle(ref, () => ({
    scrollToBottom,
  }), [scrollToBottom])

  // Scroll to bottom when interactions change
  useEffect(() => {
    if (session?.interactions) {
      scrollToBottom()
    }
  }, [session?.interactions?.length, scrollToBottom])

  // Scroll to bottom on initial mount (with slight delay to ensure DOM is ready)
  // Force scroll on initial mount - user hasn't had a chance to scroll yet
  const hasInitiallyScrolledRef = useRef(false)
  useEffect(() => {
    if (session?.interactions && session.interactions.length > 0 && !hasInitiallyScrolledRef.current) {
      hasInitiallyScrolledRef.current = true
      // Use a small timeout to ensure the container and content are fully rendered
      const timeoutId = setTimeout(() => {
        scrollToBottom(true) // Force scroll on initial mount
      }, 100)
      return () => clearTimeout(timeoutId)
    }
  }, [session?.interactions, scrollToBottom])

  // Reload session handler
  const handleReloadSession = useCallback(async () => {
    await refetchSession()
    return session
  }, [refetchSession, session])

  // Regenerate handler - required for InteractionInference to render messages
  const handleRegenerate = useCallback(async (interactionID: string, message: string) => {
    if (!session) return

    // For now, just re-send the message as a new inference
    // A more sophisticated implementation would use the regenerate API
    await NewInference({
      message: message,
      sessionId: sessionId,
      type: SESSION_TYPE_TEXT,
    })

    scrollToBottom()
  }, [session, sessionId, NewInference, scrollToBottom])

  // Show loading state while fetching session
  if (!session) {
    return (
      <Box
        sx={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexDirection: 'column',
          gap: 2,
        }}
      >
        <CircularProgress size={32} />
        <Typography variant="body2" color="text.secondary">
          Loading session...
        </Typography>
      </Box>
    )
  }

  // Show empty state if no interactions
  if (!session.interactions || session.interactions.length === 0) {
    return (
      <Box
        sx={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Typography variant="body2" color="text.secondary">
          No messages yet. Send a message to start the conversation.
        </Typography>
      </Box>
    )
  }

  const isOwner = account.user?.id === session.owner

  return (
    <Box
      ref={containerRef}
      onScroll={handleScroll}
      sx={{
        flex: 1,
        overflow: 'auto',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
        ...lightTheme.scrollbar,
      }}
    >
      <Box
        ref={contentRef}
        sx={{
          width: '100%',
          maxWidth: 700,
          mx: 'auto',
          px: 2,
          py: 2,
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
        }}
      >
        {session.interactions.map((interaction, index) => {
          const isLastInteraction = index === session.interactions!.length - 1
          const isLive = isLastInteraction && interaction.state === TypesInteractionState.InteractionStateWaiting

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
          )
        })}
      </Box>
    </Box>
  )
})

export default EmbeddedSessionView
