import type { ThemeConfig } from 'antd'

/**
 * mongodb 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * MongoDB 设计语言（白文档画布 + 亮 MongoDB 绿）：
 * - 主 CTA = 亮 MongoDB 绿 #00ED64 药丸，绿底之上是深青墨 #001E2B
 *   （规格 on-primary；colorTextLightSolid 同源，绿底白字是错误形态）；
 * - hover/按压走绿色族：hover #00B545（primary-deep）、按压 #008C34（primary-pressed）；
 * - 链接 = brand-green-dark #00684A（规格 button-link 文字色），而非亮绿
 *   （规格 Don'ts「亮绿不做正文色」）；
 * - 文字阶梯：ink #001E2B / slate #3D4F5B / steel #5C6C7A / stone #7C8C9A（规格四级）；
 * - 白画布三面全白，卡靠 hairline #E1E5E8 一等边线分层（Level 0 = flat + border）；
 * - 药丸几何：全部 Button 999px（品牌签名「don't soften corners on buttons」），
 *   输入 8px（rounded.md）、卡 12px（rounded.lg）；
 * - 字体 Euclid Circular A 系统 fallback 栈；代码 Source Code Pro（规格双栈）；
 * - 字重：标题 500（fontWeightStrong）、按钮/徽标 600（Button fontWeight）；
 * - 海拔阶梯照规格 Level 2/4 原值（rgba(0,30,43,…) 深青调投影）；
 * - 语义：warning 规格 #946F3F/#FFF8E0；success 取 brand-green-dark #00684A
 *   （文字级绿，亮绿只留给 CTA）；error/info 规格未给 → 派生
 *   error #D94141、info #3D4F9F（accent-blue 类目色借用，见 slots.ts 注）。
 * algorithm 未指定 = antd 默认算法（亮色；规格有暗面变体但默认亮）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/mongodb.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary         (#00ED64 brand-green)
 *   colorPrimaryHover  = --login-primary-active (#00B545 primary-deep)
 *   colorPrimaryActive = --primary-active  (#008C34 primary-pressed)
 *   colorInfo          = --info            (#3D4F9F accent-blue)
 *   colorSuccess       = --success         (#00684A brand-green-dark)
 *   colorWarning       = --warning         (#946F3F semantic-warning-text)
 *   colorLink          = --link            (#00684A brand-green-dark)
 *   colorText 四阶      = ink/slate/steel/stone (#001E2B/#3D4F5B/#5C6C7A/#7C8C9A)
 *   colorTextLightSolid= --on-primary      (#001E2B，绿底深青墨签名)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF)
 *   colorBorder        = hairline-strong  (#C1CCD6，规格输入一等边)
 *   colorBorderSecondary/colorSplit = --hairline/--hairline-soft (#E1E5E8/#ECEFF1)
 *   borderRadius 8 / SM 6 / LG 12 = 规格 rounded.md/lg + 徽标 rounded.sm
 *   fontFamily         = --font-sans（Euclid Circular A fallback 栈）
 *   fontFamilyCode     = --font-mono（Source Code Pro 栈）
 *   boxShadow 双键     = 规格 Elevation Level 2 / Level 4 原值
 */
