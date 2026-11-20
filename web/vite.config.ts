import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const appVersion = readFileSync(new URL('../VERSION', import.meta.url), 'utf8').trim()
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(appVersion)) {
  throw new Error('VERSION must contain a semantic version')
}

export default defineConfig({
  plugins: [vue()],
  define: { __APP_VERSION__: JSON.stringify(appVersion) },
  server: {
    port: 9528,
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
