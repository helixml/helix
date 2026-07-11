// Shared chrome for every helix-org page: top-bar nav (Chart/Bots/Topics/
// Settings) + left chat rail + right content, matching the spec-task
// chat/desktop split pattern.

import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import {
  Panel,
  Group as PanelGroup,
  Separator as PanelResizeHandle,
} from 'react-resizable-panels'

import Page from '../system/Page'
import HelixOrgChatPanel from './HelixOrgChatPanel'
import HelixOrgTopNav from './HelixOrgTopNav'
import useAccount from '../../hooks/useAccount'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'

export type HelixOrgShellProps = {
  /** Shown as the page breadcrumb title (e.g. "Chart"). */
  title?: string
  /** Optional action buttons next to the top nav (before theme toggle). */
  topbarActions?: ReactNode
  children: ReactNode
}

const HelixOrgShell: FC<HelixOrgShellProps> = ({ title, topbarActions, children }) => {
  const account = useAccount()
  const isBigScreen = useIsBigScreen()
  const lightTheme = useLightTheme()
  const isLight = lightTheme.isLight

  // react-resizable-panels v4: numbers = pixels, strings = percentages.
  // Use "32%" not 32, or the chat rail collapses to ~32px.
  return (
    <Page
      breadcrumbTitle={title}
      breadcrumbShowHome={false}
      organizationId={account.organizationTools.organization?.id}
      disableContentScroll
      px={0}
      sx={{
        height: '100%',
        minHeight: 0,
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
      }}
      topbarContent={(
        <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 1 }}>
          <HelixOrgTopNav />
          {topbarActions}
        </Box>
      )}
    >
      {isBigScreen ? (
        <Box
          sx={{
            flex: 1,
            minHeight: 0,
            width: '100%',
            height: '100%',
            display: 'flex',
            overflow: 'hidden',
          }}
        >
          <PanelGroup
            id="helix-org-shell"
            orientation="horizontal"
            style={{ height: '100%', width: '100%' }}
          >
            <Panel
              id="helix-org-chat"
              defaultSize="32%"
              minSize="20%"
              maxSize="50%"
              style={{ overflow: 'hidden', minWidth: 0, minHeight: 0 }}
            >
              <Box sx={{ height: '100%', width: '100%', minHeight: 0, minWidth: 0, overflow: 'hidden' }}>
                <HelixOrgChatPanel />
              </Box>
            </Panel>

            <PanelResizeHandle
              id="helix-org-resize"
              style={{
                width: 6,
                flex: '0 0 6px',
                background: isLight ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.04)',
                cursor: 'col-resize',
                outline: 'none',
              }}
            />

            <Panel
              id="helix-org-content"
              defaultSize="68%"
              minSize="40%"
              style={{ overflow: 'hidden', minWidth: 0, minHeight: 0 }}
            >
              <Box
                sx={{
                  height: '100%',
                  width: '100%',
                  minHeight: 0,
                  minWidth: 0,
                  display: 'flex',
                  flexDirection: 'column',
                  overflow: 'hidden',
                }}
              >
                {children}
              </Box>
            </Panel>
          </PanelGroup>
        </Box>
      ) : (
        <Box sx={{ flex: 1, minHeight: 0, overflow: 'auto' }}>
          {children}
        </Box>
      )}
    </Page>
  )
}

export default HelixOrgShell
