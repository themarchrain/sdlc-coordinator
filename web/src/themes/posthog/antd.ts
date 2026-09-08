import type { ThemeConfig } from 'antd'

/**
 * posthog 主题的 antd ThemeConfig —— 自设计语言 派生
 * （frontmatter 令牌表 + 正文角色句）。
 *
 * PostHog 产品分析品牌：#EEFE9 暖灰白（cream）画布连绵到底 + 白卡薄橄榄边
 * 平铺其上；单一饱和黄橙 #F7A501 CTA（「Get started — free」）承载全部主操作，
 * 签名一：黄底配深橄榄字 #23251D（on-primary 字面值，只钉在 Button.primaryColor，
 * 不动全局 colorTextLightSolid——tooltip 等暗底浮层仍需白字）；签名二：选中态
 * 完全反转（pill-tab-active 墨底白字），管理台导航激活 = 墨字 + 橄榄纱（css 层）。
 * IBM Plex Sans Variable 全字重阶梯 400/500/600/700/800，6px 卡圆角（rounded.md），
 * 零投影（规格 Don't「cards sit flat on cream with thin olive borders only」）；
 * 四彩 soft 通告族（blue/green/red/purple）只进 info/语义位。algorithm 未指定
 * = antd 默认算法（亮色）。
 *
 * 迁移取舍（规格未给、按设计语言推导，均在行内注明）：
 *   - 规格无 hover 态（系统政策）→ pressed #DD9001 映射为 hover，
 *     active #B17816 映射为 active；
 *   - 规格警示横幅（⚠️ Warning/Caution）走 accent-red → error = #CD4239 字面值；
 *     colorWarning 派生标准琥珀 #97650D（规格无真琥珀警示色）；
 *   - colorSuccess = accent-green #2C8C66 字面值；
 *   - colorLink = link-teal #1078A3（link-inline 组件绑定值，品牌主内联链色），
 *     hover/active 派生深化 #0D6488；
 *   - controlHeight 36 = text-input 字面高（按钮 40 由 Button 组件钉住，
 *     规格 Touch Targets 输入即 36）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/posthog.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#F7A501 黄橙 CTA)
 *   colorPrimaryHover       = #DD9001           (primary-pressed 字面值作 hover)
 *   colorPrimaryActive      = --primary-active  (#B17816 primary-active 字面值)
 *   colorInfo               = --info            (#2C84E0 accent-blue，Tip/Info 族)
 *   colorLink               = --link            (#1078A3 link-teal)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline
 *                                                (#DCEAF6 accent-blue-soft 字面值 + #BFC1B7 hairline)
 *   colorSuccess/Warning/Error = 语义三色（green 字面值 / 派生琥珀 / accent-red 字面值）
 *   colorText 四阶           = ink #23251D / body #4D4F46 / mute #6C6E63 / ash #9B9C92
 *   colorBgLayout           = --canvas          (#EEEFE9 cream 画布)
 *   colorBgContainer/Elevated = --canvas-card   (#FFFFFF 白卡)
 *   colorBorder 双阶 + colorSplit = hairline #BFC1B7 / hairline-soft #DCDFD2
 *   borderRadius 6          = rounded.md（几乎全部组件的标准档）
 *   borderRadiusLG 8        = rounded.lg（稀有容器档）
 *   controlHeight 36        = text-input 字面高；Button 40 = button 字面高
 *   fontWeightStrong 700    = Plex 阶梯的强调档（display 800 由 css 片段承担）
 *   boxShadow/Secondary     = none（零投影系统）
 *   fontFamily              = --font-sans（IBM Plex Sans Variable → 规格回退栈）
 */
