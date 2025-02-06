import React, { FC, useCallback, useRef, useState, useEffect, ComponentType } from 'react'
import { VariableSizeList as ListComponent, ListChildComponentProps } from 'react-window'
import AutoSizer from 'react-virtualized-auto-sizer'
import Box from '@mui/material/Box'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../../hooks/useThemeConfig'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import RefreshIcon from '@mui/icons-material/Refresh'

import Interaction from './Interaction'
import InteractionLiveStream from './InteractionLiveStream'
import { IInteraction, ISession, IServerConfig, ICloneInteractionMode, INTERACTION_STATE_EDITING } from '../../types'
import useAccount from '../../hooks/useAccount'

const MIN_ROW_HEIGHT = 80 // Minimum height for an interaction
const DEFAULT_ROW_HEIGHT = 150 // Default height for an interaction

interface ListData {
  interactions: IInteraction[]
  session: ISession
  serverConfig: IServerConfig
  highlightAllFiles: boolean
  retryFinetuneErrors: () => void
  onReloadSession: () => void
  onClone: (mode: ICloneInteractionMode, interactionId: string) => Promise<boolean>
  onAddDocuments?: () => void
  onRestart?: () => void
  onMessageChange?: () => void
  hasSubscription: boolean
}

interface VirtualizedInteractionListProps extends Omit<ListData, 'hasSubscription'> {
  onMessageChange?: () => void
}

const VirtualizedInteractionList: FC<VirtualizedInteractionListProps> = ({
  interactions,
  session,
  serverConfig,
  highlightAllFiles,
  retryFinetuneErrors,
  onReloadSession,
  onClone,
  onAddDocuments,
  onRestart,
  onMessageChange,
}) => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const account = useAccount()
  const listRef = useRef<ListComponent>(null)
  const [scrollToIndex, setScrollToIndex] = useState<number | null>(null)
  const rowHeights = useRef<{[key: number]: number}>({})

  // Function to get the height for a specific row
  const getRowHeight = useCallback((index: number) => {
    const interaction = interactions[index]
    const messageLength = interaction?.message?.length || 0
    
    // Calculate height based on message length and other factors
    let height = MIN_ROW_HEIGHT
    if (messageLength > 0) {
      // Roughly estimate 20px per line, assuming ~100 chars per line
      const estimatedLines = Math.ceil(messageLength / 100)
      height = Math.max(MIN_ROW_HEIGHT, estimatedLines * 20 + 60) // 60px for padding/headers
    }
    
    // Store the calculated height
    rowHeights.current[index] = height
    return height
  }, [interactions])

  // Reset cache when interactions change
  useEffect(() => {
    if (listRef.current) {
      listRef.current.resetAfterIndex(0)
    }
  }, [interactions])

  // Scroll to bottom when new messages arrive
  useEffect(() => {
    if (interactions.length > 0) {
      setScrollToIndex(interactions.length - 1)
    }
  }, [interactions.length])

  // Reset scroll position when scrollToIndex changes
  useEffect(() => {
    if (scrollToIndex !== null && listRef.current) {
      listRef.current.scrollToItem(scrollToIndex, 'end')
      setScrollToIndex(null)
    }
  }, [scrollToIndex])

  const Row: ComponentType<ListChildComponentProps<ListData>> = useCallback(({ index, style, data }) => {
    const interaction = data.interactions[index]
    const isLastInteraction = index === data.interactions.length - 1
    const isLastFinetune = false // TODO: implement this check
    const isLive = isLastInteraction && !interaction.finished && interaction.state != INTERACTION_STATE_EDITING
    const isOwner = account.user?.id === session.owner

    return (
      <div style={style}>
        <Interaction
          key={interaction.id}
          serverConfig={data.serverConfig}
          interaction={interaction}
          session={data.session}
          highlightAllFiles={data.highlightAllFiles}
          retryFinetuneErrors={data.retryFinetuneErrors}
          onReloadSession={data.onReloadSession}
          onClone={data.onClone}
          onAddDocuments={isLastFinetune ? data.onAddDocuments : undefined}
          onRestart={isLastInteraction ? data.onRestart : undefined}
          headerButtons={isLastInteraction ? (
            <Tooltip title="Restart Session">
              <IconButton onClick={data.onRestart} sx={{ mb: '0.5rem' }}>
                <RefreshIcon
                  sx={{
                    color: theme.palette.mode === 'light' ? themeConfig.lightIcon : themeConfig.darkIcon,
                    '&:hover': {
                      color: theme.palette.mode === 'light' ? themeConfig.lightIconHover : themeConfig.darkIconHover
                    },
                  }}
                />
              </IconButton>
            </Tooltip>
          ) : undefined}
        >
          {isLive && (isOwner || account.admin) && (
            <InteractionLiveStream
              session_id={data.session.id}
              interaction={interaction}
              session={data.session}
              serverConfig={data.serverConfig}
              hasSubscription={data.hasSubscription}
              onMessageChange={data.onMessageChange}
            />
          )}
        </Interaction>
      </div>
    )
  }, [account.user?.id, account.admin, theme.palette.mode])

  const listData: ListData = {
    interactions,
    session,
    serverConfig,
    highlightAllFiles,
    retryFinetuneErrors,
    onReloadSession,
    onClone,
    onAddDocuments,
    onRestart,
    onMessageChange,
    hasSubscription: account.userConfig.stripe_subscription_active || false,
  }

  return (
    <Box
      sx={{
        width: '100%',
        height: '100%',
        '& .virtual-scroll': {
          '&::-webkit-scrollbar': {
            width: '4px',
            borderRadius: '8px',
          },
          '&::-webkit-scrollbar-track': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
          },
          '&::-webkit-scrollbar-thumb': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
            borderRadius: '8px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
          },
        }
      }}
    >
      <AutoSizer>
        {({ height, width }) => {
          const List = ListComponent as any
          return (
            <List
              ref={listRef}
              className="virtual-scroll"
              height={height}
              width={width}
              itemCount={interactions.length}
              itemSize={getRowHeight}
              itemData={listData}
            >
              {Row}
            </List>
          )
        }}
      </AutoSizer>
    </Box>
  )
}

export default VirtualizedInteractionList 