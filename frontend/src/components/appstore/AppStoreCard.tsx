import React, { FC } from 'react'
import { SxProps } from '@mui/material/styles'
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

import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'

const AppStoreCard: FC<{
  avatar?: string,
  image?: string,
  name?: string,
  description?: string,
  clickTitle?: string,
  disabled?: boolean,
  selected?: boolean,
  sx?: SxProps,
  onClick?: () => void,
}> = ({
  avatar,
  image,
  name,
  description,
  clickTitle = 'Launch',
  disabled = false,
  selected = false,
  sx = {},
  onClick,
}) => {
  return (
    <Card
      sx={sx}
    >
      <CardActionArea
        disabled={ disabled }
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
          <CardActions
            sx={{
              minHeight: '50px'
            }}
          >
            {
              selected ? (
                <CheckCircleOutlineIcon
                  sx={{
                    color: 'green',
                  }}
                />
              ) : (
                <Button
                  size="small"
                  color="secondary"
                  disabled={ disabled }
                  onClick={ onClick }
                >
                  { clickTitle }
                </Button>
              )
            }
          </CardActions>
        )
      }
    </Card>
  )
}

export default AppStoreCard