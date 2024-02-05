import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

export const InteractionContainer: FC<{
  name: string,
  buttons?: React.ReactNode,
}> = ({
  name,
  buttons,
  children,
}) => {
  return (
    <Box sx={{
      mb: 3,
    }}>
      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem', pb: 1, }}>
        <Avatar className="interactionAvatar" sx={{ width: 24, height: 24 }}>{name.charAt(0).toUpperCase()}</Avatar>
        <Box sx={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
          <Row>
            <Cell flexGrow={1}>
              <Typography
                className="interactionName"
                variant="subtitle2"
                sx={{
                  // fontWeight: 'bold',
                  color: '#aaa'
                }}
              >
                { name.charAt(0).toUpperCase() + name.slice(1) }
              </Typography>
            </Cell>
            <Cell>
              {
                buttons
              }
            </Cell>
          </Row>
        </Box> 
      </Box>
      {
        children
      }
    </Box>
  )   
}

export default InteractionContainer