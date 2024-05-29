import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  IApp,
} from '../../types'

const CreateHeader: FC<{
  app: IApp,
  paddingX: number,
}> = ({
  app,
  paddingX,
}) => {
  return (
    <Row
      vertical
      center
    >
      <Cell
        sx={{
          pt: 4,
          px: paddingX,
          textAlign: 'center',
        }}
      >
        {
          app.config.helix.image && (
            <Box
              component="img"
              src={ app.config.helix.image }
              sx={{
                maxWidth: '800px',
                maxHeight: '200px',
              }}
            />
          )
        }
        {
          app.config.helix.name && (
            <Typography
              variant="h4"
            >
              { app.config.helix.name }
            </Typography>
          )
        }
        {
          app.config.helix.description && (
            <Typography
              variant="body1"
            >
              { app.config.helix.description }
            </Typography>
          )
        }
      </Cell>
    </Row>
  )
}

export default CreateHeader
