/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  corePlugins: { preflight: false },
  theme: {
    extend: {
      colors: {
        canvas: '#F3F6FA',
        surface: '#FFFFFF',
        ink: '#152033',
        muted: '#667085',
        action: '#2F6BFF',
        healthy: '#0F9F8F',
      },
      fontFamily: {
        sans: ['Inter', 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', 'sans-serif'],
        mono: ['JetBrains Mono', 'Cascadia Code', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
}
