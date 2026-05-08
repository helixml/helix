import React, { FC, useCallback, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import Grid from '@mui/material/Grid'
import IconButton from '@mui/material/IconButton'
import InputAdornment from '@mui/material/InputAdornment'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { Copy, Eye, EyeOff, RefreshCcw } from 'lucide-react'

import { Prism as SyntaxHighlighterPrism } from 'react-syntax-highlighter'
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'

import useSnackbar from '../../hooks/useSnackbar'
import useThemeConfig from '../../hooks/useThemeConfig'
import {
  useGetUserAPIKeys,
  useRegenerateUserAPIKey,
} from '../../services/userService'

const SyntaxHighlighter = SyntaxHighlighterPrism as unknown as React.FC<any>

const ApiKeysSettings: FC = () => {
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()

  const { data: apiKeys } = useGetUserAPIKeys()
  const regenerateApiKey = useRegenerateUserAPIKey()

  const [showApiKey, setShowApiKey] = useState(false)
  const [regenerateDialogOpen, setRegenerateDialogOpen] = useState(false)
  const [keyToRegenerate, setKeyToRegenerate] = useState<string>('')

  const handleCopy = useCallback((text: string) => {
    navigator.clipboard.writeText(text)
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
      <Grid container spacing={2} sx={{ mb: 2, backgroundColor: themeConfig.darkPanel, p: 2, borderRadius: 2 }}>
        <Grid item xs={12}>
          <Box sx={{ mb: 3 }}>
            <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>API Key</Typography>
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
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            onClick={() => setShowApiKey(!showApiKey)}
                            edge="end"
                            sx={{ mr: 0.25 }}
                          >
                            {showApiKey ? <EyeOff size={18} /> : <Eye size={18} />}
                          </IconButton>
                          <IconButton
                            onClick={() => handleCopy(apiKey.key || '')}
                            edge="end"
                            sx={{ mr: 0.25 }}
                          >
                            <Copy size={18} />
                          </IconButton>
                          <IconButton
                            onClick={() => handleRegenerateApiKey(apiKey.key || '')}
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
            <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>CLI Installation</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Install the Helix CLI to interact with the API from your terminal
            </Typography>

            <Box sx={{ position: 'relative' }}>
              <Box sx={{ position: 'absolute', right: 8, top: 8, zIndex: 1 }}>
                <Button
                  size="small"
                  onClick={() => handleCopy(cliInstall)}
                  startIcon={<Copy size={16} />}
                  sx={{
                    backgroundColor: 'rgba(0, 0, 0, 0.6)',
                    '&:hover': {
                      backgroundColor: 'rgba(0, 0, 0, 0.8)',
                    },
                  }}
                >
                  Copy
                </Button>
              </Box>
              <SyntaxHighlighter
                language="bash"
                style={oneDark}
                customStyle={{
                  margin: 0,
                  borderRadius: '4px',
                  fontSize: '0.8rem',
                }}
              >
                {cliInstall}
              </SyntaxHighlighter>
            </Box>
          </Box>

          <Box>
            <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>CLI Authentication</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Set your authentication credentials for the CLI
            </Typography>

            {apiKeys && apiKeys.length > 0 ? (
              apiKeys.map((apiKey) => (
                <Box key={apiKey.key} sx={{ position: 'relative' }}>
                  <Box sx={{ position: 'absolute', right: 8, top: 8, zIndex: 1 }}>
                    <Button
                      size="small"
                      onClick={() => handleCopy(cliLogin)}
                      startIcon={<Copy size={16} />}
                      sx={{
                        backgroundColor: 'rgba(0, 0, 0, 0.6)',
                        '&:hover': {
                          backgroundColor: 'rgba(0, 0, 0, 0.8)',
                        },
                      }}
                    >
                      Copy
                    </Button>
                  </Box>
                  <SyntaxHighlighter
                    language="bash"
                    style={oneDark}
                    customStyle={{
                      margin: 0,
                      borderRadius: '4px',
                      fontSize: '0.8rem',
                    }}
                  >
                    {cliLogin}
                  </SyntaxHighlighter>
                </Box>
              ))
            ) : (
              <Box sx={{ p: 2, border: '1px dashed', borderColor: 'divider', borderRadius: 1 }}>
                <Typography variant="body2" color="text.secondary" align="center">
                  CLI authentication will be available once API key is created.
                </Typography>
              </Box>
            )}
          </Box>
        </Grid>
      </Grid>

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
