import React, { FC, useState, useCallback } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Window from './Window'

interface DeleteConfirmWindowProps {
  title?: string,
  confirmString?: string,
  onCancel: {
    (): void,
  },
  onSubmit: {
    (): void,
  }
}

const DeleteConfirmWindow: FC<React.PropsWithChildren<DeleteConfirmWindowProps>> = ({
  title = 'this item',
  confirmString = 'delete',
  onCancel,
  onSubmit,
}) => {
  const [confirmValue, setConfirmValue] = useState('')

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter') {
      if(confirmValue == confirmString) {
        onSubmit()
      }
      event.preventDefault()
    }
  }, [
    confirmValue,
    confirmString,
    onSubmit,
  ])

  return (
    <Window
      open
      size="sm"
      title={`Delete ${title}`}
      withCancel
      disabled={ confirmValue != confirmString }
      submitTitle="Confirm"
      onCancel={ onCancel }
      onSubmit={ onSubmit }
    >
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'flex-start',
          width: '100%',
        }}
      >
        <Box
          sx={{
            width: '100%',
            padding:1,
          }}
        >
          <Typography>
            Are you sure you want to delete {title}?
          </Typography>
        </Box>
        <Box
          sx={{
            width: '100%',
            padding:1,
          }}
        >
          <Typography>
            Please enter the word <strong>{confirmString}</strong> into the text box below to confirm...
          </Typography>
        </Box>
        <Box
          sx={{
            width: '100%',
            padding:1,
          }}
        >
          <TextField
            autoFocus
            label={ `enter the word ${confirmString}` }
            value={ confirmValue }
            fullWidth
            onChange={ (e) => setConfirmValue(e.target.value) }
            onKeyDown={handleKeyDown}
          />
        </Box>
      </Box>
    </Window>
  )
}

export default DeleteConfirmWindow