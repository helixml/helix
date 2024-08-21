import React, { FC } from 'react'

import TextView from './TextView'

interface JsonViewProps {
  data: any,
  scrolling?: boolean
}

const JsonView: FC<React.PropsWithChildren<JsonViewProps>> = ({
  data,
  scrolling = false
}) => {
  return (
    <TextView
      data={JSON.stringify(data, null, 2)}
      scrolling={ scrolling }
    />
  )
}

export default JsonView