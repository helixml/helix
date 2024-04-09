import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  build: {
    emptyOutDir: false,
    lib: {
      entry: path.resolve(__dirname, 'src/widget.tsx'),
      formats: ['iife'],
      name: 'HelixEmbed',
      fileName: 'helix-embed'
    },
    target: 'es2015'
  },
})