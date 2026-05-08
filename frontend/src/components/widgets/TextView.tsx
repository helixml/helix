import React, { FC } from 'react'
import makeStyles from '@mui/styles/makeStyles'
import Box from '@mui/material/Box'
import useLightTheme from '../../hooks/useLightTheme'

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
  const lightTheme = useLightTheme()

  return (
    <Box
      className={classes.root}
      sx={{
        backgroundColor: lightTheme.panelColor,
        ...lightTheme.scrollbar,
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