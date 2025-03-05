import React, { FC } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import Page from '../components/system/Page'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'

const OrgSettings: FC = () => {
  // Get account context and router
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
 
  if(!account.user) return null

  return (
    <Page
      breadcrumbTitle={ account.organizationTools.organization?.display_name || 'Organization People' }
      breadcrumbShowHome={ false }
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3, p: 2 }}>
          <Typography variant="h5" component="h2" gutterBottom>
            Organization People
          </Typography>
        </Box>
      </Container>
    </Page>
  )
}

export default OrgSettings
