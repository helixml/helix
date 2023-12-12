import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import CardActions from '@mui/material/CardActions'

import ShareIcon from '@mui/icons-material/Share'
import AndroidIcon from '@mui/icons-material/Android'

export const ShareSessionOptions: FC<{
  onShareSession: {
    (): void,
  },
  onPublishBot: {
    (): void,
  },
}> = ({
  onShareSession,
  onPublishBot,
}) => {
  return (
    <Box
      sx={{
        p: 2,
      }}
    >
      <Grid container spacing={ 2 }>
        <Grid item xs={ 12 } md={ 6 }>
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
              <ShareIcon fontSize="large" />
              <Typography gutterBottom variant="h5" component="div">
                Share Session
              </Typography>
              <Typography gutterBottom variant="body2" color="text.secondary">
                Share this session with other people.
              </Typography>
              <Typography gutterBottom variant="body2" color="text.secondary">
                This session will become public and users will be able to continue the conversation from this point but in their own account.
              </Typography>
            </CardContent>
            <CardActions
              sx={{
                flexGrow: 0,
                justifyContent: 'flex-end',
              }}
            >
                <Button
                  size="small"
                  variant="contained"
                  onClick={ onShareSession }
                >
                  Share Session
                </Button>
            </CardActions>
          </Card>
        </Grid>
        <Grid item xs={ 12 } md={ 6 }>
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
              <AndroidIcon fontSize="large" />
              <Typography gutterBottom variant="h5" component="div">
                Publish Bot
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Create a new bot from this training data or add this training data to an existing bot.
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Users will be able to start new sessions using this bot.
              </Typography>
            </CardContent>
            <CardActions
              sx={{
                justifyContent: 'flex-end',
              }}
            >
                <Button
                  size="small"
                  variant="contained"
                  onClick={ onPublishBot }
                >
                  Publish Bot
                </Button>
            </CardActions>
          </Card>
        </Grid>
      </Grid>
    </Box>
  )
}

export default ShareSessionOptions