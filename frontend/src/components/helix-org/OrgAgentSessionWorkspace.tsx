// OrgAgentSessionWorkspace is the bot-session counterpart of the spec task
// detail page: chat on the left, and on the right the same view toolbar and
// surfaces a task has — desktop stream, reverse-tunnel browser, diff, files,
// details — plus the terminal drawer. Every surface is keyed by the session
// id (the container is session-backed), so nothing here needs a spec task.
//
// `children` is the chat (rendered by pages/Session.tsx). `bot` is optional:
// without it the workspace degrades to a plain session viewer, which is what
// non-org external-agent sessions get.

import { FC, ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import { PanelLeft, PanelRight } from 'lucide-react'
import {
  Group as PanelGroup,
  Panel,
  Separator as PanelResizeHandle,
} from 'react-resizable-panels'
import type { PanelImperativeHandle } from 'react-resizable-panels'

import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'
import { loadPanelLayout, savePanelLayout } from '../../lib/panelLayoutStorage'
import { BotDTO } from '../../services/helixOrgService'
import { TypesSandboxRuntime } from '../../api/api'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import DiffViewer from '../tasks/DiffViewer'
import SandboxBrowser from '../tasks/SandboxBrowser'
import { SandboxIndicatorState } from '../tasks/SandboxStatusIndicator'
import SpecTaskTerminalDrawer from '../tasks/SpecTaskTerminalDrawer'
import SpecTaskViewToolbar, { TaskView, toolbarIconButtonSx } from '../tasks/SpecTaskViewToolbar'
import TaskSessionPlaceholder from '../tasks/TaskSessionPlaceholder'
import OrgAgentSettingsPane from './OrgAgentSettingsPane'

export interface OrgAgentSessionWorkspaceProps {
  sessionId: string
  organizationId: string
  bot?: BotDTO
  onStart?: () => void
  onStop?: () => void
  onRestart?: () => void
  lifecycleBusy?: boolean
  /** Terminal "copy to chat" lands here; the parent appends it to the composer. */
  onAppendToChat?: (text: string) => void
  children: ReactNode
}

const VIEW_STORAGE_PREFIX = 'helix.orgAgentSession.view.'
const TERMINAL_STORAGE_PREFIX = 'helix.orgAgentSession.terminal.'
const DEFAULT_TERMINAL_HEIGHT = 280
const VALID_VIEWS: TaskView[] = ['chat', 'desktop', 'browser', 'changes', 'files', 'details']

const loadView = (key: string): TaskView | null => {
  if (!key) return null
  try {
    const raw = localStorage.getItem(key)
    return raw && (VALID_VIEWS as string[]).includes(raw) ? (raw as TaskView) : null
  } catch {
    return null
  }
}

const loadTerminalHeight = (key: string): number => {
  if (!key) return DEFAULT_TERMINAL_HEIGHT
  try {
    const raw = parseInt(localStorage.getItem(key) ?? '', 10)
    return Number.isFinite(raw) && raw > 0 ? raw : DEFAULT_TERMINAL_HEIGHT
  } catch {
    return DEFAULT_TERMINAL_HEIGHT
  }
}

/** Maps the bot DTO onto the three-state indicator the task page uses. */
export const botSandboxIndicatorState = (bot?: BotDTO): SandboxIndicatorState => {
  if (!bot) return 'running'
  if (bot.status === 'running') return 'running'
  if (bot.sandbox_status === 'pending') return 'starting'
  return 'stopped'
}

const OrgAgentSessionWorkspace: FC<OrgAgentSessionWorkspaceProps> = ({
  sessionId,
  organizationId,
  bot,
  onStart,
  onStop,
  onRestart,
  lifecycleBusy = false,
  onAppendToChat,
  children,
}) => {
  const isBigScreen = useIsBigScreen()
  const lightTheme = useLightTheme()
  const panelIds = ['org-agent-session-chat', 'org-agent-session-desktop'] as const
  const layoutKey = organizationId ? `helix.orgAgentSession.layout.${organizationId}` : ''
  const viewKey = organizationId ? `${VIEW_STORAGE_PREFIX}${organizationId}` : ''
  const terminalKey = organizationId ? `${TERMINAL_STORAGE_PREFIX}${organizationId}` : ''
  const savedLayout = loadPanelLayout(layoutKey, panelIds)
  const lastExpandedContentSizeRef = useRef(savedLayout?.['org-agent-session-desktop'] ?? 62)
  const contentPanelRef = useRef<PanelImperativeHandle>(null)
  const collapseContentAfterSplitRef = useRef(false)
  const dividerColor = lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'

  const isHeadless = bot?.effective_sandbox_runtime === TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu
  const indicatorState = botSandboxIndicatorState(bot)
  const desktopRunning = indicatorState === 'running'
  const starting = indicatorState === 'starting'
  const defaultView: TaskView = isHeadless ? 'changes' : 'desktop'

  const [view, setView] = useState<TaskView>(() => loadView(viewKey) ?? defaultView)
  const [terminalOpen, setTerminalOpen] = useState(false)
  const [terminalHeight, setTerminalHeight] = useState(() => loadTerminalHeight(terminalKey))
  const [chatCollapsed, setChatCollapsed] = useState(false)
  const [contentCollapsed, setContentCollapsed] = useState(false)

  // A headless bot has no desktop; if the stored view is desktop, fall back.
  // On a big screen chat has its own panel, so "chat" as a right-panel view
  // falls through to the default surface too.
  useEffect(() => {
    if (isHeadless && view === 'desktop') setView('changes')
    if (isBigScreen && view === 'chat') setView(defaultView)
  }, [isHeadless, isBigScreen, view, defaultView])

  const handleViewChange = useCallback((next: TaskView | null) => {
    if (!next) return
    setView(next)
    if (viewKey) {
      try { localStorage.setItem(viewKey, next) } catch { /* storage unavailable */ }
    }
  }, [viewKey])

  const handleTerminalHeight = useCallback((height: number) => {
    setTerminalHeight(height)
    if (terminalKey) {
      try { localStorage.setItem(terminalKey, String(height)) } catch { /* storage unavailable */ }
    }
  }, [terminalKey])

  const collapseContentPanel = useCallback(() => {
    if (chatCollapsed) {
      collapseContentAfterSplitRef.current = true
      setContentCollapsed(true)
      setChatCollapsed(false)
      return
    }
    const currentSize = contentPanelRef.current?.getSize().asPercentage
    if (currentSize && currentSize > 0) {
      lastExpandedContentSizeRef.current = currentSize
    }
    contentPanelRef.current?.collapse()
    setContentCollapsed(true)
  }, [chatCollapsed])

  const showContentPanel = useCallback(() => {
    const panel = contentPanelRef.current
    if (!panel) return
    const restoredSize = lastExpandedContentSizeRef.current || 62
    panel.expand()
    panel.resize(`${restoredSize}%`)
    setContentCollapsed(false)
  }, [])

  const toolbar = (singlePanel: boolean) => (
    <SpecTaskViewToolbar
      currentView={view}
      onViewChange={handleViewChange}
      hasSession={!!sessionId}
      showChatTab={!isBigScreen}
      showDesktop={!isHeadless}
      onToggleTerminal={() => setTerminalOpen((open) => !open)}
      terminalOpen={terminalOpen}
      showStart={!!bot && !!onStart && !desktopRunning && !starting}
      onStart={onStart}
      startBusy={lifecycleBusy}
      showStop={!!bot && !!onStop && desktopRunning}
      onStop={onStop}
      stopBusy={lifecycleBusy}
      showRestart={!!bot && !!onRestart}
      onRestart={onRestart}
      restartBusy={lifecycleBusy}
      detailsLabel="Settings"
      onRestoreSplit={singlePanel ? () => setChatCollapsed(false) : undefined}
      onCollapsePanel={isBigScreen ? collapseContentPanel : undefined}
    />
  )

  const stoppedPlaceholder = (title: string, description: string) => (
    <TaskSessionPlaceholder
      tone="paused"
      title={title}
      description={description}
      detail={bot?.sandbox_status_message}
      onStart={onStart}
      starting={starting || lifecycleBusy}
    />
  )

  const surface = useMemo(() => {
    switch (view) {
      case 'chat':
        return children
      case 'browser':
        return desktopRunning
          ? <SandboxBrowser sessionId={sessionId} />
          : stoppedPlaceholder('Sandbox not running', 'Start the agent to preview a localhost web app from its sandbox.')
      case 'changes':
      case 'files':
        return (
          <DiffViewer
            sessionId={sessionId}
            pollInterval={3000}
            primarySurface={view}
            onPrimarySurfaceChange={handleViewChange}
            onStartDesktop={onStart}
            desktopRunning={desktopRunning}
            isDesktopStarting={starting || lifecycleBusy}
            desktopUnavailableDetail={bot?.sandbox_status_message}
          />
        )
      case 'details':
        return bot
          ? <OrgAgentSettingsPane bot={bot} sessionId={sessionId} organizationId={organizationId} indicatorState={indicatorState} />
          : null
      case 'desktop':
      default:
        if (isHeadless) {
          return stoppedPlaceholder('Headless sandbox', 'This agent runs without a desktop. Use Diff, Files, Browser or the terminal.')
        }
        return (
          <ExternalAgentDesktopViewer
            sessionId={sessionId}
            sandboxId={sessionId}
            mode="stream"
          />
        )
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view, sessionId, organizationId, desktopRunning, starting, lifecycleBusy, isHeadless, indicatorState, bot?.sandbox_status_message, bot?.sandbox_id, bot?.sandbox_status, bot?.restart_required, bot?.effective_sandbox_runtime, bot?.effective_sandbox_resource_overrides?.vcpus, children])

  const content = (singlePanel = false) => (
    <Box sx={{ height: '100%', minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <Box sx={{ flexShrink: 0, borderBottom: `1px solid ${dividerColor}` }}>
        {toolbar(singlePanel)}
      </Box>
      <Box
        sx={{
          flex: 1,
          minHeight: 0,
          minWidth: 0,
          display: 'flex',
          flexDirection: 'column',
          overflow: view === 'details' ? 'auto' : 'hidden',
          p: view === 'details' ? 2 : 0,
        }}
      >
        {surface}
      </Box>
    </Box>
  )

  const chat = (
    <Box sx={{ height: '100%', minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <Box
        sx={{
          minHeight: 53,
          px: 1,
          pt: 1,
          pb: 0.5,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          gap: 0.25,
          flexShrink: 0,
          borderBottom: `1px solid ${dividerColor}`,
          backgroundColor: 'background.paper',
          boxSizing: 'border-box',
        }}
      >
        {contentCollapsed ? (
          <Tooltip title="Show task panel">
            <IconButton
              size="small"
              aria-label="Show task panel"
              onClick={showContentPanel}
              sx={toolbarIconButtonSx('comfortable')}
            >
              <PanelRight size={18} />
            </IconButton>
          </Tooltip>
        ) : (
          <Tooltip title="Collapse chat panel">
            <IconButton
              size="small"
              aria-label="Collapse chat panel"
              onClick={() => setChatCollapsed(true)}
              sx={toolbarIconButtonSx('comfortable')}
            >
              <PanelLeft size={18} />
            </IconButton>
          </Tooltip>
        )}
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, minWidth: 0, overflow: 'hidden' }}>
        {children}
      </Box>
    </Box>
  )

  // The terminal drawer sits under the whole workspace — chat and content
  // alike — exactly as on the spec task page, not inside the right panel.
  const terminalDrawer = terminalOpen && sessionId ? (
    <SpecTaskTerminalDrawer
      sessionId={sessionId}
      running={desktopRunning}
      height={terminalHeight}
      onHeightChange={handleTerminalHeight}
      onClose={() => setTerminalOpen(false)}
      onCopyToChat={(text) => onAppendToChat?.(text)}
    />
  ) : null

  if (!isBigScreen) {
    return (
      <Box sx={{ height: '100%', minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <Box sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          {content()}
        </Box>
        {terminalDrawer}
      </Box>
    )
  }

  if (chatCollapsed) {
    return (
      <Box sx={{ height: '100%', minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <Box sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          {content(true)}
        </Box>
        {terminalDrawer}
      </Box>
    )
  }

  return (
    <Box sx={{ height: '100%', minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
    <PanelGroup
      id="org-agent-session-workspace"
      orientation="horizontal"
      defaultLayout={collapseContentAfterSplitRef.current
        ? { 'org-agent-session-chat': 100, 'org-agent-session-desktop': 0 }
        : savedLayout ?? {
            'org-agent-session-chat': 38,
            'org-agent-session-desktop': 62,
          }}
      onLayoutChange={(layout) => {
        if (layout['org-agent-session-desktop'] === 0) {
          collapseContentAfterSplitRef.current = false
        }
        if (layout['org-agent-session-chat'] > 0 && layout['org-agent-session-desktop'] > 0) {
          lastExpandedContentSizeRef.current = layout['org-agent-session-desktop']
          savePanelLayout(layoutKey, layout, panelIds)
        }
      }}
      style={{ flex: 1, minHeight: 0, width: '100%' }}
    >
      <Panel
        id="org-agent-session-chat"
        defaultSize="38%"
        minSize="25%"
        style={{ overflow: 'hidden', minWidth: 0, minHeight: 0 }}
      >
        {chat}
      </Panel>
      <PanelResizeHandle
        id="org-agent-session-resize"
        style={{
          width: contentCollapsed ? 0 : 6,
          flexGrow: 0,
          flexShrink: 0,
          flexBasis: contentCollapsed ? 0 : 6,
          background: dividerColor,
          cursor: contentCollapsed ? 'default' : 'col-resize',
          outline: 'none',
          overflow: 'hidden',
        }}
      />
      <Panel
        id="org-agent-session-desktop"
        defaultSize="62%"
        minSize="30%"
        collapsible
        collapsedSize={0}
        panelRef={contentPanelRef}
        onResize={(size) => setContentCollapsed(size.asPercentage === 0)}
        style={{ overflow: 'hidden', minWidth: 0, minHeight: 0 }}
      >
        {content()}
      </Panel>
    </PanelGroup>
    {terminalDrawer}
    </Box>
  )
}

export default OrgAgentSessionWorkspace
