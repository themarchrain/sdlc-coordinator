import type { ThemeDefinition, ThemeSwatches } from '../types'
import { hashicorpAntdTheme } from './antd'
import { hashicorpSlots } from './slots'

/** hashicorp 主题五色示意条（外观面板展示）：纯黑地 + charcoal + 白 CTA + Terraform 紫。 */
export const hashicorpSwatches: ThemeSwatches = {
  canvas: '#000000',
  surface: '#15181e',
  ink: '#ffffff',
  primary: '#ffffff',
  accent: '#7b42bc',
}

/**
 * hashicorp 主题 = HashiCorp 设计语言（暗色）。
 * 企业基建黑底：画布 #000000（文档实证纯黑）+ charcoal 提升阶梯
 * #15181e/#1f232b/#3b3d45 + 1px 半透明灰发丝线（borders are felt more
 * than seen）+ 白底黑字主 CTA（inverse-canvas）+ 每产品一色的身份系统
 * （Terraform 紫 #7b42bc / Vault 黄 / Consul 红 / Waypoint 青…只作身份点缀，
 * chrome 单色纪律）。海拔 = surface lift，暗底无投影；display 700 紧行高、
 * body 500 松行高；eyebrow 大写 0.6px 字距是每节必备。
 * CSS 令牌见 themes/css/hashicorp.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#000000 纯黑；首屏防白闪映射同值）。
 */
export const hashicorpTheme: ThemeDefinition = {
  id: 'hashicorp',
  label: 'HashiCorp',
  family: 'dark',
  isDark: true,
  chromeColor: '#000000',
  swatches: hashicorpSwatches,
  antd: hashicorpAntdTheme,
  slots: hashicorpSlots,
}