export const posthogAntdTheme: ThemeConfig = {
  token: {
    // PostHog Yellow：唯一饱和 CTA（sticky「Get started — free」的按钮版）
    colorPrimary: '#F7A501',
    colorPrimaryHover: '#DD9001', // primary-pressed 字面值作 hover（规格无 hover 态）
    colorPrimaryActive: '#B17816', // primary-active 字面值（deep gold 档）
    // info = accent-blue（banner-tip-blue「💡 Tip / Info」族）
    colorInfo: '#2C84E0',
    // link = link-teal（link-inline 组件绑定值，doc 内联链主色）
    colorLink: '#1078A3',
    colorLinkHover: '#0D6488', // 派生：teal 深化一档
    colorLinkActive: '#0D6488',
    // info Alert = accent-blue-soft 蓝粉彩 + hairline 橄榄边（banner-tip-blue 形态）
    colorInfoBg: '#DCEAF6',
    colorInfoBorder: '#BFC1B7',
    // 语义三色：green 字面值 / 派生琥珀 / accent-red 字面值（警示横幅即红族）
    colorSuccess: '#2C8C66',
    colorWarning: '#97650D', // 派生：规格警示走红，真琥珀未 captured
    colorError: '#CD4239',
    // 文字阶梯：ink / body / mute / ash（规格 Text 六级取用最频四级）
    colorText: '#23251D',
    colorTextSecondary: '#4D4F46',
    colorTextTertiary: '#6C6E63',
    colorTextQuaternary: '#9B9C92',
    // cream 画布连绵到底；容器与浮层 = 白卡（surface-card）
    colorBgLayout: '#EEEFE9',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline（1px 卡边）/ hairline-soft（卡内行分隔）
    colorBorder: '#BFC1B7',
    colorBorderSecondary: '#DCDFD2',
    colorSplit: '#DCDFD2',
    // 派生：focus-ring 字面 rgba(59,130,246,.5) 削弱为 18% 光环（antd 默认 halo 强度）
    controlOutline: 'rgba(59, 130, 246, 0.18)',
    // 圆角：rounded.md 6 为标准档；rounded.sm 4 给小件；rounded.lg 8 稀有容器
    borderRadius: 6,
    borderRadiusSM: 4,
    borderRadiusLG: 8,
    // 触达高：text-input 字面 36（规格 Touch Targets：输入即 36，AA 达标）
    controlHeight: 36,
    // 强调档 700（display 800 / 眉题 uppercase 由 css 片段承担）
    fontWeightStrong: 700,
    // 零投影系统：卡平铺 cream，仅薄橄榄边（规格 Don't「no drop shadows」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // IBM Plex Sans Variable → 规格回退栈（Plex → -apple-system → system-ui）
    fontFamily:
      "'IBM Plex Sans Variable', 'IBM Plex Sans', -apple-system, system-ui, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    // ui-monospace 内联 chip 切 + Source Code Pro 展示切（规格 Code 双族）
    fontFamilyCode: 'ui-monospace, "Source Code Pro", "Cascadia Code", Consolas, monospace',
  },
  components: {
    Button: {
      // 标准按钮 = rounded.md 6（药丸只属 sticky nav CTA，登录位由 css 片段钉）
      borderRadius: 6,
      borderRadiusSM: 6,
      borderRadiusLG: 8,
      // button-md 700 标签字重
      fontWeight: 700,
      controlHeight: 40, // button 字面高 40px
      // 签名：黄底深橄榄字（on-primary 字面值；只钉按钮位，tooltip 仍白字）
      primaryColor: '#23251D',
      // 零投影：三态阴影全关
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = button-secondary：soft surface 奶灰底 + 墨字
      defaultBg: '#E5E7E0',
      defaultColor: '#23251D',
      defaultBorderColor: '#E5E7E0',
      defaultHoverBg: '#DCDFD2', // hover = hairline-soft 沉一档
      defaultHoverColor: '#23251D',
      defaultHoverBorderColor: '#DCDFD2',
    },
    Menu: {
      // 下拉当前项 = accent-blue-soft 蓝粉彩 + 墨字（banner-tip 语汇）
      itemSelectedBg: '#DCEAF6',
      itemSelectedColor: '#23251D',
    },
    Table: {
      // doc-card 暖白表头 + body 灰标签 + hairline-soft 行分隔；行 hover = 暖白
      headerBg: '#FCFCFA',
      headerColor: '#4D4F46',
      headerSplitColor: '#DCDFD2',
      rowHoverBg: '#FCFCFA',
      borderColor: '#DCDFD2',
    },
    Tooltip: {
      // surface-dark 橄榄炭反转面（code-block 的浮层化身；白字走默认 light-solid）
      colorBgSpotlight: '#23251D',
    },
    Segmented: {
      // sub-nav strip 奶灰轨 / product-tab-active 白卡抬升
      trackBg: '#E5E7E0',
      itemSelectedBg: '#FFFFFF',
    },
    Select: {
      optionSelectedBg: '#DCEAF6',
    },
    Tag: {
      // badge-promo 全圆药丸（rounded.full 字面用法）
      borderRadiusSM: 9999,
    },
  },
}
