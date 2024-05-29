import React, { FC } from 'react'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Card from '@mui/material/Card'
import CardActions from '@mui/material/CardActions'
import CardActionArea from '@mui/material/CardActionArea'
import CardContent from '@mui/material/CardContent'
import CardMedia from '@mui/material/CardMedia'
import Avatar from '@mui/material/Avatar'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  IApp,
} from '../../types'

import {
  getAppImage,
  getAppAvatar,
  getAppName,
  getAppDescription,
} from '../../utils/apps'

const AppStoreCard: FC<{
  app: IApp,
  onClick?: () => void,
}> = ({
  app,
  onClick,
}) => {
  const avatar = getAppAvatar(app)
  const image = getAppImage(app)
  const name = getAppName(app)
  const description = getAppDescription(app)

  return (
    <Card>
      <CardActionArea
        onClick={ onClick }
      >
        {
          image && (
            <CardMedia
              sx={{ height: 140 }}
              image={ image }
              title={ name }
            />
          )
        }
        <CardContent
          sx={{
            cursor: 'pointer',
          }}
        >
          <Row
            sx={{
              alignItems: 'flex-start',
            }}
          >
            {
              avatar && (
                <Cell
                  sx={{
                    mr: 2,
                    pt: 1,
                  }}
                >
                  <Avatar
                    src={ avatar }
                  />
                </Cell>
              )
            }
            <Cell grow sx={{
              minHeight: '80px'
            }}>
              <Typography gutterBottom variant="h5" component="div">
                { name }
              </Typography>
              <Typography variant="body2" color="text.secondary">
                { description }
              </Typography>
            </Cell>
          </Row>
        </CardContent>
      </CardActionArea>
      {
        onClick && (
          <CardActions>
            <Button
              size="small"
              onClick={ onClick }
            >
              Launch
            </Button>
          </CardActions>
        )
      }
    </Card>
  )
}

export default AppStoreCard