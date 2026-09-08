import type { ThemeConfig } from 'antd'

/**
 * renault 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文/色板（字面 hex，禁 var()）。
 *
 * Renault 设计语言 = 黑白 stark 对比 + 唯一品牌强调 Sunlight Yellow：
 * - Sunlight Yellow #FFED00 只落在主 CTA / yeni 徽标 / 配置圆点上，且
 *   「黄底永远配黑字」（「Yellow always pairs with black text」）
 *   → colorTextLightSolid 压到 #000000，主按钮黄底黑字；按压 = #E6D200
 *   （primary-deep 字面值，hover 无独立值，复用）；
 * - 文字阶梯坍缩为规格四级：ink #000000 / body #222222 / mute #666666 / ash #8A8A8A；
 * - 白画布三面全白，靠 #F2F2F2 发丝线分隔；控件边 = hairline-strong #000000
 *   （「full-strength dividers + card/button outlines」的描边语言）；
 * - 语义三色为规格字面值（error #BE6464 等），其中 success/warning 饱和度
 *   不足文字对比 → antd 内取同 hue 深化（#56913D / #A8760D，推导值）；
 * - 方角几何：按钮 rounded.xs 2px，卡/面板 rounded.none 0px；
 * - 字重法则 400/700：fontWeightStrong = 700（display 纪律）；
 * - 阴影近零：仅 sticky 浮层保留规格「~10% 2-4px」微影。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/renault.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#FFED00 Sunlight Yellow)
 *   colorPrimaryHover/Active = --primary-active (#E6D200 primary-deep pressed)
 *   colorTextLightSolid     = --on-primary      (#000000 黄配黑字铁律)
 *   colorInfo               = --info            (#337AB7 规格 desaturated mid-blue)
 *   colorLink               = --link            (#000000 规格生产链接继承 ink)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline 家族（info 10% 薄纱，推导）
 *   colorSuccess/Warning/Error = 语义三色（success/warning 为文字级深化推导）
 *   colorText 四阶          = ink/body/mute/ash (#000000/#222222/#666666/#8A8A8A)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF)
 *   colorBorder             = --hairline-strong (#000000 控件描边)
 *   colorBorderSecondary    = --hairline        (#F2F2F2)
 *   borderRadius 2          = 规格 rounded.xs（按钮）
 *   borderRadiusLG 0        = 规格 rounded.none（卡/面板方角）
 *   fontFamily              = --font-sans（NouvelR 替代栈 Inter Tight/Manrope）
 */
export const renaultAntdTheme: ThemeConfig = {
  token: {
    // Sunlight Yellow：唯一品牌强调色（「single brand accent」）
    colorPrimary: '#FFED00',
    colorPrimaryHover: '#E6D200', // primary-deep（规格唯一命名状态变体 pressed；hover 无独立值，复用）
    colorPrimaryActive: '#E6D200',
    // 黄底黑字铁律：antd 派生控件（主按钮/开关/勾选）的实底文字压到墨色
    colorTextLightSolid: '#000000',
    // info = 规格 desaturated mid-blue（informational chips 用色）
    colorInfo: '#337AB7',
    // 生产链接继承 ink，靠下划线/字重而非颜色（Link）
    colorLink: '#000000',
    colorLinkHover: '#222222',
    colorLinkActive: '#222222',
    // info Alert = info 10% 薄纱软面 + 同 hue 淡描边（推导值，与 --info-soft 同源）
    colorInfoBg: '#EEF4FA',
    colorInfoBorder: '#C6D8E8',
    // 语义三色：error 为规格字面值；success/warning 同 hue 深化到文字级（推导，
    // 规格 #8DC572/#F0AD4E 在白底对比不足 3:1；填充级字面值保留在 css 令牌/时间线槽位）
    colorSuccess: '#56913D',
    colorWarning: '#A8760D',
    colorError: '#BE6464',
    // 文字阶梯：ink/body/mute/ash（Text 四级原文）
    colorText: '#000000',
    colorTextHeading: '#000000',
    colorTextSecondary: '#222222',
    colorTextTertiary: '#666666',
    colorTextQuaternary: '#8A8A8A',
    // 白画布：页面、容器、浮层全白（规格「white catalogue mode」）
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 控件描边 = hairline-strong 全强度黑（规格「card / button outlines」）
    colorBorder: '#000000',
    colorBorderSecondary: '#F2F2F2',
    colorSplit: '#F2F2F2',
    // 焦点光环 = 墨系微纱（黄环在白底不可见，推导）
    controlOutline: 'rgba(0, 0, 0, 0.12)',
    // 方角几何：rounded.xs 2px 按钮；rounded.none 0px 卡/面板
    borderRadius: 2,
    borderRadiusSM: 2,
    borderRadiusLG: 0,
    // display 纪律：标题 700（「display always 700」）
    fontWeightStrong: 700,
    // 阴影近零：仅 sticky 浮层保留规格「~10% opacity, 2–4px blur」
    boxShadow: '0 2px 4px rgba(0, 0, 0, 0.1)',
    boxShadowSecondary: '0 2px 4px rgba(0, 0, 0, 0.1)',
    // NouvelR（授权定制族，禁外部加载）→ 规格替代栈 Inter Tight / Manrope / HK Grotesk
    fontFamily:
      "'Inter Tight', Manrope, 'HK Grotesk', system-ui, -apple-system, 'Segoe UI', Arial, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Button: {
      // 按钮标签 700（规格 button-md 14.4px/700；+0.144px 字距见 css 片段）
      fontWeight: 700,
      // 三态阴影全关（catalogue 层无影）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = button-outline-dark：白底 + 1px 墨边 + 墨字（规格 Components）
      defaultBg: '#FFFFFF',
      defaultColor: '#000000',
      defaultBorderColor: '#000000',
      defaultHoverBg: '#F7F7F7', // hover = surface-soft 微沉（推导）
      defaultHoverColor: '#000000',
      defaultHoverBorderColor: '#000000',
    },
    Menu: {
      // 下拉当前项 = surface-soft 微高亮 + 墨字（规格「inactive form fields」同面）
      itemSelectedBg: '#F7F7F7',
      itemSelectedColor: '#000000',
    },
    Table: {
      // 白底表头 + charcoal 标签 + #F2F2F2 发丝分隔；行 hover = surface-soft
      headerBg: '#FFFFFF',
      headerColor: '#333333',
      headerSplitColor: '#F2F2F2',
      rowHoverBg: '#F7F7F7',
      borderColor: '#F2F2F2',
    },
    Tooltip: {
      // 黑色反转面（规格「black is structural」）
      colorBgSpotlight: '#000000',
    },
  },
}
