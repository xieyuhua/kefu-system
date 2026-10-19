/**
 * 消息提示音播放。
 *
 * - `on` / 空 / 其它：播放内置「叮咚」提示音（Web Audio 合成，无需音频文件）
 * - `off`：静音
 * - http(s):// 或 / 开头的 URL：播放自定义音频
 *
 * 移动端注意：iOS Safari / Android Chrome 要求音频必须在用户手势（点击/触摸）中启动，
 * 否则 AudioContext 停留在 suspended，消息到达时播放会被静默。
 * 因此提供 `initAudioUnlock()`：在页面首次触摸/点击时解锁一次，之后提示音即可正常出声。
 */

let audioCtx: AudioContext | null = null

function getCtx(): AudioContext | null {
  if (typeof window === 'undefined') return null
  const AC =
    window.AudioContext ||
    (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
  if (!AC) return null
  if (!audioCtx) audioCtx = new AC()
  return audioCtx
}

/** 在用户手势中解锁音频（移动端必须） */
export function unlockAudio(): void {
  const ac = getCtx()
  if (!ac) return
  if (ac.state === 'suspended') void ac.resume().catch(() => undefined)
}

/** 注册首次手势解锁：touch / click / pointerdown 任一触发即解锁一次 */
export function initAudioUnlock(): void {
  if (typeof window === 'undefined') return
  const unlock = () => {
    unlockAudio()
    const ac = getCtx()
    if (ac && ac.state === 'running') {
      window.removeEventListener('touchstart', unlock)
      window.removeEventListener('click', unlock)
      window.removeEventListener('pointerdown', unlock)
    }
  }
  window.addEventListener('touchstart', unlock, { passive: true })
  window.addEventListener('click', unlock)
  window.addEventListener('pointerdown', unlock)
}

/** 内置「叮咚」提示音：两声短促高频正弦波 */
async function playBuiltinDing(): Promise<void> {
  const ac = getCtx()
  if (!ac) return
  if (ac.state === 'suspended') {
    await ac.resume().catch(() => undefined)
  }
  if (ac.state !== 'running') return

  const now = ac.currentTime
  // 两声：A5(880Hz) 短音 + D6(1175Hz) 短音，间隔约 0.16s
  const notes = [
    { freq: 880, at: 0 },
    { freq: 1175, at: 0.16 },
  ]
  for (const n of notes) {
    const osc = ac.createOscillator()
    const gain = ac.createGain()
    osc.type = 'sine'
    osc.frequency.value = n.freq
    const t = now + n.at
    gain.gain.setValueAtTime(0.0001, t)
    gain.gain.exponentialRampToValueAtTime(0.28, t + 0.012)
    gain.gain.exponentialRampToValueAtTime(0.0001, t + 0.14)
    osc.connect(gain)
    gain.connect(ac.destination)
    osc.start(t)
    osc.stop(t + 0.15)
  }
}

/** 播放自定义音频 URL */
function playUrl(url: string): void {
  const audio = new Audio(url)
  audio.volume = 0.6
  void audio.play().catch(() => undefined)
}

/**
 * 按配置播放提示音。
 * @param config 提示音配置：'on'（内置）/ 'off'（静音）/ 音频 URL
 */
export function playMessageSound(config?: string): void {
  const raw = (config || '').trim()
  const c = raw.toLowerCase()
  if (c === 'off' || c === 'false' || c === '0' || c === 'none') return
  if (/^(https?:)?\/\//i.test(raw) || raw.startsWith('/')) {
    playUrl(raw)
    return
  }
  void playBuiltinDing()
}
