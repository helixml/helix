import React from 'react'
import { RouterProvider } from 'react-router5'
import AllContextProvider from './contexts/all'
import Layout from './pages/Layout'
import router, { RenderPage } from './router'
import {
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import useAnalyticsInit from './hooks/useAnalyticsInit'

// Create a client
const queryClient = new QueryClient()

function AppInner() {
  useAnalyticsInit()
  return (
    <RouterProvider router={router}>
      <AllContextProvider>
        <Layout>
          <RenderPage />
        </Layout>
      </AllContextProvider>
    </RouterProvider>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AppInner />
    </QueryClientProvider>
  )
}