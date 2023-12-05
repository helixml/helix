import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import JsonWindowLink from '../widgets/JsonWindowLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  ISession,
} from '../../types'

import useRouter from '../../hooks/useRouter'

export const SessionHeader: FC<{
  session: ISession,
}> = ({
  session,
}) => {
  const {
    navigate,
    setParams,
  } = useRouter()

  return (
    <Row>
      <Cell flexGrow={ 1 }>
        
      </Cell>
      <Cell>
        <Row>
          <Link
            href="/files?path=%2Fsessions"
            onClick={(e) => {
              e.preventDefault()
              navigate('files', {
                path: `/sessions/${session?.id}`
              })
            }}
          >
            <Typography
              sx={{
                fontSize: "small",
                flexGrow: 0,
                textDecoration: 'underline',
              }}
            >
              View Files
            </Typography>
          </Link>
          <Typography
            sx={{
              fontSize: "small",
              color: "gray",
              flexGrow: 0,
              pl: 1,
              pr: 1,
            }}
          >
            |
          </Typography>
          <JsonWindowLink
            data={ session } 
          >
            <Typography
              sx={{
                fontSize: "small",
                flexGrow: 0,
                textDecoration: 'underline',
              }}
            >
              Info
            </Typography>
          </JsonWindowLink>
          {
            session.lora_dir && !session.parent_bot && (
              <>
                <Typography
                  sx={{
                    fontSize: "small",
                    color: "gray",
                    flexGrow: 0,
                    pl: 1,
                    pr: 1,
                  }}
                >
                  |
                </Typography>
                <Link
                  href="/create_bot"
                  onClick={(e) => {
                    e.preventDefault()
                    setParams({
                      editBot: 'yes',
                    })
                  }}
                >
                  <Typography
                    sx={{
                      fontSize: "small",
                      flexGrow: 0,
                      textDecoration: 'underline',
                    }}
                  >
                    { session.parent_bot ? 'Edit' : 'Publish' } Bot
                  </Typography>
                </Link>
              </>
            )
          }
        </Row>
      </Cell>
    </Row>
  )
}

export default SessionHeader