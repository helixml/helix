import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import { FC, useState } from 'react'
import useSnackbar from '../../hooks/useSnackbar'
import Cell from '../widgets/Cell'
import Row from '../widgets/Row'

import {
  IBotForm,
} from '../../types'

import {
  generateAmusingName,
} from '../../utils/names'

export const ShareSessionBotForm: FC<{
  bot: IBotForm,
}> = ({
  bot,
}) => {
    const snackbar = useSnackbar()
    const [botName, setBotName] = useState(bot ? bot.name : generateAmusingName())

    const handleCopy = () => {
      const url = `${window.location.protocol}//${window.location.hostname}/bot/${botName}`
      navigator.clipboard.writeText(url)
        .then(() => {
          snackbar.success('Copied to clipboard')
        })
        .catch((error) => {
          console.error('Failed to copy:', error)
          snackbar.error('Failed to copy to clipboard')
        })
    }

    return (
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
              value={botName}
              onChange={(e) => setBotName(e.target.value)}
            />
          </Cell>
          <Cell
            sx={{
              ml: 0.5,
              pb: 3,
            }}
          >
            <Button
              variant="outlined"
              color="primary"
              onClick={handleCopy}
            >
              Copy URL
            </Button>
          </Cell>
        </Row>
      </Box>
    )
  }

export default ShareSessionBotForm