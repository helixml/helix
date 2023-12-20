import React, { FC } from 'react'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import CardActions from '@mui/material/CardActions'

import UpgradeIcon from '@mui/icons-material/Upgrade'
import LocalPhoneIcon from '@mui/icons-material/LocalPhone'
import HomeWorkIcon from '@mui/icons-material/HomeWork'

import useRouter from '../../hooks/useRouter'

import {
  emitEvent,
} from '../../utils/analytics'

export const WaitingInQueue: FC<{
  hasSubscription?: boolean,
}> = ({
  hasSubscription = false,
}) => {
  const router = useRouter()
  const colSize = hasSubscription ? 6 : 4
  return (
    <Grid container spacing={ 2 }>
      <Grid item xs={ 12 }>
        <Typography variant="h5" gutterBottom>
          Very high system demand...
        </Typography>
        <Typography variant="body1" gutterBottom>
          We are working hard to get you a GPU to run your job.
        </Typography>
        <Typography variant="body1" gutterBottom>
          In the meantime, here are some options to speed things up:
        </Typography>
      </Grid>
      {
        !hasSubscription && (
          <Grid item xs={ 12 } md={ colSize }>
            <Card
              sx={{
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
              }}
            >
              <CardContent sx={{
                flexGrow: 1,
              }}>
                <UpgradeIcon fontSize="large" />
                <Typography gutterBottom variant="h5" component="div">
                    Upgrade Account
                </Typography>
                <Typography gutterBottom variant="body2" color="text.secondary">
                    Sign up for our premium plan to get priority access to GPUs.
                </Typography>
              </CardContent>
              <CardActions
                sx={{
                  flexGrow: 0,
                  justifyContent: 'flex-end',
                }}
              >
                  <Button size="small" variant="contained" onClick={() => {
                    emitEvent({
                      name: 'queue_upgrade_clicked'
                    })
                    router.navigate('account')
                  }}>Upgrade</Button>
              </CardActions>
            </Card>
          </Grid>
        )
      }
      <Grid item xs={ 12 } md={ colSize }>
        <Card
          sx={{
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <CardContent sx={{
            flexGrow: 1,
          }}>
            <HomeWorkIcon fontSize="large" />
            <Typography gutterBottom variant="h5" component="div">
                On-Prem Deployment
            </Typography>
            <Typography variant="body2" color="text.secondary">
                Deploy Helix on your own infrastructure.
            </Typography>
          </CardContent>
          <CardActions
            sx={{
              justifyContent: 'flex-end',
            }}
          >
              <Button size="small" variant="contained" onClick={() => {
                emitEvent({
                  name: 'queue_on_prem_clicked'
                })
                window.open('https://docs.helix.ml/docs/controlplane')
              }}>View Docs</Button>
          </CardActions>
        </Card>
      </Grid>
      <Grid item xs={ 12 } md={ colSize }>
        <Card
          sx={{
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <CardContent sx={{
            flexGrow: 1,
          }}>
            <LocalPhoneIcon fontSize="large" />
            <Typography gutterBottom variant="h5" component="div">
                Talk to sales
            </Typography>
            <Typography variant="body2" color="text.secondary">
                Speak to our sales team to discuss your needs.
            </Typography>
          </CardContent>
          <CardActions
            sx={{
              justifyContent: 'flex-end',
            }}
          >
              <Button size="small" variant="contained" onClick={() => {
                emitEvent({
                  name: 'queue_get_in_touch_clicked'
                })
                window.open('mailto:founders@helix.ml')
              }}>Get in touch</Button>
          </CardActions>
        </Card>
        
      </Grid>
    </Grid>
  )  
}

export default WaitingInQueue