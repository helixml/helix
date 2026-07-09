import { FC } from 'react'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import { useHelixOrgBot } from '../services/helixOrgService'

// HelixOrgHumanDetail is the profile view for a human node — a Bot with
// kind=human, i.e. a real person represented in the org graph. It shows who
// the person is, the Helix account it projects, how the org reaches them
// (identity handles), and their responsibility. It deliberately shows NONE
// of the agent surfaces (no Project Desktop, tools, activation): a human
// never runs. The chart and the bot-detail page route human nodes here.
const HelixOrgHumanDetail: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const botId = router.params.bot_id as string | undefined
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Bots', routeName: 'helix_org_bots' })
  const { data, isLoading } = useHelixOrgBot(botId)
  const bot = data?.bot

  const identityRows = Object.entries(bot?.identity ?? {}).filter(([, v]) => !!v)

  return (
    <Page
      breadcrumbTitle={bot?.name || botId || 'Person'}
      breadcrumbs={breadcrumbs}
      organizationId={account.organizationTools.organization?.id}
    >
      <Container maxWidth="md" sx={{ mb: 4, pt: 3 }}>
        {isLoading || !bot ? (
          <LoadingSpinner />
        ) : (
          <Stack spacing={3}>
            <Stack direction="row" alignItems="center" spacing={1.5}>
              <PersonOutlineIcon sx={{ fontSize: 28, color: 'rgba(60,140,210,0.9)' }} />
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="h5">{bot.name || bot.id}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                  {bot.id}
                </Typography>
              </Box>
              <Chip size="small" label="Person" sx={{ ml: 1 }} />
            </Stack>

            <Paper variant="outlined" sx={{ p: 2.5 }}>
              <Typography variant="subtitle2" sx={{ mb: 1 }}>Helix account</Typography>
              {bot.helix_user_id ? (
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{bot.helix_user_id}</Typography>
              ) : (
                <Typography variant="body2" color="text.secondary">Not linked to a Helix account.</Typography>
              )}
            </Paper>

            <Paper variant="outlined" sx={{ p: 2.5 }}>
              <Typography variant="subtitle2" sx={{ mb: 1 }}>Contact channels</Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.5 }}>
                How the org reaches this person. Bots resolve these when they need to message them.
              </Typography>
              {identityRows.length ? (
                <Stack spacing={0.75}>
                  {identityRows.map(([channel, handle]) => (
                    <Stack key={channel} direction="row" spacing={1}>
                      <Typography variant="body2" color="text.secondary" sx={{ minWidth: 80, textTransform: 'capitalize' }}>
                        {channel}
                      </Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{handle}</Typography>
                    </Stack>
                  ))}
                </Stack>
              ) : (
                <Typography variant="body2" color="text.secondary">No contact channels on file yet.</Typography>
              )}
            </Paper>

            <Paper variant="outlined" sx={{ p: 2.5 }}>
              <Typography variant="subtitle2" sx={{ mb: 1 }}>Responsibility</Typography>
              {bot.content ? (
                <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>{bot.content}</Typography>
              ) : (
                <Typography variant="body2" color="text.secondary">None described.</Typography>
              )}
            </Paper>
          </Stack>
        )}
      </Container>
    </Page>
  )
}

export default HelixOrgHumanDetail
