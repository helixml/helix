import { FC, useCallback, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import IconButton from '@mui/material/IconButton'
import InputAdornment from '@mui/material/InputAdornment'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { Copy, Eye, EyeOff, RefreshCcw } from 'lucide-react'

import MarkdownCodeBlock from '../session/MarkdownCodeBlock'
import SettingsPanel from './SettingsPanel'
import useSnackbar from '../../hooks/useSnackbar'
import { copyTextToClipboard } from '../../utils/clipboard'
import {
  useGetUserAPIKeys,
  useRegenerateUserAPIKey,
} from '../../services/userService'

const ApiKeysSettings: FC = () => {
  const snackbar = useSnackbar()

  const { data: apiKeys } = useGetUserAPIKeys()
  const regenerateApiKey = useRegenerateUserAPIKey()

  const [showApiKey, setShowApiKey] = useState(false)
  const [regenerateDialogOpen, setRegenerateDialogOpen] = useState(false)
  const [keyToRegenerate, setKeyToRegenerate] = useState<string>('')

  const handleCopy = useCallback((text: string) => {
    copyTextToClipboard(text)
      .then(() => {
        snackbar.success('Copied to clipboard')
      })
      .catch((error) => {
        console.error('Failed to copy:', error)
        snackbar.error('Failed to copy to clipboard')
      })
  }, [snackbar])

  const handleRegenerateApiKey = useCallback((key: string) => {
    setKeyToRegenerate(key)
    setRegenerateDialogOpen(true)
  }, [])

  const handleCancelRegenerate = useCallback(() => {
    setRegenerateDialogOpen(false)
    setKeyToRegenerate('')
  }, [])

  const handleConfirmRegenerate = useCallback(async () => {
    try {
      await regenerateApiKey.mutateAsync(keyToRegenerate)
      snackbar.success('API key regenerated successfully')
      setRegenerateDialogOpen(false)
      setKeyToRegenerate('')
    } catch (error) {
      console.error('Failed to regenerate API key:', error)
      snackbar.error('Failed to regenerate API key')
    }
  }, [regenerateApiKey, keyToRegenerate, snackbar])

  const cliInstall = `curl -Ls -O https://get.helixml.tech/install.sh && bash install.sh --cli`
  const firstApiKey = apiKeys && apiKeys.length > 0 ? (apiKeys[0].key || '') : ''
  const cliLogin = `export HELIX_URL=${window.location.protocol}//${window.location.host}
export HELIX_API_KEY=${firstApiKey}
`

  return (
    <>
      <SettingsPanel sx={{ mb: 0 }}>
        <Box sx={{ mb: 3 }}>
          <Typography variant="h6" sx={{ mb: 2 }}>API Key</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Specify your key as a header 'Authorization: Bearer &lt;token&gt;' with every request
          </Typography>

          {apiKeys && apiKeys.length > 0 ? (
            apiKeys.map((apiKey) => (
              <Box key={apiKey.key} sx={{ mb: 2 }}>
                <TextField
                  fullWidth
                  label="API Key"
                  value={apiKey.key}
                  type={showApiKey ? 'text' : 'password'}
                  variant="outlined"
                  InputProps={{
                    readOnly: true,
                    endAdornment: (
                      <InputAdornment position="end">
                        <IconButton
                          onClick={() => setShowApiKey(!showApiKey)}
                          aria-label={showApiKey ? 'Hide API key' : 'Show API key'}
                          edge="end"
                          sx={{ mr: 0.25 }}
                        >
                          {showApiKey ? <EyeOff size={18} /> : <Eye size={18} />}
                        </IconButton>
                        <IconButton
                          onClick={() => handleCopy(apiKey.key || '')}
                          aria-label="Copy API key"
                          edge="end"
                          sx={{ mr: 0.25 }}
                        >
                          <Copy size={18} />
                        </IconButton>
                        <IconButton
                          onClick={() => handleRegenerateApiKey(apiKey.key || '')}
                          aria-label="Regenerate API key"
                          edge="end"
                        >
                          <RefreshCcw size={18} />
                        </IconButton>
                      </InputAdornment>
                    ),
                  }}
                />
              </Box>
            ))
          ) : (
            <Box sx={{ mb: 2, p: 2, border: '1px dashed', borderColor: 'divider', borderRadius: 1 }}>
              <Typography variant="body2" color="text.secondary" align="center">
                No API keys available. Creating a new key...
              </Typography>
            </Box>
          )}
        </Box>

        <Box sx={{ mb: 3 }}>
          <Typography variant="h6" sx={{ mb: 2 }}>CLI Installation</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Install the Helix CLI to interact with the API from your terminal
          </Typography>
          <MarkdownCodeBlock language="bash" defaultWrapped>
            {cliInstall}
          </MarkdownCodeBlock>
        </Box>

        <Box>
          <Typography variant="h6" sx={{ mb: 2 }}>CLI Authentication</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Set your authentication credentials for the CLI
          </Typography>

          {apiKeys && apiKeys.length > 0 ? (
            <MarkdownCodeBlock language="bash" defaultWrapped>
              {cliLogin}
            </MarkdownCodeBlock>
          ) : (
            <Box sx={{ p: 2, border: '1px dashed', borderColor: 'divider', borderRadius: 1 }}>
              <Typography variant="body2" color="text.secondary" align="center">
                CLI authentication will be available once API key is created.
              </Typography>
            </Box>
          )}
        </Box>
      </SettingsPanel>

      <Dialog
        open={regenerateDialogOpen}
        onClose={handleCancelRegenerate}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Regenerate API Key</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to regenerate your API key? This will invalidate the current key and create a new one.
            Any applications or scripts using the current key will need to be updated with the new key.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCancelRegenerate} disabled={regenerateApiKey.isPending}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirmRegenerate}
            color="error"
            variant="contained"
            disabled={regenerateApiKey.isPending}
          >
            {regenerateApiKey.isPending ? 'Regenerating...' : 'Regenerate Key'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default ApiKeysSettings
