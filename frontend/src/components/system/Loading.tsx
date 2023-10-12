import React, { FC } from 'react'
import createStyles from '@mui/styles/createStyles';
import makeStyles from '@mui/styles/makeStyles';

import CircularProgress, {
  CircularProgressProps,
} from '@mui/material/CircularProgress'

import Typography from '@mui/material/Typography'

const useStyles = makeStyles(theme => createStyles({
  root: {
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    height: '100%',
  },
  container: {
    maxWidth: '100%'
  },
  item: {
    textAlign: 'center',
    display: 'inline-block',
  },
}))


interface LoadingProps {
  color?: CircularProgressProps["color"],
  message?: string,
}

const Loading: FC<React.PropsWithChildren<LoadingProps>> = ({
  color = 'primary',
  message = 'loading',
  children,
}) => {
  const classes = useStyles()
  return (
    <div className={classes.root}>
      <div className={classes.container}>
        <div className={classes.item}>
          <CircularProgress 
            color={ color }
          />
          { 
            message && (
              <Typography
                variant='subtitle1'
                color={ color }
              >
                { message }
              </Typography>
            )
          }
          {
            children
          }
        </div>
        
      </div>
    </div>
  )
}

export default Loading