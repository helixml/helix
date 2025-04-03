import React from 'react'
import { RouterProvider } from 'react-router5'
import AllContextProvider from './contexts/all'
import Layout from './pages/Layout'
import router, { RenderPage } from './router'
import {
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'

// Create a client
const queryClient = new QueryClient()

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router}>
      <AllContextProvider>
        <Layout>
          <RenderPage />
          </Layout>
        </AllContextProvider>
      </RouterProvider>
    </QueryClientProvider>
  )
}