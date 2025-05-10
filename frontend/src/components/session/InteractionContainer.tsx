import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import {
  COLORS,
}  from '../../config'
import { useTheme } from '@mui/material/styles'
export const InteractionContainer: FC<{
  name: string,
  badge?: string,
  background?: boolean,
  buttons?: React.ReactNode,
  children?: React.ReactNode,
  align?: 'left' | 'right',
  border?: boolean,
}> = ({
  name,
  badge,
  background = false,
  buttons,
  children,
  align = 'left',
  border = false,
}) => {
  const theme = useTheme()

  return (
    <Box
      sx={{
        px: 2,
        py: 0.5,
        borderRadius: 4,
        backgroundColor: background ? theme.palette.background.default : 'transparent',
        border: border ? '1px solid #33373a' : 'none',
        maxWidth: '700px',
        ml: align === 'left' ? 0 : 'auto',
        mr: align === 'right' ? 0 : 'auto',
        boxShadow: border ? '0 1px 2px rgba(0,0,0,0.03)' : 'none',
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
          {align !== 'right' && (
            <Typography variant="subtitle1" sx={{ fontWeight: 800 }}>
              { name }
            </Typography>
          )}
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