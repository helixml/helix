import React, { FC } from 'react'
import { DialogProps } from '@mui/material/Dialog'

import Window from './Window'
import JsonView from './JsonView'

interface JsonWindowProps {
  data: any,
  size?: DialogProps["maxWidth"],
  onClose: {
    (): void,
  },
}

const JsonWindow: FC<React.PropsWithChildren<JsonWindowProps>> = ({
  data,
  size = 'md',
  onClose,
}) => {

  return (
    <Window
      open
      withCancel
      size={ size }
      cancelTitle="Close"
      onCancel={ onClose }
    >
      <JsonView
        data={ data }
        scrolling={ false }
      />
    </Window>
  )
}

export default JsonWindow