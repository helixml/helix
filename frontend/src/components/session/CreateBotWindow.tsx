import React, { FC, useState } from 'react'
import {CopyToClipboard} from 'react-copy-to-clipboard'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import Window from '../widgets/Window'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import useSnackbar from '../../hooks/useSnackbar'

import {
  IBot,
} from '../../types'

import {
  generateAmusingName,
} from '../../utils/names'

export const CreateBotWindow: FC<{
  bot?: IBot,
  onSubmit: {
    (bot: IBot): void,
  },
  onCancel: {
    (): void,
  },
}> = ({
  bot,
  onSubmit,
  onCancel,
}) => {
  const snackbar = useSnackbar()
  const [ botName, setBotName ] = useState( bot ? bot.name : generateAmusingName() )
  return (
    <Window
      open
      title={ bot ? `Edit Bot ${bot.name}` : 'Publish Bot' }
      size="md"
      withCancel
      submitTitle={ bot ? 'Update Bot' : 'Publish Bot' }
      onCancel={onCancel}
      onSubmit={ () => {

      }}
    >
      <Box
        sx={{
          p: 1,
        }}
      >
        <Row>
          <Cell
            sx={{
              pr: 0.5,
            }}
          >
            <Typography variant="h6">
              {`${window.location.protocol}//${window.location.hostname}/bot/`}
            </Typography>
          </Cell>
          <Cell flexGrow={1}>
            <TextField
              id="textEntry"
              fullWidth
              label="Enter bot name"
              value={ botName }
              onChange={ (e) => setBotName(e.target.value) }
            />
          </Cell>
          <Cell
            sx={{
              ml: 0.5,
            }}
          >
            <CopyToClipboard
              text={`${window.location.protocol}//${window.location.hostname}/bot/${botName}`}
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
      </Box>
    </Window>
  )
}

export default CreateBotWindow