/**
 * 聊天文本消息的「安全最小 Markdown 子集」渲染工具。
 *
 * 设计原则：
 *  1. **先转义、再替换**：任何输入先做 HTML 转义（& < > " '），
 *     之后的 Markdown 替换只会在「已经是纯文本」的字符串上包标签，
 *     因此不可能出现用户输入的标签被执行（不存在 v-html 注入面）。
 *  2. **不吞字符**：所有结构 / 行内规则都要求「成对、合法」才生效，
 *     单个 `*`、孤立的 `#`、`-` 不带空格等一律原样保留。
 *  3. **零依赖**：只用正则与字符串操作，不引入 marked / markdown-it / dompurify。
 *
 * 支持的结构（刻意保持很小）：
 *  - `**加粗**`、`*斜体*`、`` `行内代码` ``
 *  - `# / ## / ### 标题` → 渲染为「加粗段落」（不生成 h1~h6，避免撑大字号）
 *  - `- / * / • 无序列表`、`1. 有序列表`
 *  - `> 引用`
 *  - `[文本](http(s)://…)` 链接（target=_blank + rel=noopener noreferrer）
 *  - 单换行 → `<br>`，空行 → 段落分隔
 */

/** HTML 转义表：文本节点与属性值共用（属性值用到的引号也一并转义） */
const ESCAPE_MAP: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}

/**
 * 把任意输入转成安全文本：`& < > " '` 全部转义。
 * 这是整条渲染链的**唯一入口**，所有 Markdown 规则只在这之后执行。
 */
