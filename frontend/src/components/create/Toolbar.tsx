import React, { FC } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import ConstructionIcon from '@mui/icons-material/Construction'

import Cell from '../widgets/Cell'
import Row from '../widgets/Row'
import SessionModeSwitch from './SessionModeSwitch'
import ModelPicker from './ModelPicker'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
} from '../../types'

const CreateToolbar: FC<{
  mode: ISessionMode,
  type: ISessionType,
  model: string,
  onOpenConfig: () => void,
  onSetMode: (mode: ISessionMode) => void,
  onSetModel: (model: string) => void,
}> = ({
  mode,
  type,
  model,
  onOpenConfig,
  onSetMode,
  onSetModel,
}) => {
  return (
    <Row>
      <Cell>
        {
          mode == SESSION_MODE_INFERENCE && type == SESSION_TYPE_TEXT && (
            <ModelPicker
              model={ model }
              onSetModel={ onSetModel }
            />
          )
        }
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
