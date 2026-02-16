import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    // Wails dev server integration
    strictPort: true,
    hmr: {
      // Wails embeds the frontend; HMR needs to connect back to Vite's server
      host: 'localhost',
    },
  },
});
