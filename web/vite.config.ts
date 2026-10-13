import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'

// 后端地址：默认 8080，可用环境变量覆盖（例如后端换了端口：VITE_PROXY_TARGET=http://127.0.0.1:8081 pnpm dev）
const backend = process.env.VITE_PROXY_TARGET || 'http://127.0.0.1:8080'

// https://vite.dev/config/
export default defineConfig({
    // 资源用绝对路径 /assets/...，这样 /、/chat/、/admin/ 三个前缀可共用同一份产物
    base: '/',
    plugins: [vue()],
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url)),
        },
    },
    server: {
        port: 5173,
        proxy: {
            '/api': {
                target: backend,
                changeOrigin: true,
            },
            '/ws': {
                target: backend,
                ws: true,
                changeOrigin: true,
            },
            '/uploads': {
                target: backend,
                changeOrigin: true,
            },
            '/demo': {
                target: backend,
                changeOrigin: true,
            },
        },
    },
})
