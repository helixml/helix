import React, { FC, useState } from 'react'
import { SxProps } from '@mui/system'
import JsonWindow from './JsonWindow'
import ClickLink from './ClickLink'

interface JsonWindowLinkProps {
  data: any,
  sx?: SxProps,
  className?: string,
}

const JsonWindowLink: FC<React.PropsWithChildren<JsonWindowLinkProps>> = ({
  data,
  sx = {},
  className,
  children,
}) => {

  const [ open, setOpen ] = useState(false)

  return (
    <>
      <ClickLink
        className={ className }
        sx={ sx }
        onClick={ () => setOpen(true) }
      >
        { children }
      </ClickLink>
      {
        open && (
          <JsonWindow
            data={ data }
            onClose={ () => setOpen(false) }
          />
        )
      }
    </>
  )
}

export default JsonWindowLink