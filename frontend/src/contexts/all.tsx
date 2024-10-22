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

// all of these contexts MUST be below the account context
// because they rely on it
import {
  SessionsContextProvider,
} from './sessions'

// Import the new StreamingProvider
import {
  StreamingContextProvider,
} from './streaming'

const AllContextProvider: FC = ({ children }) => {
  return (
    <RouterContextProvider>
      <SnackbarContextProvider>
        <LoadingContextProvider>
          <ThemeProviderWrapper>
            <AccountContextProvider>
              <SessionsContextProvider>
                <StreamingContextProvider>
                  {children}
                </StreamingContextProvider>
              </SessionsContextProvider>
            </AccountContextProvider>
          </ThemeProviderWrapper>
        </LoadingContextProvider>
      </SnackbarContextProvider>
    </RouterContextProvider>
  )
}

export default AllContextProvider