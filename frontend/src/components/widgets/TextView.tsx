import React, { FC } from 'react'
import makeStyles from '@mui/styles/makeStyles'
import Box from '@mui/material/Box'
import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

interface StyleProps {
  scrolling: boolean
}

const useStyles = makeStyles(theme => ({
  root: ({scrolling}: StyleProps) => ({
    padding: 10,
    color: '#FFFFFF',
    fontFamily: 'Courier New',
    minWidth: 'min-content',
    width: '50vw',
    height: '100%',
    overflowY: scrolling ? 'auto' : 'visible',
    overflowX: 'auto',
  }),
}))

interface TextViewProps {
  data: string,
  scrolling?: boolean
}

const TextView: FC<React.PropsWithChildren<TextViewProps>> = ({
  data,
  scrolling = false
}) => {

  const classes = useStyles({scrolling})
  const theme = useTheme()
  const themeConfig = useThemeConfig()

  return (
    <Box
      className={classes.root}
      sx={{
        backgroundColor: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkPanel,
        '&::-webkit-scrollbar': {
          width: '4px',
          borderRadius: '8px',
          my: 2,
        },
        '&::-webkit-scrollbar-track': {
          background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
        },
        '&::-webkit-scrollbar-thumb': {
          background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
          borderRadius: '8px',
        },
        '&::-webkit-scrollbar-thumb:hover': {
          background: theme.palette.mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
        },
      }}
    >
      <Box
        component="pre"
        sx={{
          p: 2,
        }}
      >
        <Box
          component="code"
          sx={{
            wordBreak: 'break-all',
            wordWrap: 'break-all',
            whiteSpace: 'pre-wrap',
            fontSize: '0.9rem',
          }}
        >
          { data }
        </Box>
      </Box>
    </Box>
  )
}

export default TextView