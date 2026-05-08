import React, { FC, ReactNode } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogContent from '@mui/material/DialogContent'
import useLightTheme from '../../hooks/useLightTheme'

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
  const lightTheme = useLightTheme()

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth={maxWidth}
      fullWidth={fullWidth}
      PaperProps={{
        sx: {
          backgroundColor: lightTheme.backgroundColor,
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

