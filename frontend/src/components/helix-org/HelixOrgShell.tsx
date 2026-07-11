// Shared chrome for every helix-org page: top-bar nav (Chart/Bots/Topics/
// Settings) + optional left chat rail + right content.
//
// Breadcrumbs sit on the left of the AppBar (same pattern as SpecTasks /
// Sandbox detail): org → section → leaf. Build them with
// useHelixOrgBreadcrumbs and pass via breadcrumbs + breadcrumbTitle.

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
import { IPageBreadcrumb } from '../../types'

export type HelixOrgShellProps = {
  /** Leading crumbs (org + optional section). Leaf goes in breadcrumbTitle. */
  breadcrumbs?: IPageBreadcrumb[]
  /** Current page leaf (e.g. "Bots", "Settings", or a bot/topic name). */
  breadcrumbTitle?: string
  /** Optional action buttons next to the top nav (before theme toggle). */
  topbarActions?: ReactNode
  /**
   * Show the left chat rail. Only the Chart page sets this true —
   * Bots / Topics / Settings / detail pages use the full width.
   */
  showChat?: boolean
  children: ReactNode
}

const HelixOrgShell: FC<HelixOrgShellProps> = ({
  breadcrumbs,
  breadcrumbTitle,
  topbarActions,
  showChat = false,
  children,
}) => {
  const account = useAccount()
  const isBigScreen = useIsBigScreen()
  const lightTheme = useLightTheme()
  const isLight = lightTheme.isLight

  const content = (
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
  )

  // react-resizable-panels v4: numbers = pixels, strings = percentages.
  // Use "32%" not 32, or the chat rail collapses to ~32px.
  return (
    <Page
      breadcrumbs={breadcrumbs}
      breadcrumbTitle={breadcrumbTitle}
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
      {isBigScreen && showChat ? (
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
              {content}
            </Panel>
          </PanelGroup>
        </Box>
      ) : (
        <Box sx={{ flex: 1, minHeight: 0, overflow: showChat ? 'auto' : 'hidden', height: '100%' }}>
          {content}
        </Box>
      )}
    </Page>
  )
}

export default HelixOrgShell
