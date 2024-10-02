import React from 'react'
import { RouterProvider } from 'react-router5'
import AllContextProvider from './contexts/all'
import { StreamingContextProvider } from './contexts/streaming'
import Layout from './pages/Layout'
import router, { RenderPage } from './router'

export default function App() {
  return (
    <RouterProvider router={router}>
      <AllContextProvider>
        <StreamingContextProvider>
          <Layout>
            <RenderPage />
          </Layout>
        </StreamingContextProvider>
      </AllContextProvider>
    </RouterProvider>
  )
}