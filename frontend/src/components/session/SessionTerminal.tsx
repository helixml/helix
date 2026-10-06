import { FC, useEffect, useMemo, useState } from 'react'
import {
  Plus,
  SquareSplitHorizontal,
  SquareSplitVertical,
  TerminalSquare,
  Trash2,
} from 'lucide-react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'

import {
  sessionTerminalUrl,
  useDeleteSessionTerminalSession,
} from '../../services/sessionService'
import PersistentTerminalPane from './PersistentTerminalPane'
import {
  addTerminalGroup,
  getFirstTerminalPaneName,
  getTerminalPaneNames,
  MAX_TERMINAL_PANES_PER_GROUP,
  readTerminalLayout,
  removeTerminalPane,
  splitActiveTerminal,
  TerminalLayoutNode,
  TerminalLayoutState,
  TerminalSplitDirection,
} from './sessionTerminalLayout'

interface Props {
  sessionId: string
  running: boolean
  fillContainer?: boolean
  onRequestClose?: () => void
  onCopyToChat?: (text: string) => void
}

const terminalSelectionStorageKey = (sessionId: string) =>
  `helix.session.${sessionId}.terminalLayout`

const previousTerminalSelectionStorageKey = (sessionId: string) =>
  `helix.session.${sessionId}.terminalSession`

const terminalActionButtonSx = {
  width: 26,
  height: 26,
  p: 0,
  color: 'rgba(255, 255, 255, 0.72)',
  borderRadius: 0.5,
  '&:hover': { bgcolor: 'rgba(255, 255, 255, 0.08)' },
} as const

const TerminalActionDivider = () => (
  <Box sx={{ width: '1px', height: 16, bgcolor: 'rgba(255, 255, 255, 0.1)' }} />
)

const generateSessionName = (): string => {
  const bytes = crypto.getRandomValues(new Uint8Array(6))
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

const readStoredLayout = (sessionId: string): TerminalLayoutState => {
  const fallback = generateSessionName()
  try {
    const current = window.localStorage.getItem(terminalSelectionStorageKey(sessionId))
    const previous = window.localStorage.getItem(previousTerminalSelectionStorageKey(sessionId))
    return readTerminalLayout(current ?? previous, fallback)
  } catch {
    return readTerminalLayout(null, fallback)
  }
}

const writeStoredLayout = (sessionId: string, layout: TerminalLayoutState) => {
  try {
    window.localStorage.setItem(
      terminalSelectionStorageKey(sessionId),
      JSON.stringify(layout),
    )
  } catch {
    // Browser persistence is best-effort; tmux remains the source of truth.
  }
}

interface TerminalTreeProps {
  node: TerminalLayoutNode
  sessionId: string
  activePaneName: string | null
  onActivate: (sessionName: string) => void
  onExit: (sessionName: string) => void
  onCopyToChat?: (text: string) => void
}

const TerminalTree: FC<TerminalTreeProps> = ({
  node,
  sessionId,
  activePaneName,
  onActivate,
  onExit,
  onCopyToChat,
}) => {
  if (node.type === 'pane') {
    return (
      <PersistentTerminalPane
        websocketUrl={sessionTerminalUrl(sessionId, node.sessionName)}
        active={node.sessionName === activePaneName}
        onActivate={() => onActivate(node.sessionName)}
        onExit={() => onExit(node.sessionName)}
        onCopyToChat={onCopyToChat}
      />
    )
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: node.direction === 'horizontal' ? 'row' : 'column',
        width: '100%',
        height: '100%',
        minWidth: 0,
        minHeight: 0,
        gap: '1px',
        bgcolor: 'rgba(255, 255, 255, 0.1)',
      }}
    >
      {node.children.map((child) => (
        <Box
          key={getFirstTerminalPaneName(child)}
          sx={{ flex: 1, minWidth: 0, minHeight: 0 }}
        >
          <TerminalTree
            node={child}
            sessionId={sessionId}
            activePaneName={activePaneName}
            onActivate={onActivate}
            onExit={onExit}
            onCopyToChat={onCopyToChat}
          />
        </Box>
      ))}
    </Box>
  )
}

