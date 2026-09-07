import type { ThemeDefinition, ThemeSwatches } from '../types'
import { supabaseAntdTheme } from './antd'
import { supabaseSlots } from './slots'

/** supabase 主题五色示意条（外观面板展示）。 */
export const supabaseSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#fafafa',
  ink: '#171717',
  primary: '#3ecf8e',
  accent: '#1c1c1c', // 夜面（json 代码块 / 登录深卡）作 accent
}

/**
 * supabase 主题 = Supabase 设计语言（design/supabase）。
 * CSS 令牌见 themes/css/supabase.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 supabase 源 index.html 的 meta theme-color（白）。
 */
export const supabaseTheme: ThemeDefinition = {
  id: 'supabase',
  label: 'Supabase',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: supabaseSwatches,
  antd: supabaseAntdTheme,
  slots: supabaseSlots,
}
