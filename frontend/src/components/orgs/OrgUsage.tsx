import { FC } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import Paper from '@mui/material/Paper'
import { BarChart as ChartIcon } from 'lucide-react'

import Page from '../system/Page'

const OrgUsage: FC = () => {
  return (
    <Page
      breadcrumbTitle="Usage"
      breadcrumbParent={{
        title: 'Organizations',
        routeName: 'orgs',
        useOrgRouter: false,
      }}
      breadcrumbShowHome={true}
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3, p: 2, maxWidth: 880 }}>
          <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 3 }}>
            <ChartIcon size={28} />
            <Typography variant="h5" component="h2">
              Usage
            </Typography>
          </Stack>
          <Paper
            variant="outlined"
            sx={{
              p: 3,
              borderStyle: 'dashed',
              backgroundColor: 'transparent',
            }}
          >
            <Typography variant="body1" sx={{ mb: 1, fontWeight: 600 }}>
              Coming soon
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              The aggregate LLM token usage and cost breakdown for this
              organization is in design. It will show tokens and
              estimated cost grouped by user, project/app/agent,
              session, and model/provider, with date-range filters and
              CSV/JSON export.
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Visible whether or not billing is enabled. Design:{' '}
              <code>design/2026-05-14-aggregate-usage-dashboard.md</code>
            </Typography>
          </Paper>
        </Box>
      </Container>
    </Page>
  )
}

export default OrgUsage
