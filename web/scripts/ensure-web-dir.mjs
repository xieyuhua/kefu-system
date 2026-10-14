/**
 * 确保后端静态目录 server/web 存在（构建产物直接输出到这里）。
 * 首次安装依赖或克隆仓库后自动执行，避免 vite 因输出目录不存在而报错。
 */
import { existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const webDir = process.env.OUT_DIR || resolve(here, '../../server/web')

if (!existsSync(webDir)) {
  mkdirSync(webDir, { recursive: true })
  // 放一个占位文件，保证空目录也能进版本库
  writeFileSync(
    resolve(webDir, '.gitkeep'),
    '# 前端构建产物目录（pnpm build 会直接输出到这里）\n# 由 web/vite.config.ts 的 build.outDir 指定\n',
    'utf8',
  )
  console.log(`[ensure-web-dir] 已创建 ${webDir}`)
} else {
  console.log(`[ensure-web-dir] 目录已存在 ${webDir}`)
}
