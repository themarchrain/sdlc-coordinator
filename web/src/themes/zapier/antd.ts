import type { ThemeConfig } from 'antd'

/**
 * zapier 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Zapier 设计语言 = 暖奶油 + 咖啡墨 + 唯一橙：画布 #FFFEFB / 软奶油 #F8F4F0
 * （「温度即品牌」，禁纯白画布）、深咖啡墨 #201515（禁纯黑）、唯一饱和橙
 * #FF4F00 承担一切主 CTA 与链接（「Don't introduce a second chromatic accent」）。
 * 控件边按规格是 1px 墨线（text-input/pricing-card/button-tertiary 字面），
 * 卡片海拔 = 软奶油填充对比（Level 2 无边框）而非阴影。
 * 算法未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/zapier.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#FF4F00，规格 colors.primary 字面)
 *   colorPrimaryHover/Active = 橙压深两阶        (#E64700 / #CC3F00，派生)
 *   colorInfo/colorLink     = Zapier 橙          (#FF4F00，唯一彩色 = 唯一交互/链接色)
 *   colorInfoBg/colorInfoBorder = 橙 7% 薄纱 / 橙 32% 线（rgba 派生）
 *   colorSuccess            = 暖绿               (#2F7A3C，派生：规格无语义色板)
 *   colorWarning            = 琥珀               (#9A6A10，派生：与主橙拉开明度)
 *   colorError              = 暖红               (#B3261E，派生：橙族加深转红)
 *   colorText 四阶          = Ink/Body/Body Mid/Mute
 *                             (#201515 / #605D52 / #939084 / #C5C0B1，规格 文字四级原文)
 *   colorBgLayout/Container = --canvas           (#FFFEFB，text-input 底即画布)
 *   colorBgElevated         = #FFFFFF（派生：浮层比画布微亮一档）
 *   colorBorder             = Ink                (#201515，规格「1px solid ink border」字面)
 *   colorBorderSecondary/colorSplit = 暖线       (#E8E2D9，派生)
 *   borderRadius 12         = rounded.md（「buttons + cards」品牌标志中位圆角）
 *   borderRadiusLG 12       = 同上（品牌只有 0/6/12/pill 四档）
 *   fontWeightStrong 600    = Inter semibold（button-md 18px/600）
 *   fontFamily              = Inter 栈（规格 替代方案：Degular → Inter 500）
 */
export const zapierAntdTheme: ThemeConfig = {
  token: {
    // Zapier 橙：一切主 CTA 的转化签名
    colorPrimary: '#FF4F00',
    colorPrimaryHover: '#E64700',
    colorPrimaryActive: '#CC3F00',
    // 唯一彩色原则：info/link 一律橙（Zapier 官网链接即橙）
    colorInfo: '#FF4F00',
    colorLink: '#FF4F00',
    colorLinkHover: '#E64700',
    // info Alert = 橙薄纱软面（rgba 派生；唯一彩色内自洽）
    colorInfoBg: 'rgba(255, 79, 0, .07)',
    colorInfoBorder: 'rgba(255, 79, 0, .32)',
    // 语义三色（派生：规格「不单列语义色板，向墨 + 橙阶层借」→ 暖调克制化）
    colorSuccess: '#2F7A3C',
    colorWarning: '#9A6A10',
    colorError: '#B3261E',
    // 文字阶梯：Ink / Body / Body Mid / Mute（四级原文，咖啡暖墨）
    colorText: '#201515',
    colorTextSecondary: '#605D52',
    colorTextTertiary: '#939084',
    colorTextQuaternary: '#C5C0B1',
    // 画布直达容器：text-input 底 = canvas（规格 text-input 字面）；浮层微亮一档（派生）
    colorBgLayout: '#FFFEFB',
    colorBgContainer: '#FFFEFB',
    colorBgElevated: '#FFFFFF',
    // 控件边 = 1px 墨线（规格 text-input / button-tertiary / pricing-card 字面）；
    // 次级分隔走暖线（派生），行线用 mute（ex-data-table-cell rowBorder 字面）
    colorBorder: '#201515',
    colorBorderSecondary: '#E8E2D9',
    colorSplit: '#E8E2D9',
    // 圆角：12px 是「按钮 + 卡」的品牌标志半径（rounded.md）；小件 6px（rounded.sm）
    borderRadius: 12,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 字重：Inter 400/500/600/700 四档在场
    fontWeightStrong: 600,
    // 海拔靠填充对比，阴影只给浮层（ex-modal-card「elevated shadow」的克制化）
    boxShadow: '0 2px 8px rgba(32, 21, 21, .08)',
    boxShadowSecondary: '0 6px 16px rgba(32, 21, 21, .12)',
    // Degular Display（定制族）→ Inter 替代（规格「Font Substitutes」：Inter 500 最近似）
    fontFamily:
      "Inter, system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Button: {
      // 按钮标签 18px/600（规格 typography.button-md）；零按钮阴影
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 当前项 = 软奶油填充高亮（card-content 的 Level 2 语汇）
      itemSelectedBg: '#F8F4F0',
      itemSelectedColor: '#201515',
    },
    Table: {
      // 表头 = canvas-soft（ex-data-table-cell headerBackground 字面）+ Body 色
      headerBg: '#F8F4F0',
      headerColor: '#605D52',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F8F4F0',
      borderColor: '#C5C0B1', // ex-data-table-cell rowBorder = mute 字面
    },
    Tooltip: {
      // 反转提示面 = 深咖啡墨
      colorBgSpotlight: '#201515',
    },
  },
}
