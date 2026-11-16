/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    // happy-dom builds a document several times faster than jsdom. Files that
    // depend on jsdom's fidelity (computed styles, img roles, keyboard events,
    // custom elements, HTML sanitization) opt back in with a `// @vitest-environment jsdom` docblock.
    environment: 'happy-dom',
    // happy-dom fetches scripts, stylesheets and iframe pages by itself, which
    // would bypass the network guard in setupTests. Tests never load resources.
    environmentOptions: {
      happyDOM: {
        settings: {
          disableJavaScriptFileLoading: true,
          disableCSSFileLoading: true,
          navigation: {
            disableMainFrameNavigation: true,
            disableChildFrameNavigation: true,
            disableChildPageNavigation: true,
          },
        },
      },
    },
    setupFiles: ['./src/setupTests.ts'],
    // CI shares CPUs and memory with the Go test steps. Under that load slow
    // component tests exceed the 5s default, and memory grows with the worker
    // count (each test file forks its own worker), so the count is fixed
    // rather than a share of the host's cores.
    ...(process.env.CI
      ? { testTimeout: 30_000, hookTimeout: 30_000, minWorkers: 1, maxWorkers: 4 }
      : {}),
    deps: {
      inline: [/react-markdown/, /react-syntax-highlighter/],
    },
    alias: {
      'react/jsx-runtime': path.resolve(__dirname, 'node_modules/react/jsx-runtime.js'),
    }
  }
}) 