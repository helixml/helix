import React, { FC } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import ConstructionIcon from '@mui/icons-material/Construction'

import SessionModeSwitch from './SessionModeSwitch'

import {
  ISessionMode,
} from '../../types'

const CreateToolbar: FC<{
  mode: ISessionMode,
  onOpenConfig: () => void,
  onSetMode: (mode: ISessionMode) => void,
}> = ({
  mode,
  onOpenConfig,
  onSetMode,
}) => {
  return (
    <Box component="span" sx={{ display: 'flex', alignItems: 'center' }}>
      <IconButton
        onClick={ onOpenConfig }
      >
        <ConstructionIcon />
      </IconButton>
      <SessionModeSwitch
        mode={ mode }
        onSetMode={ onSetMode }
      />
    </Box>
  )
}

export default CreateToolbar
