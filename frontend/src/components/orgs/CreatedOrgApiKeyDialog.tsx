import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import Typography from '@mui/material/Typography'

import MarkdownCodeBlock from '../session/MarkdownCodeBlock'

interface CreatedOrgApiKeyDialogProps {
  apiKey: string
  open: boolean
  onClose: () => void
}

const cliInstall = 'curl -Ls -O https://get.helixml.tech/install.sh && bash install.sh --cli'

const CreatedOrgApiKeyDialog = ({ apiKey, open, onClose }: CreatedOrgApiKeyDialogProps) => {
  const helixURL = window.location.origin
  const cliAuthentication = `export HELIX_URL='${helixURL}'
export HELIX_API_KEY='${apiKey}'

# Verify authentication
helix organization list`

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>API key created</DialogTitle>
      <DialogContent>
        <Alert severity="success" sx={{ mb: 2 }}>
          Your organization API key is ready. Treat it like a password and store it securely.
        </Alert>

        <Box sx={{ mb: 3 }}>
          <Typography variant="subtitle1" fontWeight={600}>
            Copy your API key
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Use the copy button in this code block, then paste the key into your application or secret manager.
          </Typography>
          <MarkdownCodeBlock language="text" defaultWrapped>
            {apiKey}
          </MarkdownCodeBlock>
        </Box>

        <Box sx={{ mb: 3 }}>
          <Typography variant="subtitle1" fontWeight={600}>
            Install the Helix CLI
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Skip this step if the <code>helix</code> command is already installed.
          </Typography>
          <MarkdownCodeBlock language="bash" defaultWrapped>
            {cliInstall}
          </MarkdownCodeBlock>
        </Box>

        <Box>
          <Typography variant="subtitle1" fontWeight={600}>
            Authenticate the Helix CLI
          </Typography>
          <Typography variant="body2" color="text.secondary">
            The CLI reads these environment variables; there is no separate login command. Copy and run this block in your terminal.
          </Typography>
          <MarkdownCodeBlock language="bash" defaultWrapped>
            {cliAuthentication}
          </MarkdownCodeBlock>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} variant="contained">
          Done
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default CreatedOrgApiKeyDialog
