import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
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
          vendor: ['react', 'react-dom'],
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