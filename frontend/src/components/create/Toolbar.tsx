import React, { FC } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import ConstructionIcon from '@mui/icons-material/Construction'

import Cell from '../widgets/Cell'
import Row from '../widgets/Row'
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
    <Row>
      <Cell>

      </Cell>
      <Cell grow>
        
      </Cell>
      <Cell>
        <IconButton
          onClick={ onOpenConfig }
        >
          <ConstructionIcon />
        </IconButton>
      </Cell>
      <Cell>
        <SessionModeSwitch
          mode={ mode }
          onSetMode={ onSetMode }
        />
      </Cell>
    </Row>
  )
}

export default CreateToolbar
