import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'

import Page from '../../components/system/Page'
import OAuthProvidersTable from '../../components/dashboard/OAuthProvidersTable'

const AdminOAuthProviders: FC = () => {
  return (
    <Page breadcrumbTitle="OAuth Providers">
      <Container maxWidth="xl" sx={{ mt: 2, height: 'calc(100% - 50px)' }}>
        <Box
          sx={{
            width: '100%',
            height: 'calc(100vh - 150px)',
            overflow: 'auto',
            p: 2,
          }}
        >
          <OAuthProvidersTable />
        </Box>
      </Container>
    </Page>
  )
}

export default AdminOAuthProviders 