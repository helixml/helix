import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'

import Page from '../../components/system/Page'
import HelixModelsTable from '../../components/dashboard/HelixModelsTable'

const AdminHelixModels: FC = () => {
  return (
    <Page breadcrumbTitle="Helix Models">
      <Container maxWidth="xl" sx={{ mt: 2, height: 'calc(100% - 50px)' }}>
        <Box
          sx={{
            width: '100%',
            height: 'calc(100vh - 150px)',
            overflow: 'auto',
          }}
        >
          <HelixModelsTable />
        </Box>
      </Container>
    </Page>
  )
}

export default AdminHelixModels 