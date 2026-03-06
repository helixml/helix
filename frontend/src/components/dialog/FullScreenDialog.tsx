import React from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import DialogContent from '@mui/material/DialogContent'
import CloseIcon from '@mui/icons-material/Close'
import DarkDialog from './DarkDialog'

interface FullScreenDialogProps {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
}

const FullScreenDialog: React.FC<FullScreenDialogProps> = ({
  open,
  onClose,
  title,
  children,
}) => {
  return (
    <DarkDialog
      open={open}
      onClose={onClose}
      maxWidth="xl"
      fullWidth
      PaperProps={{
        sx: {
          height: '90vh',
          maxHeight: '90vh',
        },
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 3,
          py: 1.5,
          borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
          flexShrink: 0,
        }}
      >
        <Typography variant="h6" sx={{ fontWeight: 600 }}>
          {title}
        </Typography>
        <IconButton
          onClick={onClose}
          sx={{
            color: '#A0AEC0',
            '&:hover': {
              color: '#F1F1F1',
              backgroundColor: 'rgba(255, 255, 255, 0.08)',
            },
          }}
        >
          <CloseIcon />
        </IconButton>
      </Box>
      <DialogContent
        sx={{
          p: 0,
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'auto',
        }}
      >
        {children}
      </DialogContent>
    </DarkDialog>
  )
}

export default FullScreenDialog
