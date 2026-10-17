import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The browser talks to this dev server. /graphql, including the
// websocket, is proxied to the Go API so the page has one origin.
export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 43132,
    strictPort: true,
    proxy: {
      '/graphql': {
        target: 'http://127.0.0.1:43131',
        ws: true,
        changeOrigin: true,
      },
    },
  },
  preview: {
    host: '0.0.0.0',
    port: 43132,
    strictPort: true,
  },
})
