import { FC, ReactNode, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'
import ChatOutlinedIcon from '@mui/icons-material/ChatOutlined'
import DesktopWindowsOutlinedIcon from '@mui/icons-material/DesktopWindowsOutlined'
import {
  Group as PanelGroup,
  Panel,
  Separator as PanelResizeHandle,
} from 'react-resizable-panels'

import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'
import { loadPanelLayout, savePanelLayout } from '../../lib/panelLayoutStorage'
import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'

interface OrgAgentSessionWorkspaceProps {
  sessionId: string
  organizationId: string
  children: ReactNode
}

type MobileView = 'chat' | 'desktop'

const OrgAgentSessionWorkspace: FC<OrgAgentSessionWorkspaceProps> = ({
  sessionId,
  organizationId,
  children,
}) => {
  const isBigScreen = useIsBigScreen()
  const lightTheme = useLightTheme()
  const [mobileView, setMobileView] = useState<MobileView>('chat')
  const panelIds = ['org-agent-session-chat', 'org-agent-session-desktop'] as const
  const layoutKey = organizationId ? `helix.orgAgentSession.layout.${organizationId}` : ''
  const savedLayout = loadPanelLayout(layoutKey, panelIds)
  const dividerColor = lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'

  const desktop = (
    <Box sx={{ height: '100%', minHeight: 0, minWidth: 0, display: 'flex', overflow: 'hidden' }}>
      <ExternalAgentDesktopViewer
        sessionId={sessionId}
        sandboxId={sessionId}
        mode="stream"
      />
    </Box>
  )

  if (!isBigScreen) {
    return (
      <Box sx={{ height: '100%', minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <Stack
          direction="row"
          spacing={0.5}
          sx={{ px: 1.5, py: 0.75, borderBottom: `1px solid ${dividerColor}`, flexShrink: 0 }}
        >
          <Button
            size="small"
            variant={mobileView === 'chat' ? 'contained' : 'text'}
            startIcon={<ChatOutlinedIcon />}
            onClick={() => setMobileView('chat')}
            sx={{ textTransform: 'none' }}
          >
            Chat
          </Button>
          <Button
            size="small"
            variant={mobileView === 'desktop' ? 'contained' : 'text'}
            startIcon={<DesktopWindowsOutlinedIcon />}
            onClick={() => setMobileView('desktop')}
            sx={{ textTransform: 'none' }}
          >
            Desktop
          </Button>
        </Stack>
        <Box sx={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
          {mobileView === 'chat' ? children : desktop}
        </Box>
      </Box>
    )
  }

  return (
    <PanelGroup
      id="org-agent-session-workspace"
      orientation="horizontal"
      defaultLayout={savedLayout ?? {
        'org-agent-session-chat': 38,
        'org-agent-session-desktop': 62,
      }}
      onLayoutChange={(layout) => savePanelLayout(layoutKey, layout, panelIds)}
      style={{ height: '100%', width: '100%' }}
    >
      <Panel
        id="org-agent-session-chat"
        defaultSize="38%"
        minSize="25%"
        maxSize="70%"
        style={{ overflow: 'hidden', minWidth: 0, minHeight: 0 }}
      >
        {children}
      </Panel>
      <PanelResizeHandle
        id="org-agent-session-resize"
        style={{
          width: 6,
          flex: '0 0 6px',
          background: dividerColor,
          cursor: 'col-resize',
          outline: 'none',
        }}
      />
      <Panel
        id="org-agent-session-desktop"
        defaultSize="62%"
        minSize="30%"
        style={{ overflow: 'hidden', minWidth: 0, minHeight: 0 }}
      >
        {desktop}
      </Panel>
    </PanelGroup>
  )
}

export default OrgAgentSessionWorkspace