export const mongodbAntdTheme: ThemeConfig = {
  token: {
    // MongoDB Green 一族：base #00ED64 → hover 压深 primary-deep → 按压 primary-pressed
    colorPrimary: '#00ED64',
    colorPrimaryHover: '#00B545',
    colorPrimaryActive: '#008C34',
    // 签名：绿底之上是深青墨而非白（规格 on-primary #001E2B）
    colorTextLightSolid: '#001E2B',
    // 链接 = brand-green-dark（规格 button-link；亮绿不做文字色）
    colorLink: '#00684A',
    colorLinkHover: '#00A35C', // 派生：green-mid 提亮一档（规格只给了基色与按压色）
    colorLinkActive: '#008C34',
    // 语义：success = 文字级深绿；warning = 规格语义对；error/info = 派生（见文件头注）
    colorSuccess: '#00684A',
    colorWarning: '#946F3F',
    colorError: '#D94141',
    colorInfo: '#3D4F9F',
    colorInfoBg: '#EDF0FA', // 派生：accent-blue 薄纱（与 --info-soft 同源）
    colorInfoBorder: '#C9CFE8',
    // 文字阶梯：ink / slate / steel / stone（四级原文）
    colorText: '#001E2B',
    colorTextHeading: '#001E2B',
    colorTextSecondary: '#3D4F5B',
    colorTextTertiary: '#5C6C7A',
    colorTextQuaternary: '#7C8C9A',
    // 白画布：页面、容器、浮层全白，卡靠 hairline 边线分层（Level 0 = flat + border）
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线：输入一等边 hairline-strong / 卡边 hairline / 行缝 hairline-soft（规格三档）
    colorBorder: '#C1CCD6',
    colorBorderSecondary: '#E1E5E8',
    colorSplit: '#ECEFF1',
    // 输入聚焦 = 2px brand-green-dark 环（规格 text-input-focused）→ 控件光环取深绿
    controlOutline: 'rgba(0, 104, 74, .12)',
    // 圆角：输入 8（rounded.md）/ 徽标·chip 6（rounded.sm）/ 大卡 12（rounded.lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 标题 500（规格 heading-1..4）；600 只给按钮/徽标（Button fontWeight）
    fontWeightStrong: 500,
    // 海拔阶梯照规格 Elevation Level 2（feature cards）/ Level 4（modals, dropdowns）原值
    boxShadow: '0 4px 12px rgba(0, 30, 43, .08)',
    boxShadowSecondary: '0 16px 48px -8px rgba(0, 30, 43, .16)',
    // Euclid Circular A（授权字体，禁外部加载）→ 规格 fallback 栈 + CJK 补齐
    fontFamily:
      "'Euclid Circular A', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    // 规格代码栈：Source Code Pro → SF Mono/Menlo/Consolas
    fontFamilyCode: "'Source Code Pro', 'SF Mono', Menlo, Consolas, monospace",
  },
  components: {
    Button: {
      // 药丸是品牌交互形状：全部按钮 999px（规格 Don'ts「don't soften corners on buttons」）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 按钮字重 600（规格 button-md 14px/600）
      fontWeight: 600,
      // 绿底深青墨字（on-primary 签名）
      primaryColor: '#001E2B',
      // 次按钮 = 透明底 + hairline-strong 描边（规格 button-secondary），无投影
      defaultBg: 'transparent',
      defaultBorderColor: '#C1CCD6',
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#001E2B' }, // 选中态绿底上的对勾 = 深青墨
    Steps: { colorWhite: '#001E2B' },    // 进行中步骤 = 绿底深青墨数字
    Menu: {
      // 当前项 = surface-feature 薄荷面（featured 档的高亮语汇）+ 深青墨字
      itemSelectedBg: '#E3FCEF',
      itemSelectedColor: '#001E2B',
    },
    Table: {
      // 表头 = surface 微灰面 + steel 标签；行 hover = surface-soft（规格 comparison-table）
      headerBg: '#F9FBFA',
      headerColor: '#5C6C7A',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F4F7F6',
      borderColor: '#ECEFF1',
    },
    Tooltip: {
      // 提示 = 深青墨面（canvas-dark）+ 白字（补回 colorTextLightSolid 的深墨转向）
      colorBgSpotlight: '#001E2B',
      colorTextLightSolid: '#FFFFFF',
    },
    Tag: { borderRadiusSM: 999 }, // 状态徽标一律药丸（规格 Do「every status badge」）
    Select: { optionSelectedBg: '#E3FCEF' },
    Segmented: { trackBg: '#F4F7F6', itemSelectedBg: '#FFFFFF' },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
