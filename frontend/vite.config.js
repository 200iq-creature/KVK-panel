import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// Dev: KVK_DEV=http://127.0.0.1:2053/<basePath> npm run dev
const target = process.env.KVK_DEV || 'http://127.0.0.1:2053'

export default defineConfig({
  plugins: [vue()],
  base: './',
  server: {
    host: '0.0.0.0',
    allowedHosts: true,
    proxy: { '/api': { target, changeOrigin: true } },
  },
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 2000,
    rollupOptions: {
      input: { index: resolve(__dirname, 'index.html'), sub: resolve(__dirname, 'sub.html') },
    },
  },
})
