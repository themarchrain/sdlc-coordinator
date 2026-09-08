import type { ThemeDefinition, ThemeSwatches } from '../types'
import { figmaAntdTheme } from './antd'
import { figmaSlots } from './slots'

/** figma 主题五色示意条（外观面板展示）。primary 与 ink 同为纯黑——
 *  Figma 的系统主色本就是黑（先例：nike/wired 同构）。 */
export const figmaSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f7f5',
  ink: '#000000',
  primary: '#000000',
  accent: '#ff3d8b',
}

/**
 * figma 主题 = Figma 营销站设计语言。
 * 黑白编辑框架：纯白画布 + 纯黑墨与主 CTA + 发丝线卡（无阴影）；
 * 叙事节奏靠巨型粉彩色块（lime/lilac/cream/mint/pink/coral/navy）插入白底之间；
 * CTA 全药丸形（50px），选中态 = 黑面白字（「selected = primary surface」）；
 * figmaSans 可变字重（320–700 细粒度）承担层级，无中灰正文；
 * figmaMono 专属 eyebrow/caption（大写正字距）；magenta #FF3D8B 单发强调。
 * CSS 令牌见 themes/css/figma.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = 纯白画布 #FFFFFF（规格「editor-clean black-and-white frame」）。
 */
export const figmaTheme: ThemeDefinition = {
  id: 'figma',
  label: 'Figma',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: figmaSwatches,
  antd: figmaAntdTheme,
  slots: figmaSlots,
}
