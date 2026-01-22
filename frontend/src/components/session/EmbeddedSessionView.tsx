import React, { useEffect, useRef, useMemo, useCallback, forwardRef, useImperativeHandle, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'

// DEBUG: Temporary debug overlay to diagnose scroll issues on iOS Safari
const DEBUG_SCROLL = true

import Interaction from './Interaction'
import InteractionLiveStream from './InteractionLiveStream'

import useAccount from '../../hooks/useAccount'
import { useGetSession, useListSessionSteps } from '../../services/sessionService'
import { useStreaming } from '../../contexts/streaming'
import { TypesInteractionState } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { SESSION_TYPE_TEXT } from '../../types'

interface EmbeddedSessionViewProps {
  sessionId: string
  onScrollToBottom?: () => void
}

export interface EmbeddedSessionViewHandle {
  scrollToBottom: () => void
}

/**
 * EmbeddedSessionView - A lightweight session message thread viewer
 *
 * Simple sticky-scroll behavior:
 * - If you're at the bottom, stay at the bottom as content grows
 * - If you scroll up, stay where you are
 * - If you scroll back to bottom, resume auto-scroll
 */
const EmbeddedSessionView = forwardRef<EmbeddedSessionViewHandle, EmbeddedSessionViewProps>(({
  sessionId,
  onScrollToBottom,
}, ref) => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const containerRef = useRef<HTMLDivElement>(null)
  const { NewInference } = useStreaming()

  // Simple scroll state: are we currently at the bottom?
  // This is the ONLY state we need for sticky scroll behavior
  const isAtBottomRef = useRef(true)
  const SCROLL_THRESHOLD = 50

  // DEBUG: State for debug overlay
  const [debugInfo, setDebugInfo] = useState({
    isAtBottom: true,
    scrollTop: 0,
    scrollHeight: 0,
    clientHeight: 0,
    lastEvent: 'init',
    mutationCount: 0,
    scrollCount: 0,
  })

  // Check if currently at bottom
  const checkIsAtBottom = useCallback(() => {
    const container = containerRef.current
    if (!container) return true
    const { scrollTop, scrollHeight, clientHeight } = container
    return scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD
  }, [])

  // Update isAtBottom on every scroll event
  const handleScroll = useCallback(() => {
    const container = containerRef.current
    if (!container) return

    isAtBottomRef.current = checkIsAtBottom()

    if (DEBUG_SCROLL) {
      setDebugInfo(prev => ({
        ...prev,
        isAtBottom: isAtBottomRef.current,
        scrollTop: Math.round(container.scrollTop),
        scrollHeight: container.scrollHeight,
        clientHeight: container.clientHeight,
        lastEvent: 'scroll',
        scrollCount: prev.scrollCount + 1,
      }))
    }
  }, [checkIsAtBottom])

  // iOS Safari fix: Also track via native scroll event listener
  // React's onScroll might not fire correctly on iOS
  // Depends on session so it re-runs after the container is rendered
  useEffect(() => {
    const container = containerRef.current
    if (!container) {
      if (DEBUG_SCROLL) {
        setDebugInfo(prev => ({ ...prev, lastEvent: 'NO CONTAINER REF' }))
      }
      return
    }

    // Debug: mark that we attached the listener
    if (DEBUG_SCROLL) {
      setDebugInfo(prev => ({ ...prev, lastEvent: 'listener-attached' }))
    }

    const onNativeScroll = () => {
      isAtBottomRef.current = checkIsAtBottom()

      if (DEBUG_SCROLL) {
        setDebugInfo(prev => ({
          ...prev,
          isAtBottom: isAtBottomRef.current,
          scrollTop: Math.round(container.scrollTop),
          scrollHeight: container.scrollHeight,
          clientHeight: container.clientHeight,
          lastEvent: 'native-scroll',
          scrollCount: prev.scrollCount + 1,
        }))
      }
    }

    // Also listen for wheel events directly (Magic Keyboard trackpad)
    const onWheel = () => {
      // Small delay to let scroll position update
      requestAnimationFrame(() => {
        isAtBottomRef.current = checkIsAtBottom()
        if (DEBUG_SCROLL) {
          setDebugInfo(prev => ({
            ...prev,
            isAtBottom: isAtBottomRef.current,
            scrollTop: Math.round(container.scrollTop),
            lastEvent: 'wheel',
            scrollCount: prev.scrollCount + 1,
          }))
        }
      })
    }

    // Use passive: true for better scroll performance
    container.addEventListener('scroll', onNativeScroll, { passive: true })
    container.addEventListener('wheel', onWheel, { passive: true })

    return () => {
      container.removeEventListener('scroll', onNativeScroll)
      container.removeEventListener('wheel', onWheel)
    }
  }, [checkIsAtBottom])

  // Scroll to bottom - always works, no conditions
  const scrollToBottom = useCallback(() => {
    const container = containerRef.current
    if (!container) return
    container.scrollTop = container.scrollHeight
    isAtBottomRef.current = true
    onScrollToBottom?.()
  }, [onScrollToBottom])

  // Expose scrollToBottom via ref for parent components
  useImperativeHandle(ref, () => ({
    scrollToBottom,
  }), [scrollToBottom])

  // Fetch session data with auto-refresh
  const { data: sessionResponse, refetch: refetchSession } = useGetSession(sessionId, {
    enabled: !!sessionId,
    refetchInterval: 2000,
  })

  const session = sessionResponse?.data

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

  // Scroll to bottom on initial load
  const hasInitiallyScrolled = useRef(false)
  useEffect(() => {
    if (session?.interactions && session.interactions.length > 0 && !hasInitiallyScrolled.current) {
      hasInitiallyScrolled.current = true
      // Small delay to ensure content is rendered
      requestAnimationFrame(() => {
        scrollToBottom()
      })
    }
  }, [session?.interactions?.length, scrollToBottom])

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
    await refetchSession()
    return session
  }, [refetchSession, session])

  // Regenerate handler
  const handleRegenerate = useCallback(async (interactionID: string, message: string) => {
    if (!session) return

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
        position: 'relative',
        // Prevent iOS momentum scroll from causing issues
        WebkitOverflowScrolling: 'touch',
        ...lightTheme.scrollbar,
      }}
    >
      {/* DEBUG OVERLAY */}
      {DEBUG_SCROLL && (
        <Box
          sx={{
            position: 'sticky',
            top: 0,
            left: 0,
            right: 0,
            zIndex: 9999,
            backgroundColor: 'rgba(0, 0, 0, 0.85)',
            color: '#0f0',
            fontFamily: 'monospace',
            fontSize: '11px',
            padding: '4px 8px',
            pointerEvents: 'none',
          }}
        >
          <div>atBottom: {debugInfo.isAtBottom ? 'YES' : 'NO'} | scrollTop: {debugInfo.scrollTop}</div>
          <div>scrollH: {debugInfo.scrollHeight} | clientH: {debugInfo.clientHeight}</div>
          <div>last: {debugInfo.lastEvent}</div>
          <div>mutations: {debugInfo.mutationCount} | scrolls: {debugInfo.scrollCount}</div>
        </Box>
      )}
      <Box
        sx={{
          width: '100%',
          maxWidth: 700,
          mx: 'auto',
          px: 2,
          py: 2,
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
          // Ensure content can shrink on narrow screens
          minWidth: 0,
          boxSizing: 'border-box',
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
