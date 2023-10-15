import { FC } from 'react'

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
    <SnackbarContextProvider>
      <LoadingContextProvider>
        <ThemeProviderWrapper>
          <AccountContextProvider>
            { children }
          </AccountContextProvider>
        </ThemeProviderWrapper>
      </LoadingContextProvider>
    </SnackbarContextProvider>
  )
}

export default AllContextProvider