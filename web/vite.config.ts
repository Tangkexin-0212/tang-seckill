import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

// 网关地址。gateway 既没有 CORS 也不托管静态资源，
// 所以前端一律通过同源反代访问后端：浏览器看到的是 5173/4173 上的 /api/*，
// 由 Vite 转发到 8080，天然规避跨域，也不需要改动后端一行代码。
const BACKEND = process.env.SK_BACKEND ?? 'http://127.0.0.1:8080'

const proxy = {
  '/api': { target: BACKEND, changeOrigin: true },
  '/health': { target: BACKEND, changeOrigin: true },
}

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  // dev 与 preview 都要挂代理：preview 用于本地"生产构建产物"验收
  server: { host: '127.0.0.1', port: 5173, proxy },
  preview: { host: '127.0.0.1', port: 4173, proxy },
})
