import React, { FC, useState } from 'react'
import {CopyToClipboard} from 'react-copy-to-clipboard'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Switch from '@mui/material/Switch'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import useSnackbar from '../../hooks/useSnackbar'

import {
  ISession,
} from '../../types'

export const ShareSessionShareForm: FC<{
  session: ISession,
  shared: boolean,
  onChange: {
    (shared: boolean): void,
  },
}> = ({
  session,
  shared,
  onChange,
}) => {
  const snackbar = useSnackbar()
  const url = `${window.location.protocol}//${window.location.hostname}/session/${session.id}`
  return (
    <Box
      sx={{
        p: 1,
      }}
    >
      <Row
        sx={{
          mb: 3,
        }}
      >
        <Cell
          sx={{
            minWidth: '200px'
          }}
        >
          <FormControlLabel
            control={
              <Switch
                checked={ shared }
                onChange={ (e) => {
                  onChange(e.target.checked)
                }}
              />
            }
            label="Share Session?"
          />
        </Cell>
        <Cell grow>
          {
            shared ? (
              <Typography
                variant="body1"
              >
                This session is shared with other users (and is public).  Give them the following URL and they can continue the conversation from this point (but in their own account).
              </Typography>
            ) : (
              <Typography
                variant="body1"
              >
                Share this session with other users (this will make it public).  They will be able to continue the conversation from this point (but in their own account).
              </Typography>
            )
          }
        </Cell>
      </Row>
      {
        shared && (
          <Row>
            <Cell
              sx={{
                pr: 0.5,
                pb: 2.5,
              }}
              flexGrow={1}
            >
              <Typography variant="h6" sx={{
                backgroundColor: '#000',
                p: 2,
                border: '1px solid #999',
                borderRadius: 5,
              }}>
                { url }
              </Typography>
            </Cell>
            <Cell
              sx={{
                ml: 0.5,
                pb: 3,
              }}
            >
              <CopyToClipboard
                text={ url }
                onCopy={ () => {
                  snackbar.success('Copied to clipboard')
                }}
              >
                <Button
                  variant="outlined"
                  color="primary"
                  onClick={ () => {
                    
                  }}
                >
                  Copy URL
                </Button>
              </CopyToClipboard>
            </Cell>
          </Row>
        )
      }
    </Box>
  )
}

export default ShareSessionShareForm