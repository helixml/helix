import React, { FC, forwardRef, useState } from 'react'
import { SxProps } from '@mui/system'
import JsonWindow from './JsonWindow'
import ClickLink from './ClickLink'

interface JsonWindowLinkProps {
  data: any,
  withFancyRendering?: boolean,
  withFancyRenderingControls?: boolean,
  sx?: SxProps,
  className?: string,
}

const JsonWindowLink: FC<React.PropsWithChildren<JsonWindowLinkProps>> = forwardRef(({
  data,
  withFancyRendering = true,
  withFancyRenderingControls = true,
  sx = {},
  className,
  children,
}, ref) => {

  const [open, setOpen] = useState(false)

  const handleOpen = () => setOpen(true)
  const handleClose = () => setOpen(false)

  return (
    <>
      <ClickLink
        className={className}
        sx={sx}
        onClick={handleOpen}
      >
        {children}
      </ClickLink>
      {
        open && (
          <JsonWindow
            data={data}
            onClose={handleClose}
            withFancyRendering={withFancyRendering}
            withFancyRenderingControls={withFancyRenderingControls}
          />
        )
      }
    </>
  )
})

export default JsonWindowLink