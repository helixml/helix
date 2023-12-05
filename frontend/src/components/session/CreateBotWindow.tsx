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
  IBotForm,
} from '../../types'

import {
  generateAmusingName,
} from '../../utils/names'

export const CreateBotWindow: FC<{
  bot?: IBotForm,
  onSubmit: {
    (bot: IBotForm): void,
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
        const newBot: IBotForm = Object.assign({}, bot, {
          name: botName,
        })
        onSubmit(newBot)
      }}
    >
      <Box
        sx={{
          p: 1,
        }}
      >
        <Typography
          variant="body1"
          sx={{
            mb: 3,
          }}
        >
          Make your bot available to others by publishing it. You can share the URL with anyone, and they will be able to interact with your bot.
        </Typography>
        <Row>
          <Cell
            sx={{
              pr: 0.5,
              pb: 2.5,
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
              helperText="Name can only include letters, numbers, and dashes"
              value={ botName }
              onChange={ (e) => setBotName(e.target.value) }
            />
          </Cell>
          <Cell
            sx={{
              ml: 0.5,
              pb: 3,
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