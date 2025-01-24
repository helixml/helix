import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 8081,
  },
  publicDir: 'assets',
  build: {
    rollupOptions: {
      external: ['#minpath', '#minproc', '#minurl'],
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom'],
        },
      },
    },
  },
  resolve: {
    alias: {
      '#minpath': 'path',
      '#minproc': 'process',
      '#minurl': 'url'
    },
  },
})