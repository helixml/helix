import React, { FC, useState } from 'react'

import JsonWindow from './JsonWindow'
import ClickLink from './ClickLink'

interface JsonWindowLinkProps {
  data: any,
  className?: string,
}

const JsonWindowLink: FC<React.PropsWithChildren<JsonWindowLinkProps>> = ({
  data,
  className,
  children,
}) => {

  const [ open, setOpen ] = useState(false)

  return (
    <>
      <ClickLink
        className={ className }
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