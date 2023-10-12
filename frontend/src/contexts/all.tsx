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

import {
  AccountContextProvider,
} from './account'

const AllContextProvider: FC = ({ children }) => {
  return (
    <SnackbarContextProvider>
      <LoadingContextProvider>
        <RouterContextProvider>
          <AccountContextProvider>
            { children }
          </AccountContextProvider>
        </RouterContextProvider>
      </LoadingContextProvider>
    </SnackbarContextProvider>
  )
}

export default AllContextProvider