import type { ThemeDefinition, ThemeSwatches } from '../types'
import { mongodbAntdTheme } from './antd'
import { mongodbSlots } from './slots'

/**
 * mongodb 主题五色示意条（外观面板展示）。
 * accent 取类目标签紫 #7B3FF2（University 课程卡的类别编码色族代表，
 * 规格限定其只作类目标签 —— 示意条仅展示调色板，不进入 UI 消费）。
 */
export const mongodbSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f9fbfa',
  ink: '#001e2b',
  primary: '#00ed64',
  accent: '#7b3ff2',
}

/**
 * mongodb 主题 = MongoDB 设计语言。
 * 双面身份、默认亮面：白文档画布 + 亮 MongoDB 绿 #00ED64 药丸 CTA（绿底深青墨字），
 * deep teal #001E2B 暗英雄带只落在 json 代码面与登录 showcase（规格有暗面变体、默认亮）；
 * 按钮全药丸、卡 12px、Euclid Circular A、标题 500 / 按钮徽标 600；
 * 链接与成功态用 brand-green-dark #00684A（亮绿不做文字色）。
 * 品牌时刻：brand-mark = 深青圆面内的三根亮绿条（暗英雄带上绿药丸的缩影）。
 * CSS 令牌见 themes/css/mongodb.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #ffffff（默认亮面：白文档画布）。
 */
export const mongodbTheme: ThemeDefinition = {
  id: 'mongodb',
  label: 'MongoDB',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: mongodbSwatches,
  antd: mongodbAntdTheme,
  slots: mongodbSlots,
}
