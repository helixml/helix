import React, { FC, ReactNode } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogContent from '@mui/material/DialogContent'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../../hooks/useThemeConfig'

export interface ExportDocumentProps {
  open: boolean
  onClose: () => void
  children: ReactNode
  maxWidth?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | false
  fullWidth?: boolean
}

const ExportDocument: FC<ExportDocumentProps> = ({
  open,
  onClose,
  children,
  maxWidth = 'xl',
  fullWidth = true,
}) => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth={maxWidth}
      fullWidth={fullWidth}
      PaperProps={{
        sx: {
          backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor,
          height: '90vh',
          maxHeight: '90vh',
        },
      }}
    >
      <DialogContent
        sx={{
          p: 0,
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {children}
      </DialogContent>
    </Dialog>
  )
}

export default ExportDocument

