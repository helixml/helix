import React, { FC } from 'react'

import SimpleConfirmWindow from './SimpleConfirmWindow'

const SimpleDeleteConfirmWindow: FC<React.PropsWithChildren<{
  title?: string,
  onCancel: {
    (): void,
  },
  onSubmit: {
    (): void,
  }
}>> = ({
  title = 'this item',
  onCancel,
  onSubmit,
}) => {
  return (
    <SimpleConfirmWindow
      title={`Delete ${title}`}
      message={`Are you sure you want to delete ${title}?`}
      confirmTitle="Confirm"
      onCancel={ onCancel }
      onSubmit={ onSubmit }
    />
  )
}

export default SimpleDeleteConfirmWindow