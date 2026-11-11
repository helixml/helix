/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/setupTests.ts'],
    // The CI runner shares CPUs with the Go test steps; under that load the
    // slower jsdom component tests exceed the 5s default and fail at random.
    ...(process.env.CI
      ? { testTimeout: 30_000, hookTimeout: 30_000, minWorkers: 1, maxWorkers: '50%' }
      : {}),
    deps: {
      inline: [/react-markdown/, /react-syntax-highlighter/],
    },
    alias: {
      'react/jsx-runtime': path.resolve(__dirname, 'node_modules/react/jsx-runtime.js'),
    }
  }
}) 