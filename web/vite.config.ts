import vue from '@vitejs/plugin-vue'
import { resolve } from 'node:path'
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'

// 后端地址：默认 8080，可用环境变量覆盖（例如后端换了端口：VITE_PROXY_TARGET=http://127.0.0.1:8081 pnpm dev）
const backend = process.env.VITE_PROXY_TARGET || 'http://127.0.0.1:8080'

// 构建产物直接输出到后端的静态目录，build 完即可用后端端口访问三个端。
// 可用环境变量覆盖：OUT_DIR=../server/web pnpm build
const outDir = process.env.OUT_DIR || resolve(process.cwd(), '../server/web')

// https://vite.dev/config/
export default defineConfig({
    // 三端各自独立入口，共用 /assets 下的静态资源
    base: '/',
    plugins: [vue()],
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url)),
        },
    },
    build: {
        // 直接产出到 server/web，并清空其中旧的静态文件（保留 .gitkeep）
        outDir,
        emptyOutDir: true,
        assetsDir: 'assets',
        // 多页构建：根路径(用户端) + admin + chat + kefu
        rollupOptions: {
            input: {
                index: resolve(process.cwd(), 'index.html'),
                admin: resolve(process.cwd(), 'admin.html'),
                chat: resolve(process.cwd(), 'chat.html'),
                kefu: resolve(process.cwd(), 'kefu.html'),
            },
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

