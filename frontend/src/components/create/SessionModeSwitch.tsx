import React, { FC } from 'react'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  ISessionMode,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
} from '../../types'

const SessionModeSwitch: FC<{
  mode: ISessionMode,
  cellWidth?: number,
  onSetMode: (mode: ISessionMode) => void,
}> = ({
  mode,
  cellWidth,
  onSetMode,
}) => {
  return (
    <Row>
      <Cell
        sx={{
          width: cellWidth,
        }}
      >
        <Typography
          sx={{
            color: mode === SESSION_MODE_INFERENCE ? 'text.primary' : 'text.secondary',
            fontWeight: mode === SESSION_MODE_INFERENCE ? 'bold' : 'normal',
            mr: 2,
            ml: 3,
            textAlign: 'right',
            cursor: 'pointer',
          }}
          onClick={() => onSetMode(SESSION_MODE_INFERENCE)}
        >
            Inference
        </Typography>
      </Cell>
      <Cell
        sx={{
          width: cellWidth,
          display: 'flex',
          justifyContent: 'center',
        }}
      >
        <Switch
          checked={mode === SESSION_MODE_FINETUNE}
          onChange={(event: any) => onSetMode(event.target.checked ? SESSION_MODE_FINETUNE : SESSION_MODE_INFERENCE)}
          name="modeSwitch"
          size="medium"
          sx={{
            transform: 'scale(1.6)',
            '& .MuiSwitch-thumb': {
            scale: 0.4,
            },
          }}
        />
      </Cell>
      <Cell
        sx={{
          width: cellWidth,
        }}
      >
        <Typography
          sx={{
            color: mode === SESSION_MODE_FINETUNE ? 'text.primary' : 'text.secondary',
            fontWeight: mode === SESSION_MODE_FINETUNE ? 'bold' : 'normal',
            marginLeft: 2,
            textAlign: 'left',
            cursor: 'pointer',
          }}
          onClick={() => onSetMode(SESSION_MODE_FINETUNE)}
        >
          Fine-tuning
        </Typography>
      </Cell>
    </Row>
  )
}

export default SessionModeSwitch
