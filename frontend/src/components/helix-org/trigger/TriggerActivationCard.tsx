import { FC } from 'react'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import MarkdownCodeBlock from '../../session/MarkdownCodeBlock'
import type { TransportResolvedActivation } from '../../../api/api'

const curlFor = (activation: TransportResolvedActivation): string =>
  [
    `curl -X ${activation.verb || 'POST'} '${activation.url}' \\`,
    "  -H 'Authorization: Bearer $HELIX_API_KEY' \\",
    "  -H 'Content-Type: application/json' \\",
    `  -d '{"hello":"world"}'`,
  ].join('\n')

const TriggerActivationCard: FC<{
  activation?: TransportResolvedActivation
  density?: 'compact' | 'full'
}> = ({ activation, density = 'full' }) => {
  if (!activation?.summary && !activation?.url && !activation?.address) return null
  const full = density === 'full'

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>How to fire this</Typography>
      <Stack spacing={1.5}>
        {activation.summary && <Typography variant="body2">{activation.summary}</Typography>}

        {activation.url && (
          <Stack spacing={0.5}>
            <Typography variant="caption" color="text.secondary">
              {activation.verb ? `${activation.verb} to this URL` : 'URL'}
            </Typography>
            <MarkdownCodeBlock language="text" defaultWrapped>{activation.url}</MarkdownCodeBlock>
            {activation.auth_header && (
              <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                {activation.auth_header}
              </Typography>
            )}
          </Stack>
        )}

        {activation.address && (
          <Stack spacing={0.5}>
            <Typography variant="caption" color="text.secondary">Address</Typography>
            <MarkdownCodeBlock language="text" defaultWrapped>{activation.address}</MarkdownCodeBlock>
          </Stack>
        )}

        {full && activation.url && (
          <Stack spacing={0.5}>
            <Typography variant="caption" color="text.secondary">Example</Typography>
            <MarkdownCodeBlock language="bash">{curlFor(activation)}</MarkdownCodeBlock>
          </Stack>
        )}

        {full && activation.note && (
          <Typography variant="caption" color="text.secondary">{activation.note}</Typography>
        )}
      </Stack>
    </Paper>
  )
}

export default TriggerActivationCard
