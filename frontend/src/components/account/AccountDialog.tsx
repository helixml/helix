import { FC, useCallback, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import DialogContent from '@mui/material/DialogContent'
import IconButton from '@mui/material/IconButton'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Typography from '@mui/material/Typography'
import useMediaQuery from '@mui/material/useMediaQuery'
import { useTheme } from '@mui/material/styles'

import { ChevronLeft, ChevronRight, X } from 'lucide-react'

import DarkDialog from '../dialog/DarkDialog'
import Account from '../../pages/Account'
import AccountSidebar from './AccountSidebar'
import {
  ACCOUNT_SECTIONS,
  DEFAULT_ACCOUNT_TAB,
  accountSectionLabel,
  isAccountTab,
} from './accountSections'

interface AccountDialogProps {
  open: boolean
  onClose: () => void
  // Section to land on when the dialog is opened from a deep link.
  initialTab?: string
  onTabChange?: (tab: string) => void
}

// Comfortable touch target for the phone list rows, per the platform HIG
// minimum of 44px.
const TOUCH_ROW_HEIGHT = 56

/**
 * Account settings.
 *
 * Wide viewports get the usual sidebar-plus-pane split. Phones can't afford a
 * 240px sidebar next to a settings form, so they get the master/detail pattern
 * every mobile settings app uses instead: a full-screen list of sections that
 * pushes to one section at a time, with a back arrow in the header.
 */
const AccountDialog: FC<AccountDialogProps> = ({ open, onClose, initialTab, onTabChange }) => {
  const theme = useTheme()
  // Below `sm` there is no room for a sidebar alongside content. Short
  // viewports (landscape phones) keep the split but still want every pixel of
  // height, so they go full-screen without changing navigation.
  const isNarrow = useMediaQuery(theme.breakpoints.down('sm'))
  const isShort = useMediaQuery('(max-height: 600px)')
  const fullScreen = isNarrow || isShort

  const [tab, setTab] = useState(isAccountTab(initialTab) ? initialTab! : DEFAULT_ACCOUNT_TAB)
  // On phones the list is the root of the navigation stack; a deep link to a
  // section opens straight into it.
  const [showSection, setShowSection] = useState(isAccountTab(initialTab))

  useEffect(() => {
    if (!open) return
    const deepLinked = isAccountTab(initialTab)
    setTab(deepLinked ? initialTab! : DEFAULT_ACCOUNT_TAB)
    setShowSection(deepLinked)
  }, [open, initialTab])

  const selectTab = useCallback((next: string) => {
    setTab(next)
    setShowSection(true)
    onTabChange?.(next)
  }, [onTabChange])

  const goBackToList = useCallback(() => {
    setShowSection(false)
  }, [])

  // Only the phone layout has a list to go back to.
  const showBackButton = isNarrow && showSection
  const title = showBackButton ? accountSectionLabel(tab) : 'Account'

  return (
    <DarkDialog
      open={open}
      onClose={onClose}
      maxWidth="xl"
      fullWidth
      fullScreen={fullScreen}
      PaperProps={{
        sx: fullScreen
          ? {
              height: '100%',
              maxHeight: '100%',
              m: 0,
              borderRadius: 0,
              border: 'none',
              boxShadow: 'none',
              // The translucent glass surface reads as a floating card. Once
              // the dialog covers the whole screen it has to be opaque, or the
              // page behind it shows through the settings forms.
              backgroundColor: 'background.default',
              background: (t) => t.palette.background.default,
              backdropFilter: 'none',
              WebkitBackdropFilter: 'none',
              // Keep the chrome clear of the notch and the home indicator.
              pt: 'env(safe-area-inset-top)',
              pb: 'env(safe-area-inset-bottom)',
            }
          : {
              height: '90vh',
              maxHeight: '90vh',
            },
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
          px: { xs: 1, sm: 3 },
          py: 1.5,
          flexShrink: 0,
          borderBottom: fullScreen ? '1px solid' : 'none',
          borderColor: 'divider',
        }}
      >
        {showBackButton && (
          <IconButton
            onClick={goBackToList}
            aria-label="Back to account settings"
            sx={{
              color: 'text.secondary',
              '&:hover': { color: 'text.primary' },
            }}
          >
            <ChevronLeft size={22} />
          </IconButton>
        )}
        <Typography
          variant="h6"
          noWrap
          sx={{
            fontWeight: 600,
            flex: 1,
            minWidth: 0,
            pl: showBackButton ? 0 : { xs: 1, sm: 0 },
          }}
        >
          {title}
        </Typography>
        <IconButton
          onClick={onClose}
          aria-label="Close account settings"
          sx={{
            color: '#A0AEC0',
            '&:hover': {
              color: '#F1F1F1',
              backgroundColor: 'rgba(255, 255, 255, 0.08)',
            },
          }}
        >
          <X size={22} />
        </IconButton>
      </Box>
      <DialogContent sx={{ p: 0, display: 'flex', overflow: 'hidden' }}>
        {isNarrow ? (
          showSection ? (
            <Box sx={{ flex: 1, minWidth: 0, overflowY: 'auto', WebkitOverflowScrolling: 'touch' }}>
              <Account tab={tab} />
            </Box>
          ) : (
            <List sx={{ width: '100%', p: 1 }}>
              {ACCOUNT_SECTIONS.map((section) => (
                <ListItemButton
                  key={section.id}
                  data-account-section={section.id}
                  onClick={() => selectTab(section.id)}
                  sx={{
                    borderRadius: '8px',
                    minHeight: TOUCH_ROW_HEIGHT,
                    px: 1.5,
                  }}
                >
                  <ListItemIcon
                    sx={{
                      minWidth: 40,
                      color: 'text.secondary',
                      '& svg': { width: 20, height: 20 },
                    }}
                  >
                    {section.icon}
                  </ListItemIcon>
                  <ListItemText
                    primary={section.label}
                    primaryTypographyProps={{ fontSize: '1rem', fontWeight: 500 }}
                  />
                  <ChevronRight size={18} opacity={0.5} />
                </ListItemButton>
              ))}
            </List>
          )
        ) : (
          <Box sx={{ display: 'flex', height: '100%', width: '100%' }}>
            <Box
              sx={{
                width: 240,
                flexShrink: 0,
                borderRight: '1px solid',
                borderColor: 'divider',
                overflowY: 'auto',
                pr: 1,
              }}
            >
              <AccountSidebar activeTab={tab} onTabChange={selectTab} />
            </Box>
            <Box sx={{ flex: 1, minWidth: 0, overflow: 'auto' }}>
              <Account tab={tab} />
            </Box>
          </Box>
        )}
      </DialogContent>
    </DarkDialog>
  )
}

export default AccountDialog
