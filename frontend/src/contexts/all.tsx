import { FC } from 'react'

import {
  SnackbarContextProvider,
} from './snackbar'

import {
  LoadingContextProvider,
} from './loading'

import {
  RouterContextProvider,
} from './router'

const AllContextProvider: FC = ({ children }) => {
  return (
    <SnackbarContextProvider>
      <LoadingContextProvider>
        <RouterContextProvider>
          { children }
        </RouterContextProvider>
      </LoadingContextProvider>
    </SnackbarContextProvider>
  )
}

export default AllContextProvider