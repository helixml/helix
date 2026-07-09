import { FC, useMemo } from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { useTheme } from '@mui/material/styles'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import { useListHelixOrgBots, BotDTO } from '../services/helixOrgService'

// HelixOrgPeople lists the real people in the org — the human nodes
// (Bot kind=human), one per org member. People are NOT on the agent chart
// (that graph is for agents); they live here, where you see and (soon)
// configure their role/responsibility and contact handles. Membership drives
// the list — you don't add people here, they appear when they join the org.
const HelixOrgPeople: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const theme = useTheme()
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'People', routeName: 'helix_org_people' })
  const orgSlug = router.params.org_id as string | undefined

  const { data, isLoading } = useListHelixOrgBots()
  const people = (data ?? []).filter((b) => b.kind === 'human')

  const openPerson = (botId: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_human_detail', { org_id: orgSlug, bot_id: botId })
  }

  const channel = (b: BotDTO, key: string) => (
    <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
      {b.identity?.[key] || '—'}
    </Typography>
  )

  const tableData = useMemo(() => people.map((b) => ({
    id: b.id,
    _data: b,
    name: (
      <Typography variant="body1">
        <a
          style={{
            textDecoration: 'none',
            fontWeight: 'bold',
            color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
          }}
          href="#"
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); openPerson(b.id ?? '') }}
        >
          {b.name || b.id}
        </a>
      </Typography>
    ),
    email: channel(b, 'email'),
    github: channel(b, 'github'),
    slack: channel(b, 'slack'),
    responsibility: (
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 360 }}
      >
        {(b.content || '').split('\n').find((l) => l.trim() !== '')?.slice(0, 80) || '—'}
      </Typography>
    ),
  })), [people, theme])

  return (
    <Page
      breadcrumbTitle="People"
      breadcrumbs={breadcrumbs}
      organizationId={account.organizationTools.organization?.id}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>People</Typography>
            <Typography variant="body2" color="text.secondary">
              The real people in this org — one per member. People aren't on the agent chart;
              they live here. Each holds the contact handles (Slack, GitHub, email) the org uses
              to reach them and a responsibility description bots resolve for "who owns X". People
              appear automatically when they join the org.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : people.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary">
                No people yet — they appear here when members join the org.
              </Typography>
            </Box>
          ) : (
            <SimpleTable
              authenticated={true}
              fields={[
                { name: 'name', title: 'Name' },
                { name: 'email', title: 'Email' },
                { name: 'github', title: 'GitHub' },
                { name: 'slack', title: 'Slack' },
                { name: 'responsibility', title: 'Responsibility' },
              ]}
              data={tableData}
            />
          )}
        </Stack>
      </Container>
    </Page>
  )
}

export default HelixOrgPeople
