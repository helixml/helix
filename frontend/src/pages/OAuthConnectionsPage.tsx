import React from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Typography from '@mui/material/Typography'
import Page from '../components/system/Page'
import OAuthConnections from '../components/account/OAuthConnections'

const OAuthConnectionsPage: React.FC = () => {
  return (
    <Page
      breadcrumbTitle="Connected Services"
      topbarContent={null}
    >
      <Container
        maxWidth="md"
        sx={{
          mt: 10,
          height: '100%',
        }}
      >
        <Box sx={{ mb: 4 }}>
          <Typography variant="h4" gutterBottom sx={{ mb: 2, fontWeight: 600 }}>
            Connected Services
          </Typography>
          <Typography variant="body1" color="textSecondary">
            Connect your account to external services to enable integrations with third-party applications and platforms.
          </Typography>
        </Box>
        
        <OAuthConnections />
      </Container>
    </Page>
  )
}

export default OAuthConnectionsPage