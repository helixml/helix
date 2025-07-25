import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'

import Page from '../../components/system/Page'
import ProviderEndpointsTable from '../../components/dashboard/ProviderEndpointsTable'

const AdminProviders: FC = () => {
  return (
    <Page breadcrumbTitle="Inference Providers">
      <Container maxWidth="xl" sx={{ mt: 2, height: 'calc(100% - 50px)' }}>
        <Box
          sx={{
            width: '100%',
            height: 'calc(100vh - 150px)',
            overflow: 'auto',
          }}
        >
          <ProviderEndpointsTable />
        </Box>
      </Container>
    </Page>
  )
}

export default AdminProviders 