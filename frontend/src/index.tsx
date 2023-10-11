import React from "react"
import ReactDOM from "react-dom"
import App from "./App"
import CssBaseline from "@mui/material/CssBaseline"

import UserService from "./services/UserService"

let render = () => {
  ReactDOM.render(
    <>
      <CssBaseline />
      <App />
    </>,
    document.getElementById("root")
  )
}

UserService.initKeycloak(render);