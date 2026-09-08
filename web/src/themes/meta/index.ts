import type { ThemeDefinition, ThemeSwatches } from '../types'
import { metaAntdTheme } from './antd'
import { metaSlots } from './slots'

/** meta 主题五色示意条（外观面板展示）。 */
export const metaSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f1f4f7',
  ink: '#0a1317',
  primary: '#0064e0',
  accent: '#a121ce',
}

/**
 * meta 主题 = Meta 硬件商务设计语言。
 * stark white 画布 + 摄影优先留白；双轨 CTA——黑药丸属营销页，管理台取产品内轨
 * 钴蓝 #0064e0（buy-cta「Add to cart / Configure」）+ 描边次 CTA（2px 墨边）；
 * 100px 全药丸按钮、16-32px 大圆角卡、平坦海拔（Level 2 影只属粘性购买面板）、
 * Deep Ink 六级文字阶、Optimistic VF 展示字（ss01+ss02 特征集 + 500/300/400
 * 三档字重节奏 + -0.14~-0.16px 负字距）；深带（ink-deep promo strip）只作
 * json 深卡与登录展示板，Oculus 紫 #a121ce 仅作品类强调点缀。
 * CSS 令牌见 themes/css/meta.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = Canvas White（规格「stark white canvas」）。
 */
export const metaTheme: ThemeDefinition = {
  id: 'meta',
  label: 'Meta',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: metaSwatches,
  antd: metaAntdTheme,
  slots: metaSlots,
}
