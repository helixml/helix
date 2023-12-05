import React, { FC, useState } from 'react'
import {CopyToClipboard} from 'react-copy-to-clipboard'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import Window from '../widgets/Window'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import useSnackbar from '../../hooks/useSnackbar'

import {
  IBotForm,
} from '../../types'

import {
  generateAmusingName,
} from '../../utils/names'

export const AddDocumentsWindow: FC<{
  onSubmit: {
    (bot: IBotForm): void,
  },
  onCancel: {
    (): void,
  },
}> = ({
  onSubmit,
  onCancel,
}) => {
  return (
    <Window
      open
      size="md"
      title={`Add Documents`}
      withCancel
      submitTitle="Upload"
      onSubmit={ () => {
        
      } }
      onCancel={ onCancel }
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
        
      </Box>
    </Window>
  )
}

export default AddDocumentsWindow