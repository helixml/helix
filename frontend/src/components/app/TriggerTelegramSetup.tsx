import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import ListItemIcon from '@mui/material/ListItemIcon'
import Divider from '@mui/material/Divider'
import TextField from '@mui/material/TextField'
import Visibility from '@mui/icons-material/Visibility'
import VisibilityOff from '@mui/icons-material/VisibilityOff'
import IconButton from '@mui/material/IconButton'
import InputAdornment from '@mui/material/InputAdornment'
import TelegramIcon from '@mui/icons-material/Telegram'
import DarkDialog from '../dialog/DarkDialog'

const setupStepsCustomBot = [
  {
    step: 1,
    text: 'Open Telegram and search for @BotFather',
    link: 'https://t.me/BotFather'
  },
  {
    step: 2,
    text: 'Send /newbot to create a new bot'
  },
  {
    step: 3,
    text: 'Follow the prompts to choose a name and username for your bot'
  },
  {
    step: 4,
    text: 'Copy the bot token provided by BotFather'
  },
  {
    step: 5,
    text: 'Paste the bot token below and enable the integration'
  }
]

const setupStepsGlobalBot = [
  {
    step: 1,
    text: 'Your admin has configured a global Telegram bot (TELEGRAM_BOT_TOKEN)'
  },
  {
    step: 2,
    text: 'Enable the Telegram integration and check "Use global bot"'
  },
  {
    step: 3,
    text: 'Add allowed Telegram user IDs to restrict access (optional)'
  },
]

interface TriggerTelegramSetupProps {
  open: boolean
  onClose: () => void
  botToken?: string
  onBotTokenChange?: (token: string) => void
  useGlobalBot?: boolean
}

const TriggerTelegramSetup: FC<TriggerTelegramSetupProps> = ({
  open,
  onClose,
  botToken = '',
  onBotTokenChange,
  useGlobalBot = false
}) => {
  const [showToken, setShowToken] = useState<boolean>(false)

  const steps = useGlobalBot ? setupStepsGlobalBot : setupStepsCustomBot

  return (
    <DarkDialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
    >
      <DialogTitle sx={{ pb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <TelegramIcon sx={{ fontSize: 24, color: '#26A5E4' }} />
          <Typography variant="h6">Telegram Bot Setup Instructions</Typography>
        </Box>
      </DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          {useGlobalBot
            ? 'Your organization has a global Telegram bot configured. Follow these steps to connect:'
            : 'Follow these steps to create a Telegram bot and connect it to your Helix agent:'}
        </Typography>

        <List sx={{ mb: 3 }}>
          {steps.map((step, index) => (
            <React.Fragment key={step.step}>
              <ListItem sx={{ px: 0, flexDirection: 'column', alignItems: 'flex-start' }}>
                <Box sx={{ display: 'flex', alignItems: 'flex-start', width: '100%' }}>
                  <ListItemIcon sx={{ minWidth: 40, mt: 0 }}>
                    <Box
                      sx={{
                        width: 24,
                        height: 24,
                        borderRadius: '50%',
                        mt: 0.7,
                        backgroundColor: 'primary.main',
                        color: 'white',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        fontSize: '0.875rem',
                        fontWeight: 'bold'
                      }}
                    >
                      {step.step}
                    </Box>
                  </ListItemIcon>
                  <ListItemText
                    primary={
                      'link' in step && step.link ? (
                        <Typography
                          component="a"
                          href={step.link}
                          target="_blank"
                          rel="noopener noreferrer"
                          sx={{
                            color: 'primary.main',
                            textDecoration: 'none',
                            '&:hover': {
                              textDecoration: 'underline'
                            }
                          }}
                        >
                          {step.text}
                        </Typography>
                      ) : (
                        <Typography>{step.text}</Typography>
                      )
                    }
                  />
                </Box>

                {!useGlobalBot && step.step === 5 && (
                  <Box sx={{ ml: 6, mt: 2, width: 'calc(100% - 48px)' }}>
                    <Typography variant="body2" color="text.secondary" gutterBottom sx={{ mb: 1 }}>
                      Paste your bot token here:
                    </Typography>
                    <TextField
                      fullWidth
                      size="small"
                      placeholder="123456789:ABCdefGhIjKlMnOpQrStUvWxYz"
                      value={botToken}
                      onChange={(e) => onBotTokenChange?.(e.target.value)}
                      helperText="The token you received from @BotFather"
                      type={showToken ? 'text' : 'password'}
                      autoComplete="new-password"
                      InputProps={{
                        endAdornment: (
                          <InputAdornment position="end">
                            <IconButton
                              aria-label="toggle token visibility"
                              onClick={() => setShowToken(!showToken)}
                              edge="end"
                            >
                              {showToken ? <VisibilityOff /> : <Visibility />}
                            </IconButton>
                          </InputAdornment>
                        ),
                      }}
                    />
                  </Box>
                )}
              </ListItem>
              {index < steps.length - 1 && <Divider sx={{ ml: 6 }} />}
            </React.Fragment>
          ))}
        </List>

        <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider', mb: 2 }}>
          <Typography variant="body2" color="text.secondary">
            <strong>How it works:</strong> Once configured, your bot will respond to all direct messages.
            In group chats, the bot will only respond when mentioned (@botname) or when someone replies to its messages.
          </Typography>
        </Box>

        <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider', mb: 2 }}>
          <Typography variant="body2" color="text.secondary">
            <strong>Commands:</strong>
          </Typography>
          <Box component="ul" sx={{ m: 0, pl: 2, '& li': { mb: 0.5 } }}>
            <Typography component="li" variant="body2" color="text.secondary">
              <strong>/project</strong> - List available projects
            </Typography>
            <Typography component="li" variant="body2" color="text.secondary">
              <strong>/project &lt;name&gt;</strong> - Select a project for this chat
            </Typography>
            <Typography component="li" variant="body2" color="text.secondary">
              <strong>/updates</strong> - Toggle spec task notifications for the linked project
            </Typography>
          </Box>
        </Box>

        <Box sx={{ p: 2, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
          <Typography variant="body2" color="text.secondary">
            <strong>Finding your Telegram User ID:</strong> Forward any message to{' '}
            <Typography
              component="a"
              href="https://t.me/userinfobot"
              target="_blank"
              rel="noopener noreferrer"
              variant="body2"
              sx={{ color: 'primary.main' }}
            >
              @userinfobot
            </Typography>
            {' '}on Telegram. It will reply with your numeric user ID.
          </Typography>
        </Box>
      </DialogContent>
      <DialogActions sx={{ p: 3, pt: 1 }}>
        <Button onClick={onClose} variant="outlined">
          Close
        </Button>
      </DialogActions>
    </DarkDialog>
  )
}

export default TriggerTelegramSetup
