import type { ThemeConfig } from 'antd'

/**
 * stripe 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Stripe 设计语言（金融基础设施品牌）：
 * - 亮画布：canvas #ffffff / soft #f6f9fc；卡片白面 + hairline #e3e8ee + 冷调输入线 #a8c3de；
 * - 品牌靛蓝一族：primary #533afd（唯一填充 CTA）/ hover 提亮 #665efd（primary-soft）/
 *   按压 #2e2b8c（primary-press）/ 深档 #4434d4（primary-deep，链接 hover）；
 * - 深海军蓝 ink #0d253d 是正文墨色；brand-dark-900 #1c1e54 承担深色浮层；
 * - 药丸按钮：全部 Button 999px，padding 紧凑 8px 16px，三态阴影全关
 *   （深度交给渐变 mesh 与蓝调投影，不落在按钮上）；
 * - 次按钮 = 白底靛蓝字 + 靛蓝描边（规格 button-secondary 原文）；
 * - 排版签名 = sohne 细字重：fontWeightStrong 压 500，display 层 300 + 负字距
 *   由 stripe.css 专属片段承担；数字位 tnum 见同处；
 * - 投影走蓝调 shadow-blue 家族：Level 1 `0 1px 3px rgba(0,55,112,.08)` /
 *   Level 2 `0 8px 24px … + 0 2px 6px …`。
 * 语义三色为推导值（规格明言营销系统无语义色盘，状态色属 dashboard 产品 UI）：
 *   success #0e7d4f（与 ink 同明度带的克制绿）/ warning #9b6829（规格 lemon）/
 *   error #ea2261（规格 ruby）。
 * algorithm 未指定 = antd 默认算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/stripe.css 的同名令牌手工保持同源：
 *   colorPrimary        = --primary         (#533afd)
 *   colorPrimaryHover   = --primary-soft    (#665efd 提亮档)
 *   colorPrimaryActive  = --primary-active  (#2e2b8c primary-press)
 *   colorInfo           = --info            (#533afd 靛蓝即 info/品牌)
 *   colorSuccess/Warning/Error = 语义推导三色（见上）
 *   colorLink           = --link            (#533afd link-on-light = primary)
 *   colorText 四阶       = ink #0d253d / ink-secondary #273951 / ink-mute #64748d / #61718a
 *   colorBgLayout/Container/Elevated = --canvas (#ffffff 白画布)
 *   colorBorder         = --login-input-border (#a8c3de hairline-input，控件边)
 *   colorBorderSecondary/colorSplit = --hairline (#e3e8ee)
 *   boxShadow 双键      = --shadow-panel 家族（蓝调 Level 1 / Level 2）
 *   fontFamily          = --font-sans（sohne-var 栈，ss01 开关在 stripe.css 片段）
 */