export function escapeHtml(input: string): string {
  return String(input ?? '').replace(/[&<>"']/g, (ch) => ESCAPE_MAP[ch] || ch)
}

/** 块级结构：标题（1~3 级） */
const RE_HEADING = /^[ \t]{0,3}(#{1,3})[ \t]+(.+?)[ \t]*$/
/** 块级结构：无序列表项（必须「符号 + 空格」才算，`-` 单独出现保持原样） */
const RE_UL = /^[ \t]{0,3}[-*•][ \t]+(.+?)[ \t]*$/
/** 块级结构：有序列表项（`1. ` 形式） */
const RE_OL = /^[ \t]{0,3}\d{1,9}\.[ \t]+(.+?)[ \t]*$/
/** 块级结构：引用 */
const RE_QUOTE = /^[ \t]{0,3}>[ \t]?(.*)$/

/** 行内：`行内代码`（不跨行） */
const RE_CODE = /`([^`\n]+)`/g
/** 行内：`***粗斜***`（先于加粗 / 斜体处理，避免留下孤立的 `*`） */
const RE_BOLD_ITALIC = /\*\*\*([^\s*][^\n]*?)\*\*\*/g
/** 行内：`**加粗**`（不跨行；未闭合时正则不命中 → 原样保留） */
const RE_BOLD = /\*\*([^\n]+?)\*\*/g
/** 行内：`*斜体*`（内容首尾不能是空白 / `*`，因此 `5 * 3 * 4` 不会被误判） */
const RE_ITALIC = /\*([^\s*](?:[^*\n]*[^\s*])?)\*/g
/** 行内：`[文本](url)` */
const RE_LINK = /\[([^\]\n]{0,400})\]\(\s*([^)\s]{1,2000})\s*\)/g
/** 行内代码占位符（\u0000 不会被用户输入或转义结果产生） */
const RE_CODE_SLOT = /\u0000(\d+)\u0000/g
/** 链接占位符 */
const RE_LINK_SLOT = /\u0001(\d+)\u0001/g

export interface MarkdownResult {
  /** 渲染出的安全 HTML（`rich === false` 时为空字符串） */
  html: string
  /** 是否真的用到了 Markdown 结构：false 表示应走原来的纯文本渲染路径 */
  rich: boolean
}

interface InlineResult {
  html: string
  /** 是否产生了 Markdown 标签（用于判定这段文本需不需要走富文本路径） */
  changed: boolean
}

/**
 * 行内渲染：入口先整体转义，再依次处理
 * 行内代码 → 链接 → 粗斜体 → 加粗 → 斜体。
 * 代码与链接会先抽成占位符，避免其内容（或 href 属性）被后续规则二次改写。
 */
function renderInline(raw: string): InlineResult {
  let changed = false
  let text = escapeHtml(raw)

  const codes: string[] = []
  text = text.replace(RE_CODE, (_m: string, code: string) => {
    changed = true
    codes.push(code)
    return `\u0000${codes.length - 1}\u0000`
  })

  const links: string[] = []
  text = text.replace(RE_LINK, (whole: string, label: string, url: string) => {
    const href = url.trim()
    // 安全边界：只放行 http / https，其它协议（javascript: / data: / vbscript: …）降级为纯文本
    if (!/^https?:\/\//i.test(href)) return whole
    changed = true
    // 链接整体占位，避免 href 属性被后续的加粗/斜体规则二次改写；
    // 链接文本里若含行内代码占位符，保持原样，由最后的还原步骤统一处理。
    links.push(
      `<a class="md-a" href="${href}" target="_blank" rel="noopener noreferrer">${label}</a>`,
    )
    return `\u0001${links.length - 1}\u0001`
  })

  const boldItalic = (_m: string, inner: string): string => {
    changed = true
    return `<strong><em>${inner}</em></strong>`
  }
  const bold = (_m: string, inner: string): string => {
    changed = true
    return `<strong>${inner}</strong>`
  }
  const italic = (_m: string, inner: string): string => {
    changed = true
    return `<em>${inner}</em>`
  }

  text = text.replace(RE_BOLD_ITALIC, boldItalic)
  text = text.replace(RE_BOLD, bold)
  text = text.replace(RE_ITALIC, italic)

  if (links.length) {
    text = text.replace(RE_LINK_SLOT, (_m: string, idx: string) => links[Number(idx)] ?? '')
  }
  if (codes.length) {
    text = text.replace(
      RE_CODE_SLOT,
      (_m: string, idx: string) => `<code class="md-code">${codes[Number(idx)] ?? ''}</code>`,
    )
  }

  return { html: text, changed }
}

/** 是否是块级结构的起始行（用于切分段落） */
function isBlockStart(line: string): boolean {
  return RE_HEADING.test(line) || RE_UL.test(line) || RE_OL.test(line) || RE_QUOTE.test(line)
}

/**
 * 把 Markdown 子集渲染成安全 HTML。
 * 返回 `rich === false` 时调用方应回退到纯文本渲染（保持历史消息样式零变化）。
 */
export function renderMarkdownSafe(input: string): MarkdownResult {
  const source = String(input ?? '').replace(/\r\n?/g, '\n')
  const lines = source.split('\n')
  const blocks: string[] = []
  let rich = false
  let i = 0

  while (i < lines.length) {
    const line = lines[i]

    // 空行：段落分隔（纯文本里的空行由原来的 pre-wrap 渲染保证，不需要标记 rich）
    if (!line.trim()) {
      i += 1
      continue
    }

    // 标题 → 加粗段落（不生成 h1~h6）
    const heading = RE_HEADING.exec(line)
    if (heading) {
      blocks.push(`<p class="md-p md-h">${renderInline(heading[2]).html}</p>`)
      rich = true
      i += 1
      continue
    }

    // 无序列表：把连续的列表行合并成一个 ul
    if (RE_UL.test(line)) {
      const items: string[] = []
      while (i < lines.length) {
        const m = RE_UL.exec(lines[i])
        if (!m) break
        items.push(`<li>${renderInline(m[1]).html}</li>`)
        i += 1
      }
      blocks.push(`<ul class="md-ul">${items.join('')}</ul>`)
      rich = true
      continue
    }

    // 有序列表
    if (RE_OL.test(line)) {
      const items: string[] = []
      while (i < lines.length) {
        const m = RE_OL.exec(lines[i])
        if (!m) break
        items.push(`<li>${renderInline(m[1]).html}</li>`)
        i += 1
      }
      blocks.push(`<ol class="md-ol">${items.join('')}</ol>`)
      rich = true
      continue
    }

    // 引用
    if (RE_QUOTE.test(line)) {
      const parts: string[] = []
      while (i < lines.length) {
        const m = RE_QUOTE.exec(lines[i])
        if (!m) break
        const inner = m[1] ? renderInline(m[1]).html : ''
        parts.push(inner)
        i += 1
      }
      blocks.push(`<blockquote class="md-quote">${parts.join('<br>')}</blockquote>`)
      rich = true
      continue
    }

    // 普通段落：连续非空、且不是块级结构的行，用 <br> 连接
    const parts: string[] = []
    while (i < lines.length) {
      const current = lines[i]
      if (!current.trim() || isBlockStart(current)) break
      const inline = renderInline(current)
      if (inline.changed) rich = true
      parts.push(inline.html)
      i += 1
    }
    blocks.push(`<p class="md-p">${parts.join('<br>')}</p>`)
  }

  // 没有任何 Markdown 标记：交给调用方的原纯文本路径（样式保持不变）
  if (!rich) return { html: '', rich: false }
  return { html: blocks.join(''), rich: true }
}

/** 便捷判断：这段文本是否包含会被渲染的 Markdown 标记 */
export function hasMarkdown(input: string): boolean {
  return renderMarkdownSafe(input).rich
}
