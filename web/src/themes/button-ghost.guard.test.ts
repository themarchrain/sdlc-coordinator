import { describe, expect, it } from 'vitest'

import { THEME_LIST } from './registry'

const cssFiles = import.meta.glob<string>('./css/*.css', { query: '?raw', import: 'default', eager: true })

/**
 * antd 幽灵按钮（type="primary" ghost）的设计是透明底 + 主色描边/文字。
 * 主题若给 .ant-btn-primary 一律涂底色而不排除 .ant-btn-background-ghost，
 * 幽灵按钮就会变成主色底 + 主色字（近黑主色主题下=全黑不可读）。
 * 本守卫：凡 CSS 给 .ant-btn-primary 声明 background，选择器必须带
 * :not(.ant-btn-background-ghost)。
 */
function ruleBodiesFor(css: string, selectorFragment: string): Array<{ selector: string; body: string }> {
  const rules: Array<{ selector: string; body: string }> = []
  const rule = /([^{}]+)\{([^{}]*)\}/g
  let match: RegExpExecArray | null
  while ((match = rule.exec(css)) !== null) {
    if (match[1].includes(selectorFragment)) rules.push({ selector: match[1], body: match[2] })
  }
  return rules
}

describe('primary ghost button guard', () => {
  it('never paints .ant-btn-primary background without excluding the ghost variant', () => {
    const failures: string[] = []
    for (const themeDef of THEME_LIST) {
      const css = cssFiles[`./css/${themeDef.id}.css`]
      if (!css) continue
      for (const { selector, body } of ruleBodiesFor(css, '.ant-btn-primary')) {
        if (!/(?:^|[^-])background\s*:/.test(body)) continue
        if (selector.includes(':not(.ant-btn-background-ghost)')) continue
        failures.push(`${themeDef.id}: ${selector.trim().slice(-80)} paints .ant-btn-primary background without excluding ghost`)
      }
    }
    expect(failures).toEqual([])
  })
})
