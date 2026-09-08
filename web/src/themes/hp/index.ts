import type { ThemeDefinition, ThemeSwatches } from '../types'
import { hpAntdTheme } from './antd'
import { hpSlots } from './slots'

/** hp 主题五色示意条（外观面板展示）：白纸画布 + 深墨 + 唯一蓝 + 珊瑚 sale 点缀。 */
export const hpSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f7f7f7',
  ink: '#1a1a1a',
  primary: '#024ad8',
  accent: '#ff5050',
}

/**
 * hp 主题 = HP 设计语言（亮色）。
 * 白纸目录系统：纯白画布 + Cloud/Fog 灰带节奏 + 深墨 #1a1a1a 正文；
 * HP Electric Blue #024ad8 是唯一信号（CTA/链接/chevron/激活指示，每视口至多两处）；
 * 单一字体族 Forma DJR Micro、展示字重恒 500；双层圆角哲学
 * （按钮/输入 4px sharp 对 卡/照片框 16px soft）；签名手势 =
 * 角度 ~60° 的蓝色 chevron 平行四边形（wordmark 双斜杠回声，
 * 只落在 brand-mark 与登录 signal，不做卡片内噪点装饰）；
 * 页面节奏以 Ink 暗板收尾（json 卡 = promo-strip-dark 暗板时刻）。
 * CSS 令牌见 themes/css/hp.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = --canvas（#ffffff）。
 */
export const hpTheme: ThemeDefinition = {
  id: 'hp',
  label: 'HP',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: hpSwatches,
  antd: hpAntdTheme,
  slots: hpSlots,
}
