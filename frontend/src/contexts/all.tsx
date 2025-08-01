import { ReactNode } from 'react'

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

// Import the new AppsContextProvider
import {
  AppsContextProvider,
} from './apps'

// Import the StreamingProvider
import {
  StreamingContextProvider,
} from './streaming'

// Import the FloatingRunnerStateProvider
import {
  FloatingRunnerStateProvider,
} from './floatingRunnerState'

const AllContextProvider = ({ children }: { children: ReactNode }) => {
  return (
    <RouterContextProvider>
      <SnackbarContextProvider>
        <LoadingContextProvider>
          <ThemeProviderWrapper>
            <AccountContextProvider>
              <SessionsContextProvider>
                <AppsContextProvider>
                  <StreamingContextProvider>
                    <FloatingRunnerStateProvider>
                      {children}
                    </FloatingRunnerStateProvider>
                  </StreamingContextProvider>
                </AppsContextProvider>
              </SessionsContextProvider>
            </AccountContextProvider>
          </ThemeProviderWrapper>
        </LoadingContextProvider>
      </SnackbarContextProvider>
    </RouterContextProvider>
  )
}

export default AllContextProvider