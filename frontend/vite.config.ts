import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The browser talks to this dev server. /graphql, including the
// websocket, is proxied to the Go API so the page has one origin.
export default defineConfig({
  plugins: [react()],
  build: {
    // The lazily loaded 3D view chunk is mostly three.js (~570 kB, ~140 kB
    // gzipped). It only loads for 3D models, so it is allowed to be large.
    chunkSizeWarningLimit: 700,
  },
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
      // Tab close posts here with navigator.sendBeacon.
      '/session': {
        target: 'http://127.0.0.1:43131',
        changeOrigin: true,
      },
      // Info images live next to the model, under the API's models directory.
      '/model-asset': {
        target: 'http://127.0.0.1:43131',
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
