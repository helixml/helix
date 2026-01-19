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

// Import the FloatingModalProvider
import {
  FloatingModalProvider,
} from './floatingModal'

// Import the VideoStreamProvider (tracks when video streaming is active)
import {
  VideoStreamProvider,
} from './VideoStreamContext'

const AllContextProvider = ({ children }: { children: ReactNode }) => {
  return (
    <RouterContextProvider>
      <SnackbarContextProvider>
        <LoadingContextProvider>
          <ThemeProviderWrapper>
            <AccountContextProvider>
                <AppsContextProvider>
                  <StreamingContextProvider>
                    <FloatingRunnerStateProvider>
                      <FloatingModalProvider>
                        <VideoStreamProvider>
                          {children}
                        </VideoStreamProvider>
                      </FloatingModalProvider>
                    </FloatingRunnerStateProvider>
                  </StreamingContextProvider>
                </AppsContextProvider>
            </AccountContextProvider>
          </ThemeProviderWrapper>
        </LoadingContextProvider>
      </SnackbarContextProvider>
    </RouterContextProvider>
  )
}

export default AllContextProvider