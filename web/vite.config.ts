import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: process.env.ZENTROLA_API_TARGET || 'http://127.0.0.1:9527',
        changeOrigin: false,
      },
      '/v1': {
        target: process.env.ZENTROLA_API_TARGET || 'http://127.0.0.1:9527',
        changeOrigin: false,
      },
      '/anthropic': {
        target: process.env.ZENTROLA_API_TARGET || 'http://127.0.0.1:9527',
        changeOrigin: false,
      },
    },
  },
  preview: { port: 4173, strictPort: true },
  build: { sourcemap: false },
})
