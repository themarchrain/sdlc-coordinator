/** @type {import('tailwindcss').Config} */
// 颜色统一引用 styles/index.css 的语义令牌（防御性同源）：
// 工具类与骨架 CSS 共享同一套主题变量，换主题时零差异。
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  corePlugins: { preflight: false },
  theme: {
    extend: {
      colors: {
        canvas: 'var(--canvas)',
        'canvas-soft': 'var(--canvas-soft)',
        card: 'var(--canvas-card)',
        sidebar: 'var(--surface-sidebar)',
        ink: 'var(--ink)',
        'ink-strong': 'var(--ink-strong)',
        body: 'var(--body)',
        muted: 'var(--mute)',
        faint: 'var(--faint)',
        action: 'var(--primary)',
        'action-active': 'var(--primary-active)',
        link: 'var(--link)',
        healthy: 'var(--success)',
        warning: 'var(--warning)',
        error: 'var(--error)',
        hairline: 'var(--hairline)',
      },
      fontFamily: {
        sans: ['var(--font-sans)'],
        mono: ['var(--font-mono)'],
        display: ['var(--font-display)'],
      },
      borderRadius: {
        xs: 'var(--radius-xs)',
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
        pill: 'var(--radius-pill)',
      },
    },
  },
  plugins: [],
}