const SessionTerminal: FC<Props> = ({
  sessionId,
  running,
  fillContainer = false,
  onRequestClose,
  onCopyToChat,
}) => {
  const [layout, setLayout] = useState<TerminalLayoutState>(() =>
    readStoredLayout(sessionId),
  )
  const [collapseWhenEmpty, setCollapseWhenEmpty] = useState(false)
  const deleteTerminal = useDeleteSessionTerminalSession(sessionId)
  const activeGroup = useMemo(
    () => layout.groups.find((group) => group.id === layout.activeGroupId),
    [layout.activeGroupId, layout.groups],
  )

  useEffect(() => {
    writeStoredLayout(sessionId, layout)
  }, [layout, sessionId])

  useEffect(() => {
    if (layout.groups.length > 0 || !collapseWhenEmpty) return
    setCollapseWhenEmpty(false)
    onRequestClose?.()
  }, [collapseWhenEmpty, layout.groups.length, onRequestClose])

  const createGroup = () => {
    setCollapseWhenEmpty(false)
    setLayout((current) => addTerminalGroup(current, generateSessionName()))
  }

  const splitTerminal = (direction: TerminalSplitDirection) => {
    if (
      activeGroup
      && getTerminalPaneNames(activeGroup.root).length
        >= MAX_TERMINAL_PANES_PER_GROUP
    ) {
      return
    }
    setLayout((current) => splitActiveTerminal(
      current,
      generateSessionName(),
      direction,
    ))
  }

  const removePane = (terminalSessionName: string) => {
    setCollapseWhenEmpty(true)
    setLayout((current) => removeTerminalPane(current, terminalSessionName))
  }

  const deleteActiveTerminal = () => {
    const terminalSessionName = layout.activePaneName
    if (!terminalSessionName) return
    deleteTerminal.mutate(terminalSessionName, {
      onSuccess: () => {
        removePane(terminalSessionName)
      },
    })
  }

  if (!running) {
    return (
      <Box sx={{ display: 'grid', placeItems: 'center', height: '100%', p: 4 }}>
        <Typography variant="body2" color="text.secondary">
          The development sandbox is not running. Start the desktop before opening a terminal.
        </Typography>
      </Box>
    )
  }

  return (
    <Box
      sx={{
        position: 'relative',
        display: 'flex',
        width: '100%',
        height: fillContainer ? '100%' : 320,
        minHeight: 0,
        bgcolor: '#090909',
        overflow: 'hidden',
      }}
    >
      {layout.groups.length > 1 && (
        <Box
          component="nav"
          aria-label="Terminal groups"
          sx={{
            zIndex: 2,
            width: 34,
            flexShrink: 0,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 0.5,
            pt: 0.75,
            bgcolor: '#111',
            borderRight: '1px solid rgba(255, 255, 255, 0.08)',
          }}
        >
          {layout.groups.map((group, index) => (
            <Tooltip key={group.id} title={`Terminal ${index + 1}`} placement="right">
              <IconButton
                aria-label={`Open terminal ${index + 1}`}
                size="small"
                onClick={() => setLayout((current) => ({
                  ...current,
                  activeGroupId: group.id,
                  activePaneName: getFirstTerminalPaneName(group.root),
                }))}
                sx={{
                  width: 28,
                  height: 28,
                  color: group.id === layout.activeGroupId ? 'common.white' : 'grey.600',
                  bgcolor: group.id === layout.activeGroupId
                    ? 'rgba(255, 255, 255, 0.1)'
                    : 'transparent',
                }}
              >
                <TerminalSquare size={15} />
              </IconButton>
            </Tooltip>
          ))}
        </Box>
      )}

      <Box sx={{ position: 'relative', flex: 1, minWidth: 0, minHeight: 0 }}>
        <Box
          role="toolbar"
          aria-label="Terminal layout controls"
          sx={{
            position: 'absolute',
            top: 7,
            right: 7,
            zIndex: 4,
            display: 'flex',
            alignItems: 'center',
            border: '1px solid rgba(255, 255, 255, 0.1)',
            borderRadius: 1,
            bgcolor: 'rgba(12, 12, 12, 0.92)',
            boxShadow: '0 2px 10px rgba(0, 0, 0, 0.32)',
          }}
        >
          <Tooltip title="Split horizontally">
            <span>
              <IconButton
                aria-label="Split terminal horizontally"
                size="small"
                disabled={activeGroup
                  ? getTerminalPaneNames(activeGroup.root).length
                    >= MAX_TERMINAL_PANES_PER_GROUP
                  : false}
                onClick={() => splitTerminal('horizontal')}
                sx={terminalActionButtonSx}
              >
                <SquareSplitHorizontal size={14} />
              </IconButton>
            </span>
          </Tooltip>
          <TerminalActionDivider />
          <Tooltip title="Split vertically">
            <span>
              <IconButton
                aria-label="Split terminal vertically"
                size="small"
                disabled={activeGroup
                  ? getTerminalPaneNames(activeGroup.root).length
                    >= MAX_TERMINAL_PANES_PER_GROUP
                  : false}
                onClick={() => splitTerminal('vertical')}
                sx={terminalActionButtonSx}
              >
                <SquareSplitVertical size={14} />
              </IconButton>
            </span>
          </Tooltip>
          <TerminalActionDivider />
          <Tooltip title="New terminal">
            <IconButton
              aria-label="New terminal"
              size="small"
              onClick={createGroup}
              sx={terminalActionButtonSx}
            >
              <Plus size={14} />
            </IconButton>
          </Tooltip>
          <TerminalActionDivider />
          <Tooltip title="Kill terminal session">
            <span>
              <IconButton
                aria-label="Kill terminal session"
                size="small"
                disabled={!layout.activePaneName || deleteTerminal.isPending}
                onClick={deleteActiveTerminal}
                sx={terminalActionButtonSx}
              >
                <Trash2 size={14} />
              </IconButton>
            </span>
          </Tooltip>
        </Box>

        {activeGroup ? (
          <TerminalTree
            node={activeGroup.root}
            sessionId={sessionId}
            activePaneName={layout.activePaneName}
            onActivate={(terminalSessionName) => setLayout((current) => ({
              ...current,
              activePaneName: terminalSessionName,
            }))}
            onExit={removePane}
            onCopyToChat={onCopyToChat}
          />
        ) : (
          <Box sx={{ display: 'grid', placeItems: 'center', height: '100%' }}>
            <Typography variant="body2" color="grey.600">
              Create a terminal with the + button.
            </Typography>
          </Box>
        )}
      </Box>
    </Box>
  )
}

export default SessionTerminal
