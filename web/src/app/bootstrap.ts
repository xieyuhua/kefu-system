import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
import './style.css'
import App from './App.vue'
import type { Router } from 'vue-router'

/**
 * 三个端（admin / chat / kefu）各自独立打包、独立路由，
 * 但共用同一套应用引导逻辑与登录态（token 存 localStorage，跨端共享）。
 */
export function bootstrap(router: Router, mountId = '#app'): void {
  const app = createApp(App)
  app.use(createPinia())
  app.use(ElementPlus, { locale: zhCn })
  app.use(router)
  app.mount(mountId)
}
