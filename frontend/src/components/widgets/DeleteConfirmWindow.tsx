import React, { FC, ReactNode, useState, useCallback, useEffect } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'

interface DeleteConfirmWindowProps {
  open?: boolean,
  title?: string,
  submitTitle?: string,
  confirmString?: string,
  children?: ReactNode,
  onCancel: () => void,
  onSubmit: () => void | Promise<void>,
}

const DeleteConfirmWindow: FC<DeleteConfirmWindowProps> = ({
  open = true,
  title = 'this item',
  submitTitle = 'Delete',
  confirmString = 'delete',
  children,
  onCancel,
  onSubmit,
}) => {
  const [confirmValue, setConfirmValue] = useState('')
  const [loading, setLoading] = useState(false)

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if(confirmValue == confirmString && !loading) {
        handleSubmit()
      }
      event.preventDefault()
    }
  }, [
    confirmValue,
    confirmString,
    loading,
  ])

  const handleSubmit = async () => {
    try {
      setLoading(true)
      await onSubmit()
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if(!open) {
      setConfirmValue('')
    }
  }, [open])

  return (
    <Dialog
      open={open}
      onClose={onCancel}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>
        {submitTitle} {title}
      </DialogTitle>
      <DialogContent>
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'flex-start',
            width: '100%',
            mt: 2,
          }}
        >
          <Box
            sx={{
              width: '100%',
              padding: 1,
            }}
          >
            {children ? (
              children
            ) : (
              <Typography>
                Are you sure you want to delete {title}?
              </Typography>
            )}
          </Box>
          <Box
            sx={{
              width: '100%',
              padding: 1,
            }}
          >
            <Typography>
              Please enter the word <strong>{confirmString}</strong> into the text box below to confirm...
            </Typography>
          </Box>
          <Box
            sx={{
              width: '100%',
              padding: 1,
            }}
          >
            <TextField
              autoFocus
              label={`enter the word ${confirmString}`}
              value={confirmValue}
              fullWidth
              onChange={(e) => setConfirmValue(e.target.value)}
              onKeyDown={handleKeyDown}
              disabled={loading}
            />
          </Box>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button
          onClick={onCancel}
          disabled={loading}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          color="primary"
          disabled={loading || confirmValue !== confirmString}
        >
          {loading ? `${submitTitle}...` : submitTitle}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default DeleteConfirmWindow