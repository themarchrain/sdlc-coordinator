import type { ThemeConfig } from 'antd'

/**
 * uber 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Uber 设计语言 = 黑白二重唱 + 药丸签名：
 * - 主操作 = 纯黑药丸（规格「Ink Black #000000 是品牌唯一转化色」），
 *   hover 派生提亮一阶 #3D3D3D（规格未记黑钮 hover），按压 = black-elevated #282828
 *   （规格记载的近黑抬升档）；卡片一律 16px、输入 8px（rounded.md），
 *   按钮全药丸 999px（Button 组件级钉住，全局 token 不能上 999 —— 输入框是 8px）；
 * - 文字阶梯全规格值：ink #000000 / hairline-mid #4B4B4B / body #5E5E5E / mute #AFAFAF；
 * - 输入 = 灰填充无边框风（规格 text-input bg canvas-soft #EFEFEF + rounded 8）；
 * - 表头 = canvas-soft 灰面 + 500 字重（规格 ex-data-table-cell）；
 * - 阴影两档取规格海拔原文：Level 1 `rgba(0,0,0,.12) 0 4px 16px`（浮层）/
 *   Level 2 `rgba(0,0,0,.16) 0 4px 16px`（模态/toast）——按钮阴影全关（Level 0 扁平）；
 * - #0000EE 是系统唯一彩（浏览器默认链接蓝，规格 link-blue）→ colorInfo/colorLink；
 * - 语义三色：规格明言无语义色盘（「验证线索来自主黑」）→ 按「浏览器默认语义
 *   的克制化」推导（与 #0000EE 的系统 chrome 语声同理，先例：tesla/antd.ts）：
 *   success #1F7A3D / warning #A16207 / error #C8102E；
 * - 字重签名：标题 700、按钮/强调 500（fontWeightStrong 700；Button 500，
 *   规格「Don't promote button labels to 700」）；
 * - UberMove/UberMoveText（私有）→ Inter 替身（规格 官方推荐）+ 系统/web-safe 栈。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/uber.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary         (#000000，规格 primary = ink black)
 *   colorPrimaryHover  = 提亮一阶          (#3D3D3D，规格未记 → 派生)
 *   colorPrimaryActive = --primary-active  (#282828 = 规格 black-elevated)
 *   colorInfo/colorLink = --info/--link    (#0000EE，规格 link 浏览器默认蓝)
 *   colorInfoBg        = --info-soft       (#F1F1FE 蓝 6% 薄纱，派生)
 *   colorInfoBorder    = --hairline        (#E2E2E2 = surface-pressed 兼分隔线)
 *   colorText 四阶      = ink/hairline-mid/body/mute (#000000/#4B4B4B/#5E5E5E/#AFAFAF)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 白画布)
 *   colorBorder 双阶 + colorSplit = #E2E2E2/#EFEFEF（rowBorder / canvas-soft）
 *   borderRadius 8 / LG 16 = 规格 rounded.md（输入）/ rounded.xl（卡）；
 *   Button 999 = 规格 rounded.pill（药丸签名）
 *   boxShadow/Secondary = 规格 Level 1 / Level 2 原文
 */
export const uberAntdTheme: ThemeConfig = {
  token: {
    // 黑药丸 = 品牌唯一转化色
    colorPrimary: '#000000',
    colorPrimaryHover: '#3D3D3D',
    colorPrimaryActive: '#282828',
    // info / link = #0000EE：系统唯一彩（浏览器默认链接蓝）
    colorInfo: '#0000EE',
    colorLink: '#0000EE',
    colorLinkHover: '#0000C7',
    colorLinkActive: '#00008F',
    // info Alert 钉成「蓝 6% 薄纱 + surface-pressed 线」（灰系软面 + 唯一彩）
    colorInfoBg: '#F1F1FE',
    colorInfoBorder: '#E2E2E2',
    // 语义三色（规格无语义盘 → 浏览器默认语义克制化推导）
    colorSuccess: '#1F7A3D',
    colorWarning: '#A16207',
    colorError: '#C8102E',
    // 文字阶梯：ink / hairline-mid / body / mute（规格灰阶四级原文）
    colorText: '#000000',
    colorTextHeading: '#000000',
    colorTextSecondary: '#4B4B4B',
    colorTextTertiary: '#5E5E5E',
    colorTextQuaternary: '#AFAFAF',
    // 白画布：页面、容器、浮层全白，深度靠灰面与投影两档
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：surface-pressed #E2E2E2（行分隔/控件边）/ canvas-soft #EFEFEF（软线）
    colorBorder: '#E2E2E2',
    colorBorderSecondary: '#EFEFEF',
    colorSplit: '#EFEFEF',
    // focus 光环 = 黑 15%（黑是主操作，焦点即墨）
    controlOutline: 'rgba(0, 0, 0, 0.15)',
    // 圆角：输入/小件 8（rounded.md）、大面卡 16（rounded.xl）；按钮药丸见 components
    borderRadius: 8,
    borderRadiusSM: 8,
    borderRadiusLG: 16,
    // 标题 700、按钮 500 是字重签名
    fontWeightStrong: 700,
    // 阴影两档 = 规格 Level 1 / Level 2 原文（浮层 / 模态）；按钮阴影在 Button 关
    boxShadow: 'rgba(0, 0, 0, 0.12) 0px 4px 16px 0px',
    boxShadowSecondary: 'rgba(0, 0, 0, 0.16) 0px 4px 16px 0px',
    // UberMove/UberMoveText → Inter 官方替身（规格「Note on Font Substitutes」）
    fontFamily:
      '"UberMove", "UberMoveText", Inter, system-ui, -apple-system, "Segoe UI", "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
  },
  components: {
    Button: {
      // 药丸 999px 是品牌几何签名（规格「The pill IS the brand」）；大钮 16 走全局 LG
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 16,
      // 按钮字重 500（规格 button-md 16/500；「Don't promote to 700」）
      fontWeight: 500,
      // Level 0 扁平：药丸不带投影（浮动白药丸的 Level 3 本仓骨架无对应物）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = canvas-soft 灰高亮 + ink 黑（category-button 语汇）
      itemSelectedBg: '#EFEFEF',
      itemSelectedColor: '#000000',
    },
    Table: {
      // 表头 = canvas-soft 灰面 + ink 500（规格 ex-data-table-cell 原文）
      headerBg: '#EFEFEF',
      headerColor: '#000000',
      headerSplitColor: 'transparent',
      headerBorderRadius: 8,
      rowHoverBg: '#F3F3F3',
      borderColor: '#E2E2E2',
    },
    Tooltip: {
      // tooltip = 纯黑（品牌唯一真黑面的浮层化身）
      colorBgSpotlight: '#000000',
    },
    Input: {
      // 输入 = 灰填充无边框风（规格 text-input bg canvas-soft + rounded 8）
      colorBgContainer: '#EFEFEF',
      borderRadius: 8,
    },
    InputNumber: {
      colorBgContainer: '#EFEFEF',
    },
    Segmented: {
      // tab-toggle 药丸：灰轨 + 白选中面（规格 button-tab-translucent 近似）
      trackBg: '#EFEFEF',
      itemSelectedBg: '#FFFFFF',
      itemSelectedColor: '#000000',
    },
    Tag: {
      // 标签 = 药丸（category chip 几何）
      borderRadiusSM: 999,
    },
  },
}