export const stripeAntdTheme: ThemeConfig = {
  token: {
    // 品牌靛蓝一族：base #533afd → hover 提亮 #665efd → 按压 #2e2b8c
    colorPrimary: '#533afd',
    colorPrimaryHover: '#665efd',
    colorPrimaryActive: '#2e2b8c',
    // 语义三色（推导：规格营销系统无语义色盘 → 与调色板同饱和带的克制语义）
    colorSuccess: '#0e7d4f',
    colorWarning: '#9b6829',
    colorError: '#ea2261',
    // info = 靛蓝（品牌即信息色）；info Alert = 靛蓝 7% 薄纱 + 22% 淡边
    colorInfo: '#533afd',
    colorInfoBg: 'rgba(83, 58, 253, 0.07)',
    colorInfoBorder: 'rgba(83, 58, 253, 0.22)',
    // 链接 = 靛蓝，hover 落 primary-deep（规格「press-state warmer alternative」档）
    colorLink: '#533afd',
    colorLinkHover: '#4434d4',
    colorLinkActive: '#2e2b8c',
    // 文字阶梯：ink / ink-secondary / ink-mute / ink-mute-2（规格四级原文）
    colorText: '#0d253d',
    colorTextHeading: '#0d253d',
    colorTextSecondary: '#273951',
    colorTextTertiary: '#64748d',
    colorTextQuaternary: '#61718a',
    // 靛蓝底白字（规格 on-primary）
    colorTextLightSolid: '#ffffff',
    // 白画布：页面、容器、浮层全白
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 边线双轨：hairline-input #a8c3de（控件边）/ hairline #e3e8ee（卡与分隔）
    colorBorder: '#a8c3de',
    colorBorderSecondary: '#e3e8ee',
    colorSplit: '#e3e8ee',
    controlOutline: 'rgba(83, 58, 253, 0.18)',
    // 圆角刻度：默认 8（rounded.md 紧凑卡）/ SM 6（rounded.sm 输入）/ LG 12（rounded.lg 卡）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 蓝调投影：Level 1 卡片微抬 / Level 2 浮层（规格 Elevation 原文）
    boxShadow: '0 1px 3px rgba(0, 55, 112, 0.08)',
    boxShadowSecondary: '0 8px 24px rgba(0, 55, 112, 0.08), 0 2px 6px rgba(0, 55, 112, 0.04)',
    // sohne-var 栈：本地无 sohne 时回落 SF Pro Display / system-ui（ss01 见 stripe.css）
    fontFamily:
      "'sohne-var', 'SF Pro Display', system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: 'ui-monospace, Menlo, Monaco, Consolas, monospace',
    // 强调位压到 500：display 层的 300 细字重由 stripe.css 片段单独承担
    fontWeightStrong: 500,
  },
  components: {
    Button: {
      // 药丸是品牌交互形状：全部按钮 999px（规格 rounded.pill）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 按钮字重 400（规格 button-md 16px/400）；深度交给环境，按钮三态阴影全关
      fontWeight: 400,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 次按钮 = 规格 button-secondary：白底 + 靛蓝字 + 靛蓝描边
      defaultBg: '#ffffff',
      defaultColor: '#533afd',
      defaultBorderColor: '#533afd',
      defaultHoverBg: '#ffffff',
      defaultHoverColor: '#4434d4',
      defaultHoverBorderColor: '#4434d4',
    },
    Menu: {
      // 下拉当前项 = 靛蓝 8% 薄纱 + primary-deep 墨（pill-tag-soft 的淡染思路）
      itemSelectedBg: 'rgba(83, 58, 253, 0.08)',
      itemSelectedColor: '#4434d4',
    },
    Table: {
      // 扁平表头：soft 面白卡 + ink-mute 标签（规格 caption「table labels」角色）
      headerBg: '#f6f9fc',
      headerColor: '#64748d',
      headerSplitColor: 'transparent',
      rowHoverBg: '#f6f9fc',
      rowSelectedBg: 'rgba(83, 58, 253, 0.06)',
      rowSelectedHoverBg: 'rgba(83, 58, 253, 0.1)',
      fixedHeaderSortActiveBg: '#f6f9fc',
      borderColor: '#e3e8ee',
    },
    Modal: { contentBg: '#ffffff', headerBg: '#ffffff', footerBg: '#ffffff' },
    Drawer: { colorBgElevated: '#ffffff' },
    // 深海军蓝 tooltip（brand-dark-900：dashboard 深色 chrome 的浮层版）
    Tooltip: { colorBgSpotlight: '#1c1e54' },
    Popover: { colorBgElevated: '#ffffff' },
    Select: { colorBgContainer: '#ffffff', optionSelectedBg: 'rgba(83, 58, 253, 0.08)' },
    Input: {
      // 输入白底；聚焦边换 primary（规格 text-input-focused 原文）
      colorBgContainer: '#ffffff',
      activeBorderColor: '#533afd',
      hoverBorderColor: '#a8c3de',
    },
    InputNumber: { colorBgContainer: '#ffffff' },
    Form: { labelColor: '#0d253d' },
    Segmented: { trackBg: '#f6f9fc', itemSelectedBg: '#ffffff' },
    Tag: { borderRadiusSM: 4 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
