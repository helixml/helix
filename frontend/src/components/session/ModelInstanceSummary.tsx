import React, { FC } from 'react'
import Box from '@mui/material/Box'
import prettyBytes from 'pretty-bytes'
import Typography from '@mui/material/Typography'
import SessionBadge from './SessionBadge'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  IModelInstanceState,
} from '../../types'

import {
  getColor,
  getHeadline,
  getSessionHeadline,
  getSummaryCaption,
  getModelInstanceIdleTime,
} from '../../utils/session'

export const ModelInstanceSummary: FC<{
  modelInstance: IModelInstanceState,
}> = ({
  modelInstance,
}) => {

  const activeColor = getColor(modelInstance.model_name, modelInstance.mode)

  return (
    <Box
      sx={{
        width: '100%',
        p: 1,
        border: `1px solid ${modelInstance.current_session ? activeColor : '#e5e5e5'}`,
        mt: 1,
        mb: 1,
      }}
    >
      <Row>
        <Cell>
          <SessionBadge
            reverse={ modelInstance.current_session ? false : true }
            modelName={ modelInstance.model_name }
            mode={ modelInstance.mode }
          />
        </Cell>
        <Cell sx={{
          ml: 2,
        }}>
          {
            modelInstance.current_session ? (
              <Typography
                sx={{
                  lineHeight: 1,
                  fontWeight: 'bold',
                }}
                variant="body2"
              >
                { getSessionHeadline(modelInstance.current_session) }
              </Typography>
            ) : (
              <>
                <Typography
                  sx={{lineHeight: 1}}
                  variant="body2"
                >
                  { getHeadline(modelInstance.model_name, modelInstance.mode) }
                </Typography>
                <Typography
                  sx={{lineHeight: 1}}
                  variant="caption"
                >
                  { getModelInstanceIdleTime(modelInstance) }
                </Typography>
              </>
              
            )
          }
        </Cell>
        <Cell flexGrow={1} />
        <Cell>
          <Typography
            sx={{lineHeight: 1}}
            variant="body2"
          >
            { prettyBytes(modelInstance.memory) }
          </Typography>
        </Cell>
      </Row>
      {
        modelInstance.current_session && (
          <Row>
            <Cell>
              <Typography
                sx={{lineHeight: 1}}
                variant="caption"
              >
                { getSummaryCaption(modelInstance.current_session) }
              </Typography>
            </Cell>
            <Cell flexGrow={1} />
          </Row>
        )
      }
    </Box>
  )
}

export default ModelInstanceSummary