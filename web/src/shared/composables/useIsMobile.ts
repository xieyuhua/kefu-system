import { onBeforeUnmount, onMounted, ref } from 'vue'

/** 移动端断点：与企微客服工作台的移动布局切换阈值一致 */
export const MOBILE_BREAKPOINT = 768

/** 平板断点：低于此宽度隐藏右侧客户资料固定栏（改为抽屉） */
export const TABLET_BREAKPOINT = 1200

/**
 * 响应式断点：返回是否为移动端窄屏视图。
 * 用 matchMedia 监听，窗口变化时自动切换 PC / 移动布局。
 */
export function useIsMobile(breakpoint: number = MOBILE_BREAKPOINT) {
  const isMobile = ref(false)
  let mql: MediaQueryList | null = null

  function update(e?: MediaQueryListEvent | MediaQueryList): void {
    if (e && 'matches' in e) {
      isMobile.value = e.matches
    }
  }

  function detach(): void {
    if (mql && typeof mql.removeEventListener === 'function') {
      mql.removeEventListener('change', update)
    } else if (mql && typeof mql.removeListener === 'function') {
      mql.removeListener(update)
    }
    mql = null
  }

  onMounted(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
    mql = window.matchMedia(`(max-width: ${breakpoint}px)`)
    isMobile.value = mql.matches
    if (typeof mql.addEventListener === 'function') {
      mql.addEventListener('change', update)
    } else if (typeof mql.addListener === 'function') {
      mql.addListener(update)
    }
  })

  onBeforeUnmount(detach)

  return { isMobile }
}

/**
 * 屏幕分档：返回当前处于哪一档。
 *   desktop (>=1201) 四栏 · tablet (769-1200) 三栏 · mobile (<=768) 单栏
 * JS 与 CSS 媒体查询共用同一组断点常量，避免布局层级与样式脱节。
 */
export type ScreenTier = 'desktop' | 'tablet' | 'mobile'

export function useScreenTier(
  mobileBp: number = MOBILE_BREAKPOINT,
  tabletBp: number = TABLET_BREAKPOINT,
) {
  const tier = ref<ScreenTier>('desktop')

  function resolve(width: number): ScreenTier {
    if (width <= mobileBp) return 'mobile'
    if (width <= tabletBp) return 'tablet'
    return 'desktop'
  }

  function onChange(e?: MediaQueryListEvent | MediaQueryList): void {
    if (e && 'matches' in e) {
      tier.value = e.matches ? 'mobile' : resolve(window.innerWidth)
    }
  }

  onMounted(() => {
    if (typeof window === 'undefined') return
    tier.value = resolve(window.innerWidth)
    if (typeof window.matchMedia !== 'function') return
    const mql = window.matchMedia(`(max-width: ${mobileBp}px)`)
    mql.addEventListener?.('change', onChange)
    window.addEventListener('resize', () => {
      tier.value = resolve(window.innerWidth)
    })
  })

  return {
    tier,
    /** 是否应显示右侧客户资料固定栏（仅 desktop 档） */
    showSidePanel: () => tier.value === 'desktop',
  }
}
