import { theme } from 'antd'
import { describe, expect, it } from 'vitest'

import { THEME_LIST } from './registry'

/** 相对亮度（WCAG 2.1），输入 #rgb/#rrggbb。 */
function relativeLuminance(hex: string): number {
  const raw = hex.replace('#', '')
  const full = raw.length === 3 ? raw.split('').map((ch) => ch + ch).join('') : raw
  const channels = [0, 2, 4].map((offset) => parseInt(full.slice(offset, offset + 2), 16) / 255)
  const [r, g, b] = channels.map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(a)
  const lb = relativeLuminance(b)
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la]
  return (hi + 0.05) / (lo + 0.05)
}

/** 把 #rrggbb / #rgb / rgba(r,g,b,a) 颜色按不透明底色合成，返回 #rrggbb。 */
function compositeOver(fg: string, bgHex: string): string {
  if (!fg.startsWith('rgba(')) {
    const raw = fg.replace('#', '')
    return `#${(raw.length === 3 ? raw.split('').map((ch) => ch + ch).join('') : raw).toLowerCase()}`
  }
  const [r, g, b, a] = fg.replace(/^rgba\(|\)$/g, '').split(',').map((part) => Number(part.trim()))
  const raw = bgHex.replace('#', '')
  const full = raw.length === 3 ? raw.split('').map((ch) => ch + ch).join('') : raw
  const blended = [r, g, b].map((fgChannel, index) => {
    const bgChannel = parseInt(full.slice(index * 2, index * 2 + 2), 16)
    return Math.round(bgChannel * (1 - a) + fgChannel * a)
  })
  return `#${blended.map((v) => v.toString(16).padStart(2, '0')).join('')}`
}

describe('select dropdown selected-option contrast guard', () => {
  it('keeps every theme selected-option background readable against its option text', () => {
    const failures: string[] = []
    for (const themeDef of THEME_LIST) {
      const override = themeDef.antd.components?.Select?.optionSelectedBg
      const derived = theme.getDesignToken({ token: themeDef.antd.token })
      const elevated = themeDef.antd.token.colorBgElevated ?? derived.colorBgElevated
      const selectedBg = compositeOver(String(override ?? derived.colorPrimaryBg), elevated)
      const optionText = compositeOver(String(derived.colorText), elevated)
      const ratio = contrastRatio(selectedBg, optionText)
      if (ratio < 3) {
        failures.push(`${themeDef.id}: selected bg ${selectedBg} vs text ${optionText} contrast ${ratio.toFixed(2)} (< 3)`)
      }
    }
    expect(failures).toEqual([])
  })
})
