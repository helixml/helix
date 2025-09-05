import React, { FC, useEffect } from 'react'
import Container from '@mui/material/Container'

import Page from '../components/system/Page'
import AgentDashboard from '../components/tasks/AgentDashboard'

import useAccount from '../hooks/useAccount'
import useApps from '../hooks/useApps'

const Fleet: FC = () => {
  const account = useAccount()
  const apps = useApps()

  useEffect(() => {
    if(account.user) {
      apps.loadApps()
    }
  }, [
    account,
    apps.loadApps,
  ])



  return (
    <Page
      breadcrumbTitle="Fleet"
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl" sx={{ mb: 4 }}>
        <AgentDashboard apps={apps.apps} />
      </Container>
    </Page>
  )
}

export default Fleet