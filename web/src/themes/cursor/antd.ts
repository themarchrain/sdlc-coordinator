import type { ThemeConfig } from 'antd'

/**
 * cursor 主题的 antd ThemeConfig —— 自 design/cursor main.tsx 的
 * cursorToken 与 cursorComponents 原样搬移（13 组 components 覆写全量誊抄）。
 *
 * Cursor 设计语言：Cursor Orange #f54e00 是唯一品牌电压，只给主 CTA（Button）；
 * 其余控件的活动电压（info/processing/link/选中态）全部收敛为 ink #26251e，
 * 不引入第二动作色。海拔只靠发丝线：antd 两级阴影 + Button 三态阴影全关。
 * algorithm 未指定 = antd 默认算法（cursor 源即如此）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/cursor.css 的同名令牌手工保持同源：
 *   colorPrimary          = --primary        (#f54e00)
 *   colorPrimaryActive    = --primary-active (#d04200)
 *   colorSuccess          = 语义绿           (#1f8a65，slots.timeline 语义色同源)
 *   colorError            = 语义红           (#cf2d56)
 *   colorWarning          = 暖金             (#c08532 = timeline-done，偏离点)
 *   colorInfo/colorLink   = --ink            (#26251e，活动电压 = 墨色)
 *   colorText 四阶         = ink/body/muted/muted-soft
 *   colorBorder 双阶       = --hairline-strong/--hairline
 *   colorFill 四阶         = 暖墨 alpha（替代默认黑 alpha，保持奶油协调）
 *   colorBgLayout         = --canvas         (#f7f7f4)
 *   borderRadius 8/6/12   = input/tile→card 圆角刻度
 *   fontFamily            = --font-sans（Inter 近似 CursorGothic）
 *   fontFamilyCode        = --font-mono（JetBrains Mono）
 */

// Cursor 令牌层：以下字面 hex 必须与 themes/css/cursor.css 的令牌块、
// 源 styles/index.css 的 :root 变量三层同源一致。
const cursorToken = {
  // 品牌电压：Cursor Orange 仅主 CTA；active 压深
  colorPrimary: '#f54e00',
  colorPrimaryActive: '#d04200',
  // 语义色（semantic-success / semantic-error；warning 取暖金偏离点）
  colorSuccess: '#1f8a65',
  colorWarning: '#c08532',
  colorError: '#cf2d56',
  // info / processing / link 收敛为 ink：活动电压 = 墨色，不引入第二动作色
  colorInfo: '#26251e',
  // colorInfo 为墨色时，antd 会派生实色灰橄榄 Alert 底再压 ink 字（对比度仅约 2.6:1）；
  // 按设计语言钉住为「canvas-soft 内嵌面 + 发丝线」：hairline-only depth，无彩色 info 填充。
  colorInfoBg: '#fafaf7',
  colorInfoBorder: '#e6e5e0',
  colorLink: '#26251e',
  colorLinkHover: '#26251e',
  // 文字阶梯：ink / body / muted / muted-soft
  colorText: '#26251e',
  colorTextSecondary: '#5a5852',
  colorTextTertiary: '#807d72',
  colorTextQuaternary: '#a09c92',
  // 边框双阶 + 暖色填充（替代默认黑 alpha，保持奶油协调）
  colorBorder: '#cfcdc4',
  colorBorderSecondary: '#e6e5e0',
  colorFill: 'rgba(38, 37, 30, 0.14)',
  colorFillSecondary: 'rgba(38, 37, 30, 0.10)',
  colorFillTertiary: 'rgba(38, 37, 30, 0.06)',
  colorFillQuaternary: 'rgba(38, 37, 30, 0.04)',
  // 表面：奶油画布 + 纯白卡片
  colorBgLayout: '#f7f7f4',
  colorBgContainer: '#ffffff',
  colorBgElevated: '#ffffff',
  // 圆角：按钮/输入 8px，卡片 12px，小件 6px
  borderRadius: 8,
  borderRadiusSM: 6,
  borderRadiusLG: 12,
  // 海拔只靠发丝线：关闭 antd 两级阴影
  boxShadow: 'none',
  boxShadowSecondary: 'none',
  // 字体：Inter 近似 CursorGothic；代码面 JetBrains Mono
  fontFamily: "Inter, system-ui, 'Helvetica Neue', Helvetica, Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif",
  fontFamilyCode: "'JetBrains Mono', 'Fira Code', Consolas, monospace",
}

// 橙色只留给 Button（主 CTA）；其余控件的激活电压收敛为 ink
const cursorComponents = {
  Button: { fontWeight: 500, primaryShadow: 'none', defaultShadow: 'none', dangerShadow: 'none' },
  Input: { colorPrimary: '#26251e', activeShadow: 'none' },
  InputNumber: { colorPrimary: '#26251e', activeShadow: 'none' },
  Select: { colorPrimary: '#26251e', optionSelectedBg: '#e6e5e0' },
  Switch: { colorPrimary: '#26251e', colorPrimaryHover: '#26251e' },
  Checkbox: { colorPrimary: '#26251e', colorPrimaryHover: '#26251e' },
  Radio: { colorPrimary: '#26251e', colorPrimaryHover: '#26251e' },
  Steps: { colorPrimary: '#26251e' },
  Pagination: { colorPrimary: '#26251e', colorPrimaryHover: '#26251e' },
  Menu: { colorPrimary: '#26251e' },
  Segmented: { colorPrimary: '#26251e', trackBg: '#efeee8', itemSelectedBg: '#ffffff' },
  Table: { headerBg: '#fafaf7', headerColor: '#807d72', rowHoverBg: '#fafaf7', borderColor: '#e6e5e0' },
  Tooltip: { colorBgSpotlight: '#26251e' },
}

export const cursorAntdTheme: ThemeConfig = {
  token: cursorToken,
  components: cursorComponents,
}
