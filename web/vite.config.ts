import { defineConfig } from 'vitest/config'
import type { Plugin } from 'vite'
import react from '@vitejs/plugin-react'

function adminBaseRedirect(): Plugin {
  return {
    name: 'admin-base-redirect',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = (req as { url?: string }).url ?? ''
        // Vite base is /admin/; bare /admin (often with ?repository_id=...) must redirect
        // or the dev server suggests /admin/admin and the SPA never loads.
        if (url === '/admin' || url.startsWith('/admin?')) {
          const tail = url === '/admin' ? '' : url.slice('/admin'.length)
          res.statusCode = 301
          res.setHeader('Location', `/admin/${tail}`)
          res.end()
          return
        }
        next()
      })
    },
  }
}

export default defineConfig({
  base: '/admin/',
  plugins: [react(), adminBaseRedirect()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    css: true,
    // jsdom+antd 每线程各自引导,按核数并发会打满开发机;串行 + 线程封顶 2。
    fileParallelism: false,
    poolOptions: { threads: { minThreads: 1, maxThreads: 2 } },
  },
})
