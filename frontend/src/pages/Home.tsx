import React, { FC } from 'react'
import { useTheme } from '@mui/material/styles'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Alert from '@mui/material/Alert';

import HomeFeatureGrid from '../components/home/FeatureGrid'
import Page from '../components/system/Page'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useIsBigScreen from '../hooks/useIsBigScreen'

const Home: FC = () => {
  const theme = useTheme()
  const isLight = theme.palette.mode === 'light'
  const isBigScreen = useIsBigScreen()

  return (
    <Page
      showTopbar={ isBigScreen ? false : true }
    >
      <Container
        maxWidth="xl"
        sx={{
          // mt: 12,
          pt: 3,
          height: 'calc(100% - 100px)',
        }}
      >
        <Box>
          <Grid container spacing={ 2 }>
            <Grid item xs={ 12 } sm={ 12 } md={ 12 } lg={ 6 }>
              <Row
                sx={{
                  height: '100%',
                  display: 'flex',
                  flexDirection: 'row',
                  alignItems: 'center',
                }}
              >
                <Cell>
                  <Box
                    component="img"
                    src="/img/logo.png"
                    sx={{
                      width: 100,
                    }}
                  />
                </Cell>
                <Cell
                  sx={{
                    ml: 4,
                  }}
                >
                  <Typography variant={ isBigScreen ? 'h3' : 'h5' } gutterBottom>
                    Helix GenAI Stack
                  </Typography>
                  <Typography variant="body1">
                    Use AI, customize it with your own data, or integrate LLMs with APIs and develop your own AI-powered applications
                  </Typography>
                </Cell>
              </Row>
            </Grid>
            <Grid item xs={ 12 } sm={ 12 } md={ 12 } lg={ 6 }>
              <Box
                sx={{
                  textAlign: 'center',
                }}
              >
                <Box
                  component="img"
                  src={ isLight ? '/img/nebula-light.png' : '/img/nebula-dark.png' }
                  sx={{
                    width: '100%',
                    maxWidth: '800px'
                  }}
                />
              </Box>
            </Grid>
          </Grid>
        </Box>
        <Box sx={{mb:4, mt: 4}}>
          <Alert variant="outlined" severity="info">
            <strong>Calling all DevOps & platform engineers!</strong>&nbsp;
            You can <a href="https://docs.helix.ml/helix/private-deployment/controlplane/" target="_blank" style={{"color": "white"}}>deploy Helix easily</a> on your own cloud, container or Kubernetes infrastructure.
            &nbsp;<a href="mailto:founders@helix.ml" target="_blank" style={{"color": "white"}}>Email us</a> or <a href="https://discord.gg/VJftd844GE" target="_blank" style={{"color": "white"}}>join Discord</a> for help.
            {/* <AlertTitle sx={{fontSize: "15pt", marginTop: "-5px", fontWeight: "bold"}}> */}
              {/* </AlertTitle> */}
          </Alert>
        </Box>
        <HomeFeatureGrid />
      </Container>
    </Page>
  )
}

export default Home