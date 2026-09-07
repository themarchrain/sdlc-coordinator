import type { ThemeConfig } from 'antd'

/**
 * default 主题的 antd ThemeConfig —— 自 develop main.tsx 原样搬移。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/default.css 的同名令牌手工保持同源：
 *   colorPrimary  = --primary      (#2f6bff / 大小写等价)
 *   colorSuccess  = --success      (#0f9f8f)
 *   colorText     = --ink          (#152033)
 *   colorTextSecondary ≈ --body-alt(#667085)
 *   colorBgLayout = --canvas-soft 家族（antd 布局底，骨架内为 --canvas）
 */
export const defaultAntdTheme: ThemeConfig = {
  token: {
    colorPrimary: '#2F6BFF',
    colorSuccess: '#0F9F8F',
    colorText: '#152033',
    colorTextSecondary: '#667085',
    colorBgLayout: '#F3F6FA',
    borderRadius: 8,
    fontFamily: 'Inter, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif',
  },
}
