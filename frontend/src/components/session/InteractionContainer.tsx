import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  COLORS,
}  from '../../config'

export const InteractionContainer: FC<{
  name: string,
  badge?: string,
  background?: boolean,
  buttons?: React.ReactNode,
}> = ({
  name,
  badge,
  background = false,
  buttons,
  children,
}) => {
  return (
    <Box
      sx={{
        px: 2,
        py: 1,
        borderRadius: 4,
        backgroundColor: background ? 'rgba(255, 255, 255, 0.05)' : 'transparent',
      }}
    >
      <Row>
        {
          badge && (
            <Cell
              sx={{
                mr: 1,
              }}
            >
              <Button
                variant="contained"
                color="primary"
                size="small"
                sx={{
                  textTransform: 'none',
                  bgcolor: COLORS['AI_BADGE'],
                  color: 'black',
                  fontWeight: 800,
                  padding: '2px 8px',
                  minWidth: 'auto',
                  height: 'auto'
                }}
              >
                { badge }
              </Button>
            </Cell>
          )
        }
        <Cell>
          <Typography variant="subtitle1" sx={{ fontWeight: 800 }}>
            { name }
          </Typography>
        </Cell>
        <Cell grow />
        <Cell>
          {buttons}
        </Cell>
      </Row>

      {children}
    </Box>
  );
};

export default InteractionContainer;