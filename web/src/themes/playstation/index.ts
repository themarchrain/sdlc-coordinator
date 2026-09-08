import type { ThemeDefinition, ThemeSwatches } from '../types'
import { playstationAntdTheme } from './antd'
import { playstationSlots } from './slots'

/** playstation 主题五色示意条（外观面板展示）。accent 位展示 commerce 橙（仅示意，不进管理台交互位）。 */
export const playstationSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f5f7fa',
  ink: '#000000',
  primary: '#0070d1',
  accent: '#d53b00',
}

/**
 * playstation 主题 = PlayStation 三面营销系统。
 * 黑/白/PS Blue 章节交替的三面画布，管理台默认取白亮轨（canvas-light #ffffff）；
 * PS Blue #0070d1 承载每一颗全圆药丸 CTA（rounded.full 9999px），SST 感
 * weight-300 展示字（44/35/28px 阶梯 + 0.1px 微字距）给出 airy 编辑感，
 * 8px 次级卡系统（surface-card #f5f7fa 冷蓝染）+ 4px 输入；黑章节面
 * （#121314/#181818）只保留给 json 深卡、tooltip 与登录 showcase（暗 hero 带，
 * link-dark #53b1ff 点缀）；commerce 橙/PS Plus 金/marathon 黄按规格保留条款
 * 不进管理台铬。系统平坦无影，浮层唯一 0 4px 12px rgba(0,0,0,.16)。
 * CSS 令牌见 themes/css/playstation.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = canvas-light（规格「true white console-showcase band」白亮轨）。
 */
export const playstationTheme: ThemeDefinition = {
  id: 'playstation',
  label: 'PlayStation',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: playstationSwatches,
  antd: playstationAntdTheme,
  slots: playstationSlots,
}
