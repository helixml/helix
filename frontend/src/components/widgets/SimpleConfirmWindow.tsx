import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Window from './Window'

const SimpleConfirmWindow: FC<React.PropsWithChildren<{
  title?: string,
  message?: string,
  confirmTitle?: string,
  cancelTitle?: string,
  onCancel: {
    (): void,
  },
  onSubmit: {
    (): void,
  }
}
>> = ({
  title,
  message = 'Are you sure?',
  confirmTitle = 'Confirm',
  cancelTitle = 'Cancel',
  onCancel,
  onSubmit,
}) => {
  return (
    <Window
      open
      size="sm"
      title={ title }
      withCancel
      submitTitle={ confirmTitle }
      cancelTitle={ cancelTitle }
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
            { message }
          </Typography>
        </Box>
      </Box>
    </Window>
  )
}

export default SimpleConfirmWindow