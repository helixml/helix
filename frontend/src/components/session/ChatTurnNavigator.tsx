import React, { FC, MouseEvent, useCallback, useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import { alpha, useTheme } from '@mui/material/styles'

import { getChatColors } from './chatStyles'
import {
  CHAT_TURN_NAVIGATOR_ITEM_SPACING,
  CHAT_TURN_NAVIGATOR_MIN_ITEMS,
  ChatTurnNavigatorItem,
  resolveChatTurnNavigatorIndexFromPointer,
  resolveChatTurnNavigatorTopPercent,
} from './ChatTurnNavigator.logic'

interface ChatTurnNavigatorProps {
  items: ChatTurnNavigatorItem[]
  scrollContainer: HTMLDivElement | null
  onSelect: (item: ChatTurnNavigatorItem) => void
}

const ChatTurnNavigator: FC<ChatTurnNavigatorProps> = ({
  items,
  scrollContainer,
  onSelect,
}) => {
  const theme = useTheme()
  const colors = getChatColors(theme)
  const [activeIndex, setActiveIndex] = useState<number | null>(null)
  const markerRefs = useRef(new Map<string, HTMLSpanElement>())

  const resolvedActiveIndex = activeIndex !== null && activeIndex < items.length
    ? activeIndex
    : null
  const activeItem = resolvedActiveIndex === null ? null : items[resolvedActiveIndex]
  const activeTop = resolvedActiveIndex === null
    ? 0
    : resolveChatTurnNavigatorTopPercent(resolvedActiveIndex, items.length)
  const activeTranslate = resolvedActiveIndex === 0
    ? '0%'
    : resolvedActiveIndex === items.length - 1
      ? '-100%'
      : '-50%'

  const naturalHeight = Math.max(1, (items.length - 1) * CHAT_TURN_NAVIGATOR_ITEM_SPACING)

  const updateVisibleMarkers = useCallback(() => {
    if (!scrollContainer) return
    const viewport = scrollContainer.getBoundingClientRect()
    const turnElements = new Map<string, HTMLElement>()
    scrollContainer.querySelectorAll<HTMLElement>('[data-chat-turn]').forEach((element) => {
      const id = element.dataset.chatTurn
      if (id) turnElements.set(id, element)
    })
    items.forEach((item) => {
      const marker = markerRefs.current.get(item.id)
      const turn = turnElements.get(item.id)
      if (!marker || !turn) return
      const rect = turn.getBoundingClientRect()
      const inView = rect.top < viewport.bottom && rect.bottom > viewport.top
      marker.dataset.inView = inView ? 'true' : 'false'
    })
  }, [items, scrollContainer])

  useEffect(() => {
    if (!scrollContainer) return
    const frame = requestAnimationFrame(updateVisibleMarkers)
    const observer = new ResizeObserver(updateVisibleMarkers)
    observer.observe(scrollContainer)
    if (scrollContainer.firstElementChild) observer.observe(scrollContainer.firstElementChild)
    scrollContainer.addEventListener('scroll', updateVisibleMarkers, { passive: true })
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
      scrollContainer.removeEventListener('scroll', updateVisibleMarkers)
    }
  }, [scrollContainer, updateVisibleMarkers])

  const resolveIndexFromPointer = useCallback((event: MouseEvent<HTMLButtonElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    return resolveChatTurnNavigatorIndexFromPointer({
      itemCount: items.length,
      railTop: rect.top,
      railHeight: rect.height,
      pointerY: event.clientY,
    })
  }, [items.length])

  const moveActiveIndex = useCallback((delta: number) => {
    setActiveIndex((current) => {
      const base = current ?? 0
      return Math.max(0, Math.min(items.length - 1, base + delta))
    })
  }, [items.length])

  if (items.length < CHAT_TURN_NAVIGATOR_MIN_ITEMS) return null

  return (
    <Box
      data-chat-turn-navigator
      sx={{
        pointerEvents: 'none',
        position: 'absolute',
        inset: 0,
        right: 'auto',
        zIndex: 4,
        width: '100%',
        display: 'none',
        '@media (pointer: fine)': {
          display: 'block',
        },
      }}
    >
      <Box
        component="button"
        type="button"
        aria-label={`Jump to message: ${activeItem?.userText ?? 'User message'}`}
        onBlur={() => setActiveIndex(null)}
        onFocus={() => setActiveIndex((current) => current ?? 0)}
        onMouseLeave={() => setActiveIndex(null)}
        onMouseMove={(event: MouseEvent<HTMLButtonElement>) => {
          const next = resolveIndexFromPointer(event)
          setActiveIndex((current) => current === next ? current : next)
        }}
        onMouseDown={(event: MouseEvent<HTMLButtonElement>) => {
          event.preventDefault()
        }}
        onClick={(event: MouseEvent<HTMLButtonElement>) => {
          const index = resolveIndexFromPointer(event)
          const item = index === null ? null : items[index]
          if (item) onSelect(item)
          event.currentTarget.blur()
        }}
        onKeyDown={(event: React.KeyboardEvent<HTMLButtonElement>) => {
          if (event.key === 'ArrowDown') {
            event.preventDefault()
            moveActiveIndex(1)
          } else if (event.key === 'ArrowUp') {
            event.preventDefault()
            moveActiveIndex(-1)
          } else if (event.key === 'Home') {
            event.preventDefault()
            setActiveIndex(0)
          } else if (event.key === 'End') {
            event.preventDefault()
            setActiveIndex(items.length - 1)
          } else if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            if (activeItem) onSelect(activeItem)
          }
        }}
        sx={{
          pointerEvents: 'none',
          position: 'absolute',
          top: '50%',
          left: 12,
          transform: 'translateY(-50%)',
          width: activeItem ? 'calc(100% - 20px)' : 24,
          height: `min(${naturalHeight}px, calc(100% - 48px))`,
          m: 0,
          p: 0,
          border: 0,
          background: 'transparent',
          cursor: 'pointer',
          color: colors.muted,
          textAlign: 'left',
          '&:focus-visible': {
            outline: `2px solid ${alpha(colors.foreground, 0.55)}`,
            outlineOffset: -2,
          },
        }}
      >
        <Box
          component="span"
          aria-hidden
          data-chat-turn-rail-hit-target
          sx={{
            pointerEvents: 'auto',
            position: 'absolute',
            inset: 0,
            width: 24,
          }}
        />
        {items.map((item, index) => {
          const distance = resolvedActiveIndex === null
            ? null
            : Math.abs(index - resolvedActiveIndex)
          const width = distance === 0 ? 24 : distance === 1 ? 16 : distance === 2 ? 10 : 8
          return (
            <Box
              component="span"
              aria-hidden
              data-chat-turn-marker
              data-in-view="false"
              key={item.id}
              ref={(node: HTMLSpanElement | null) => {
                if (node) markerRefs.current.set(item.id, node)
                else markerRefs.current.delete(item.id)
              }}
              sx={{
                pointerEvents: 'none',
                position: 'absolute',
                top: `${resolveChatTurnNavigatorTopPercent(index, items.length)}%`,
                left: 0,
                width,
                height: 2,
                transform: 'translateY(-50%)',
                borderRadius: 999,
                backgroundColor: alpha(colors.muted, distance === 0 ? 0.78 : 0.38),
                transition: 'width 150ms ease, background-color 150ms ease',
                '&[data-in-view="true"]': {
                  backgroundColor: alpha(colors.foreground, 0.92),
                },
              }}
            />
          )
        })}
        {activeItem && (
          <Box
            component="span"
            data-chat-turn-preview
            sx={{
              pointerEvents: 'none',
              position: 'absolute',
              top: `${activeTop}%`,
              left: 32,
              right: 0,
              maxWidth: 300,
              transform: `translateY(${activeTranslate})`,
            }}
          >
            <Box
              component="span"
              sx={{
                display: 'block',
                p: 1.5,
                borderRadius: 3,
                border: `1px solid ${colors.borderStrong}`,
                backgroundColor: alpha(colors.surfaceRaised, 0.96),
                color: colors.foreground,
                boxShadow: '0 12px 30px rgba(0,0,0,0.28)',
                backdropFilter: 'blur(12px)',
              }}
            >
              <Box
                component="span"
                sx={{
                  display: 'block',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  fontSize: '0.875rem',
                  fontWeight: 500,
                  lineHeight: 1.45,
                }}
              >
                {activeItem.userText ?? 'User message'}
              </Box>
              {activeItem.assistantText && (
                <Box
                  component="span"
                  sx={{
                    display: '-webkit-box',
                    mt: 0.5,
                    overflow: 'hidden',
                    WebkitBoxOrient: 'vertical',
                    WebkitLineClamp: 3,
                    color: colors.muted,
                    fontSize: '0.875rem',
                    lineHeight: 1.45,
                  }}
                >
                  {activeItem.assistantText}
                </Box>
              )}
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  )
}

export default ChatTurnNavigator
