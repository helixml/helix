// Shared right-side drawer chrome for helix-org create/edit forms
// (bots, topics, processors). Keeps title bar + close + width consistent
// so every create flow feels like the same surface.

import { FC, ReactNode, useEffect } from 'react'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import CloseIcon from '@mui/icons-material/Close'

export type HelixOrgSideDrawerProps = {
  open: boolean
  onClose: () => void
  title: string
  /** Paper width in px. Default 460 (matches processor form). */
  width?: number
  headerAction?: ReactNode
  /** Keep the underlying chart interactive while this drawer is open. */
  allowInteractionBehind?: boolean
  /** Close persistent drawers on Escape. Temporary drawers handle this through MUI. */
  closeOnEscape?: boolean
  children: ReactNode
}

const HelixOrgSideDrawer: FC<HelixOrgSideDrawerProps> = ({
  open,
  onClose,
  title,
  width = 460,
  headerAction,
  allowInteractionBehind = false,
  closeOnEscape = false,
  children,
}) => {
  useEffect(() => {
    if (!open || !closeOnEscape) return

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented) return
      // Let a nested menu/select/dialog consume Escape before closing the drawer.
      if (document.querySelector('.MuiPopover-root, .MuiDialog-root')) return
      onClose()
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [closeOnEscape, onClose, open])

  return (
    <Drawer
      anchor="right"
      variant={allowInteractionBehind ? 'persistent' : 'temporary'}
      open={open}
      onClose={onClose}
      hideBackdrop={allowInteractionBehind}
      ModalProps={allowInteractionBehind ? {
        disableAutoFocus: true,
        disableEnforceFocus: true,
        disableRestoreFocus: true,
      } : undefined}
      PaperProps={{ sx: { backgroundImage: 'none' } }}
    >
      <Box
        sx={{
          p: 2.5,
          width,
          maxWidth: '100vw',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          boxSizing: 'border-box',
        }}
      >
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          sx={{ mb: 2, flexShrink: 0 }}
        >
          <Typography variant="h6">{title}</Typography>
          <Stack direction="row" alignItems="center" spacing={0.5}>
            {headerAction}
            <IconButton size="small" onClick={onClose} aria-label="Close">
              <CloseIcon />
            </IconButton>
          </Stack>
        </Stack>
        <Box sx={{ flex: 1, overflow: 'auto', minHeight: 0 }}>{children}</Box>
      </Box>
    </Drawer>
  )
}

export default HelixOrgSideDrawer
