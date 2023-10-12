import React from "react"
import ReactDOM from "react-dom"
import App from "./App"
import CssBaseline from "@mui/material/CssBaseline"

let render = () => {
  ReactDOM.render(
    <>
      <CssBaseline />
      <App />
    </>,
    document.getElementById("root")
  )
}

render()