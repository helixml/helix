import React, { FC, useState, useMemo } from 'react'
import Box from '@mui/material/Box'
import { prettyBytes } from '../../utils/format'
import IconButton from '@mui/material/IconButton'
import VisibilityIcon from '@mui/icons-material/Visibility'
import Typography from '@mui/material/Typography'
import SessionBadge from './SessionBadge'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'

import {
  IModelInstanceState,
  ISessionSummary,
} from '../../types'

import {
  getColor,
  getHeadline,
  getSessionHeadline,
  getSummaryCaption,
  getModelInstanceIdleTime,
  shortID,
} from '../../utils/session'

export const ModelInstanceSummary: FC<{
  modelInstance: IModelInstanceState,
  onViewSession: {
    (id: string): void,
  }
}> = ({
  modelInstance,
  onViewSession,
}) => {

  const [ historyViewing, setHistoryViewing ] = useState(false)
  const activeColor = getColor(modelInstance.model_name, modelInstance.mode)

  const jobHistory = useMemo(() => {
    const history = [...modelInstance.job_history]
    history.reverse()
    return history
  }, [
    modelInstance,
  ])

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
                  { getHeadline(modelInstance.model_name, modelInstance.mode, modelInstance.lora_dir) }
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
          <Typography
            sx={{lineHeight: 1}}
            variant="caption"
          >
            <br /><code>{ modelInstance.status }</code>
          </Typography>
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
      <Row>
        <Cell flexGrow={1} />
        {
          historyViewing ? (
            <Cell>
              <ClickLink
                onClick={ () => setHistoryViewing(false) }
              >
                <Typography
                  sx={{
                    lineHeight: 1,
                    textAlign: 'right'
                  }}
                  variant="caption"
                >
                  hide jobs
                </Typography>
              </ClickLink>
            </Cell>
          ) : (
            <Cell>
              <ClickLink
                onClick={ () => setHistoryViewing(true) }
              >
                <Typography
                  sx={{
                    lineHeight: 1,
                    textAlign: 'right'
                  }}
                  variant="caption"
                >
                  view {modelInstance.job_history.length} job{modelInstance.job_history.length == 1 ? '' : 's'}
                </Typography>
              </ClickLink>
            </Cell>
          )
        }
        
      </Row>
      {
        historyViewing && (
          <Box
            sx={{
              maxHeight: '100px',
              overflowY: 'auto',
            }}
          >
            <Typography component="ul" variant="caption" gutterBottom>
              {
                jobHistory.map((job, i) => {
                  return (
                    <li key={ i }>
                      { job.created.split('T')[1].split('.')[0] }&nbsp;&nbsp;
                      <JsonWindowLink
                        sx={{
                          display: 'inline-block',
                          width: '140px',
                        }}
                        data={ job }
                      >
                        { shortID(job.session_id) } : { shortID(job.interaction_id) }
                      </JsonWindowLink>
                      <IconButton
                        size="small"
                        color="primary"
                        sx={{
                          p: 0,
                        }}
                        onClick={ () => {
                          onViewSession(job.session_id)
                        }}
                      >
                        <VisibilityIcon />
                      </IconButton>
                    </li>
                  )
                })
              }
            </Typography>
          </Box>
        )
      }
    </Box>
  )
}

export default ModelInstanceSummary