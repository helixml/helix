import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'widget',
    lib: {
      entry: path.resolve(__dirname, 'src/components/widgets/Embed.tsx'),
      name: 'HelixEmbed',
      fileName: (format) => `helix-embed.${format}.js`
    },
    target: 'es2015'
  },
})