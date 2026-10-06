import type { ThemeConfig } from 'antd'

/**
 * wired 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * WIRED 设计语言 = 严格的黑白二重奏：纯黑 #000000 就是品牌色（wordmark/CTA/footer），
 * 唯一彩色是内文链接蓝 #057DBC（「reserved for inline body links only」，此处兼任 info）。
 * 方角几何（rounded.none 0px 全站，「square corners are non-negotiable」）、
 * 零软阴影（海拔 = 发丝线 #E0E0E0 与表面对比）、Apercu sans 承担全部控件结构文字
 * （「serif for narrative, sans for structure」——衬线只在 css 展示位出现）。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/wired.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#000000，规格「Ink Black IS the accent」)
 *   colorPrimaryHover/Active = 纯黑提亮两阶     (#1A1A1A ink-soft / #333333，黑无法再压深)
 *   colorInfo/colorLink     = --info/--link     (#057DBC，规格唯一彩色的功能化复用)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline (蓝 6% 薄纱 + #E0E0E0 hairline)
 *   colorSuccess/Warning/Error = 语义三色（派生：规格无语义段 → 标准语义克制化）
 *   colorText 四阶           = ink #000000 / #333333(派生) / body #757575 / #9E9E9E(派生占位)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF，白画布)
 *   colorBorder             = #000000（规格 text-input/button-outline 的 1px ink 边，
 *                              colorBorderSecondary/#E0E0E0 分隔线双轨）
 *   borderRadius 三档 0      = 规格 rounded.none「every interactive shape」
 *   fontFamily              = --font-sans（Apercu 的系统/web-safe 栈近似，禁外部加载）
 */
export const wiredAntdTheme: ThemeConfig = {
  token: {
    // Ink Black：品牌即黑白二重奏，纯黑就是 CTA 色（「never softened」）
    colorPrimary: '#000000',
    colorPrimaryHover: '#1A1A1A',
    colorPrimaryActive: '#333333',
    // info = 链接蓝：规格唯一彩色的功能化复用（内文链接蓝兼任 info 信号）
    colorInfo: '#057DBC',
    colorInfoBg: '#EAF3FA',
    colorInfoBorder: '#E0E0E0',
    colorLink: '#057DBC',
    colorLinkHover: '#045D99',
    // 语义三色（规格无语义段：「validation cues use the ink + body grey hierarchy」
    // → 图表/时间线仍需可分辨语义，派生为标准语义克制化）
    colorSuccess: '#1E7B34',
    colorWarning: '#9A6700',
    colorError: '#B3261E',
    // 文字阶梯坍缩为规格三级：ink/ink-soft/body（#333333、#9E9E9E 为中间派生档）
    colorText: '#000000',
    colorTextSecondary: '#333333',
    colorTextTertiary: '#757575',
    colorTextQuaternary: '#9E9E9E',
    // 白画布：页面、容器、浮层全白，卡面靠 hairline 分隔（规格 story-card 无边、行间 hairline）
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双轨：控件边 = 1px ink（规格 text-input / button-outline 字面值）；分隔线 = hairline
    colorBorder: '#000000',
    colorBorderSecondary: '#E0E0E0',
    colorSplit: '#E0E0E0',
    // 圆角：全站 0（「square corners are non-negotiable」）
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 0,
    // Apercu 700 = 结构强调档（规格 body-sm-strong / byline 700）
    fontWeightStrong: 700,
    // 零软阴影（「no drop-shadows, hairlines carry hierarchy」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Apercu（定制族禁外部加载）→ 规格「Font Substitutes」的 Helvetica/Inter 系栈近似
    fontFamily:
      '"Apercu", "Helvetica Neue", Helvetica, Arial, "Inter", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(0, 0, 0, 0.08)' },
    Button: {
      // 方角黑 CTA；标签 700 + .3px tracking（规格 button-md 16/700/0.3px）
      fontWeight: 700,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Input: {
      // 输入 hover = body 灰、聚焦 = 链接蓝（唯一彩色的 affordance 化）
      hoverBorderColor: '#757575',
      activeBorderColor: '#057DBC',
    },
    Menu: {
      // 下拉当前项 = canvas-soft 微高亮 + 纯黑（规格 search-hover 面）
      itemSelectedBg: '#F5F5F5',
      itemSelectedColor: '#000000',
    },
    Table: {
      // 表头 = canvas-soft 底 + 纯黑 mono-caps（ex-data-table headerBackground/caption）
      headerBg: '#F5F5F5',
      headerColor: '#000000',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F5F5F5',
      borderColor: '#E0E0E0',
    },
    Tooltip: {
      // 纯黑反转面（规格 footer band 的最小表达）
      colorBgSpotlight: '#000000',
    },
  },
}
