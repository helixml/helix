import React, { FC } from 'react'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  ISessionType,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../../types'

const SessionTypeSwitch: FC<{
  type: ISessionType,
  cellWidth?: number,
  onSetType: (type: ISessionType) => void,
}> = ({
  type,
  cellWidth,
  onSetType,
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
            color: type === SESSION_TYPE_TEXT ? 'text.primary' : 'text.secondary',
            fontWeight: type === SESSION_TYPE_TEXT ? 'bold' : 'normal',
            mr: 2,
            ml: 3,
            textAlign: 'right',
            cursor: 'pointer',
          }}
          onClick={() => onSetType(SESSION_TYPE_TEXT)}
        >
          Text
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
          checked={type === SESSION_TYPE_IMAGE}
          onChange={(event: any) => onSetType(event.target.checked ? SESSION_TYPE_IMAGE : SESSION_TYPE_TEXT)}
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
            color: type === SESSION_TYPE_IMAGE ? 'text.primary' : 'text.secondary',
            fontWeight: type === SESSION_TYPE_IMAGE ? 'bold' : 'normal',
            marginLeft: 2,
            textAlign: 'left',
            cursor: 'pointer',
          }}
          onClick={() => onSetType(SESSION_TYPE_IMAGE)}
        >
          Image
        </Typography>
      </Cell>
    </Row>
  )
}

export default SessionTypeSwitch
