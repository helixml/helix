import React from 'react'
import ReactDOM from 'react-dom/client'
import Widget from './Widget.tsx'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <Widget
      searchBoxTheme={{
        iconColor: '#ffffff'
      }}
    />
  </React.StrictMode>,
)
