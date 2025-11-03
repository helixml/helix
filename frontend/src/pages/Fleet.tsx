import React, { FC, useEffect, useState } from 'react'
import Container from '@mui/material/Container'
import { Box, Tabs, Tab, Typography, Card, CardContent, Button } from '@mui/material'
import { Computer as ComputerIcon, Login as LoginIcon } from '@mui/icons-material'

import Page from '../components/system/Page'
import AgentDashboard from '../components/tasks/AgentDashboard'
import LiveAgentFleetDashboard from '../components/fleet/LiveAgentFleetDashboard'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'

import useAccount from '../hooks/useAccount'
import useApps from '../hooks/useApps'

const Fleet: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const [tabValue, setTabValue] = useState(0)

  useEffect(() => {
    if(account.user) {
      apps.loadApps()
    }
  }, [
    account,
    apps.loadApps,
  ])

  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    setTabValue(newValue)
  }

  // Show logged out state if user is not authenticated
  if (!account.user) {
    return (
      <Page
        breadcrumbTitle="Fleet"
        orgBreadcrumbs={true}
      >
        <Container maxWidth="xl" sx={{ mb: 4 }}>
          <Card sx={{ textAlign: 'center', py: 8 }}>
            <CardContent>
              <ComputerIcon sx={{ fontSize: 64, color: 'text.secondary', mb: 3 }} />
              <Typography variant="h4" component="h1" gutterBottom>
                Fleet Management
              </Typography>
              <Typography variant="h6" color="text.secondary" gutterBottom>
                Manage AI agents and live agent sessions
              </Typography>
              <Typography variant="body1" color="text.secondary" sx={{ mb: 4, maxWidth: 600, mx: 'auto' }}>
                Access the Agent Dashboard to monitor and manage your AI agents and view live agent sessions.
                Sign in to get started.
              </Typography>
              <Box sx={{ display: 'flex', gap: 2, justifyContent: 'center', flexWrap: 'wrap' }}>
                <Button
                  variant="contained"
                  size="large"
                  startIcon={<LoginIcon />}
                  onClick={() => account.setShowLoginWindow(true)}
                  sx={{ minWidth: 200 }}
                >
                  Sign In
                </Button>
                <LaunchpadCTAButton size="large" />
              </Box>
            </CardContent>
          </Card>
        </Container>
      </Page>
    )
  }

  return (
    <Page
      breadcrumbTitle="Fleet"
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl" sx={{ mb: 4 }}>
        <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 3 }}>
          <Tabs value={tabValue} onChange={handleTabChange} aria-label="fleet tabs">
            <Tab label="Agent Dashboard" />
            <Tab label="Live Agent Fleet" />
          </Tabs>
        </Box>

        {tabValue === 0 && <AgentDashboard apps={apps.apps} />}
        {tabValue === 1 && <LiveAgentFleetDashboard />}
      </Container>
    </Page>
  )
}

export default Fleet