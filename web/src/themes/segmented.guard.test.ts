import { describe, expect, it } from 'vitest'

import { THEME_LIST } from './registry'

const cssFiles = import.meta.glob<string>('./css/*.css', { query: '?raw', import: 'default', eager: true })

/**
 * antd Segmented 的浮动 thumb 与静止选中项共用 itemSelectedBg/itemSelectedColor
 * 组件 token（浮动 thumb 仅在滑动动画期挂载，静止状态由选中项自身背景承担）。
 * 主题 CSS 若只覆写其中一面（如给 thumb 上底色、或强制选中项文字色），
 * 另一面就会回落 antd 默认值，出现白底白字/黑底黑字。
 * 本守卫：凡 CSS 触碰 thumb 背景或选中项文字色，组件 token 必须成对定义。
 */
function ruleBodiesFor(css: string, selectorFragment: string): string[] {
  const bodies: string[] = []
  const rule = /([^{}]+)\{([^{}]*)\}/g
  let match: RegExpExecArray | null
  while ((match = rule.exec(css)) !== null) {
    if (match[1].includes(selectorFragment)) bodies.push(match[2])
  }
  return bodies
}

function declaresProperty(bodies: string[], property: string): boolean {
  return bodies.some((body) => new RegExp(`(?:^|;)\\s*${property}\\s*:`, 'm').test(body))
}

describe('segmented thumb/selected token pairing guard', () => {
  it('requires Segmented component tokens whenever theme CSS paints the thumb or selected item', () => {
    const failures: string[] = []
    for (const themeDef of THEME_LIST) {
      const css = cssFiles[`./css/${themeDef.id}.css`]
      if (!css) continue
      const paintsThumbBg = declaresProperty(ruleBodiesFor(css, 'ant-segmented-thumb'), 'background')
      const paintsSelectedColor = declaresProperty(ruleBodiesFor(css, 'ant-segmented-item-selected'), 'color')
      if (!paintsThumbBg && !paintsSelectedColor) continue
      const segmented = themeDef.antd.components?.Segmented
      if (paintsThumbBg && !segmented?.itemSelectedBg) {
        failures.push(`${themeDef.id}: CSS paints .ant-segmented-thumb background but components.Segmented.itemSelectedBg is missing`)
      }
      if (paintsSelectedColor && !segmented?.itemSelectedColor) {
        failures.push(`${themeDef.id}: CSS paints .ant-segmented-item-selected color but components.Segmented.itemSelectedColor is missing`)
      }
    }
    expect(failures).toEqual([])
  })
})
