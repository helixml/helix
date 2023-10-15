import { FC } from 'react'

import {
  RouterContextProvider,
} from './router'

import {
  SnackbarContextProvider,
} from './snackbar'

import {
  LoadingContextProvider,
} from './loading'

import {
  ThemeProviderWrapper,
} from './theme'

import {
  AccountContextProvider,
} from './account'

const AllContextProvider: FC = ({ children }) => {
  return (
    <RouterContextProvider>
      <SnackbarContextProvider>
        <LoadingContextProvider>
          <ThemeProviderWrapper>
            <AccountContextProvider>
              { children }
            </AccountContextProvider>
          </ThemeProviderWrapper>
        </LoadingContextProvider>
      </SnackbarContextProvider>
    </RouterContextProvider>
  )
}

export default AllContextProvider