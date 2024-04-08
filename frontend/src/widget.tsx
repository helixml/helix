import React from 'react'
import ReactDOM from 'react-dom'
import Widget, { WidgetProps } from '@helixml/chat-widget'

const render = (config: WidgetProps = {
  url: '',
  model: '',
}) => {
  document.write('<div id="helix-widget-root"></div>')
  ReactDOM.render(
    <>
      <Widget {...config} />
    </>,
    document.getElementById("helix-widget-root")
  )
}

export default render