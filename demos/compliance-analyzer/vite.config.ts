import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const defaultTarget = env.VITE_HELIX_URL || 'http://localhost:8080'

  return {
    plugins: [react()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      // Dynamic CORS-busting proxy: the browser always sends requests to localhost,
      // and we forward them to whatever Helix instance the user configured.
      // The target is passed via the X-Helix-Target header (stripped before forwarding).
      proxy: {
        '/api': {
          target: defaultTarget,
          changeOrigin: true,
          configure: (proxy) => {
            proxy.on('proxyReq', (proxyReq, req) => {
              const target = req.headers['x-helix-target'] as string | undefined
              if (target) {
                const url = new URL(target)
                proxyReq.setHeader('host', url.host)
                proxyReq.removeHeader('x-helix-target')
              }
            })
          },
          router: (req) => {
            const target = req.headers['x-helix-target'] as string | undefined
            return target || defaultTarget
          },
        },
        '/v1': {
          target: defaultTarget,
          changeOrigin: true,
          configure: (proxy) => {
            proxy.on('proxyReq', (proxyReq, req) => {
              const target = req.headers['x-helix-target'] as string | undefined
              if (target) {
                const url = new URL(target)
                proxyReq.setHeader('host', url.host)
                proxyReq.removeHeader('x-helix-target')
              }
            })
          },
          router: (req) => {
            const target = req.headers['x-helix-target'] as string | undefined
            return target || defaultTarget
          },
        },
      },
    },
  }
})
