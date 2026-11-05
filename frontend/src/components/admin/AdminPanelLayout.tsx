import React, { FC, ReactNode, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Drawer from '@mui/material/Drawer'
import useMediaQuery from '@mui/material/useMediaQuery'
import { useTheme } from '@mui/material/styles'
import { ChevronDown, Menu } from 'lucide-react'

import AdminPanelSidebar, { getAdminTabLabel } from './AdminPanelSidebar'

interface AdminPanelLayoutProps {
  activeTab: string
  onTabChange: (tab: string) => void
  children: ReactNode
}

// Below `md` a permanent 240px sidebar leaves the content pane too narrow to
// use, so navigation collapses into a drawer opened from a section bar.
const AdminPanelLayout: FC<AdminPanelLayoutProps> = ({ activeTab, onTabChange, children }) => {
  const theme = useTheme()
  const isNarrow = useMediaQuery(theme.breakpoints.down('md'))
  const [navOpen, setNavOpen] = useState(false)
  const activeLabel = getAdminTabLabel(activeTab)

  const handleNarrowTabChange = (tab: string) => {
    onTabChange(tab)
    setNavOpen(false)
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: isNarrow ? 'column' : 'row', height: '100%', overflow: 'hidden' }}>
      {isNarrow ? (
        <>
          <Box sx={{
            flexShrink: 0,
            px: 1,
            py: 0.5,
            borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
          }}>
            <Button
              onClick={() => setNavOpen(true)}
              aria-label={`Admin sections: ${activeLabel}`}
              aria-haspopup="dialog"
              startIcon={<Menu size={18} />}
              endIcon={<ChevronDown size={18} />}
              sx={{ minHeight: 44, textTransform: 'none', color: 'text.primary', fontWeight: 600 }}
            >
              {activeLabel}
            </Button>
          </Box>
          <Drawer
            anchor="left"
            open={navOpen}
            onClose={() => setNavOpen(false)}
            // Same layer as popovers/menus so it opens above the dialog (100002, see contexts/theme.tsx).
            sx={{ zIndex: 100003 }}
            PaperProps={{ sx: { width: 280, maxWidth: '85vw' } }}
          >
            <Box sx={{ height: '100%', '& .MuiListItemButton-root': { minHeight: 44 } }}>
              <AdminPanelSidebar activeTab={activeTab} onTabChange={handleNarrowTabChange} />
            </Box>
          </Drawer>
        </>
      ) : (
        <Box sx={{
          width: 240,
          flexShrink: 0,
          borderRight: '1px solid rgba(255, 255, 255, 0.1)',
          overflowY: 'auto',
        }}>
          <AdminPanelSidebar activeTab={activeTab} onTabChange={onTabChange} />
        </Box>
      )}
      <Box sx={{
        flex: 1,
        overflow: 'auto',
        // Keep table columns readable on narrow screens; the TableContainer scrolls instead.
        [theme.breakpoints.down('md')]: {
          '& .MuiTableContainer-root .MuiTable-root': { minWidth: 640 },
        },
        '@media (pointer: coarse)': {
          '& .MuiButton-root, & .MuiIconButton-root': { minHeight: 44 },
          '& .MuiIconButton-root': { minWidth: 44 },
        },
      }}>
        {children}
      </Box>
    </Box>
  )
}

export default AdminPanelLayout
