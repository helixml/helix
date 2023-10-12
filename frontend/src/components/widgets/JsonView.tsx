import React, { FC } from 'react'
import makeStyles from '@mui/styles/makeStyles';

interface StyleProps {
  scrolling: boolean,
}

const useStyles = makeStyles(theme => ({
  root: ({scrolling}: StyleProps) => ({
    width: '100%',
    height: '100%',
    overflow: scrolling ? 'auto' : 'visible',
  }),
}))

interface JsonViewProps {
  data: any,
  scrolling?: boolean,
}

const JsonView: FC<React.PropsWithChildren<JsonViewProps>> = ({
  data,
  scrolling = false,
}) => {

  const classes = useStyles({scrolling})

  return (
    <div className={ classes.root }>
      <pre><code>{ JSON.stringify(data, null, 4) }</code></pre>
    </div>
  )
}

export default JsonView