import React, { FC, useState } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Window from './Window'

interface SimpleDeleteConfirmWindowProps {
  title?: string,
  confirmString?: string,
  onCancel: {
    (): void,
  },
  onSubmit: {
    (): void,
  }
}

const SimpleDeleteConfirmWindow: FC<React.PropsWithChildren<SimpleDeleteConfirmWindowProps>> = ({
  title = 'this item',
  onCancel,
  onSubmit,
}) => {
  return (
    <Window
      open
      size="sm"
      title={`Delete ${title}`}
      withCancel
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
      </Box>
    </Window>
  )
}

export default SimpleDeleteConfirmWindow