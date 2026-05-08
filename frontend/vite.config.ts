import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 8081,
    allowedHosts: true,  // Allow access from any hostname
  },
  publicDir: 'assets',
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          mui: ['@mui/material', '@mui/icons-material', '@mui/styles'],
          'mui-charts': ['@mui/x-charts'],
          monaco: ['monaco-editor'],
          mermaid: ['mermaid'],
          pdf: ['@react-pdf/renderer', '@uiw/react-md-editor'],
          datagrid: ['@inovua/reactdatagrid-community'],
          sentry: ['@sentry/browser', '@sentry/react'],
        },
      },
    },
  },
  resolve: {
    alias: {
      '#minpath': path.resolve(__dirname, 'src/polyfills/path.js'),
      '#minproc': path.resolve(__dirname, 'src/polyfills/process.js'),
      '#minurl': path.resolve(__dirname, 'src/polyfills/url.js')
    },
  },
})