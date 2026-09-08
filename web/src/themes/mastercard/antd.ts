import type { ThemeConfig } from 'antd'

/**
 * mastercard 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Mastercard 设计语言 = 暖石报纸编辑风 + 超圆角轨道母题：
 * putty-cream 画布 #F3F0EE（「not white, not gray…paper of a premium annual report」）、
 * 三层暖面 canvas cream → lifted cream #FCFBFA → white（浮层/嵌面）、
 * 暖黑 Ink #141413 兼任主 CTA（「Ink Black: primary CTAs, headline text, footer」），
 * CTA 文字用奶油 #F3F0EE 而非纯白（「Text: Canvas Cream — not pure white」字面），
 * 圆角刻度跳过 8–12 中段：按钮 20 / 同意 24 / hero 40 / 全药丸 999+（表格），
 * 阴影 = 大弥散低透明度「呼吸垫」（L1 0 4px 24px @4% / L2 0 24px 48px @8%），
 * 字重窄幅 450 正文 / 500 标题 / 700 仅眉题，MarkForMC 全站单族。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生，这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/mastercard.css 的同名令牌手工保持同源：
 *   colorPrimary = --primary      (#141413 Ink Pill CTA；hover/active = Charcoal #262627 / 纯黑)
 *   colorInfo/colorLink = --info/--link (#3860BE Link Blue「inline links and
 *                                  informational callouts」)
 *   colorText 阶 = Ink #141413（正文即 Ink，「Body text: Ink Black at weight 450」）/
 *                  Charcoal #262627 / Slate Gray #696969（Neutrals 原文）
 *   colorBgLayout = --canvas      (#F3F0EE Canvas Cream)
 *   colorBgContainer = #FCFBFA    (Lifted Cream「nested raised sections」)
 *   colorBgElevated = #FFFFFF     (White「modal cards, secondary button fills」)
 *   colorBorder = #D1CDC7         (Dust Taupe 暖灰边)
 *   borderRadius 20 = Ink Pill「Radius: 20px」签名；LG 24 = 同意/浮层档；SM 6 = 微件
 *   fontWeightStrong 500 = 标题 500/-2%（700 只给 ≤14px 眉题，由 CSS 片段承担）
 *   boxShadow = Level 2「rgba(0,0,0,0.08) 0px 24px 48px」呼吸垫字面值
 *   fontFamily = MarkForMC + 规格声明回退栈「SofiaSans, Arial, sans-serif」
 */
export const mastercardAntdTheme: ThemeConfig = {
  token: {
    // Ink Black：主 CTA 药丸（Primary — Ink Pill）；hover/按压 = Charcoal / 纯黑
    colorPrimary: '#141413',
    colorPrimaryHover: '#262627',
    colorPrimaryActive: '#000000',
    // info/link = Link Blue：深尘蓝的内链与信息色（Semantic & Accent 原文）
    colorInfo: '#3860be',
    colorLink: '#3860be',
    colorLinkHover: '#2f5199',
    colorInfoBg: '#e4e4ea',
    colorInfoBorder: '#d1cdc7',
    // 语义三色：Signal Orange 是规格的单一进攻色，克制的功能位 = warning
    // （「must be used sparingly」）；success 规格无绿 → 推导暖调文字级；
    // error 禁用品牌红（Red「never as a UI color」）→ 推导暖红
    colorSuccess: '#3a7d50',
    colorWarning: '#cf4500',
    colorError: '#c03a2a',
    // 文字阶梯：正文即 Ink（「Body text: Ink Black」）；Charcoal 次档；Slate Gray 静音档
    colorTextBase: '#141413',
    colorTextHeading: '#141413',
    colorText: '#141413',
    colorTextSecondary: '#262627',
    colorTextTertiary: '#696969',
    colorTextQuaternary: '#696969',
    // 三层暖面：画布 cream → 容器 lifted cream → 浮层 white
    colorBgLayout: '#f3f0ee',
    colorBgContainer: '#fcfbfa',
    colorBgElevated: '#ffffff',
    // 暖灰边：Dust Taupe / 更轻一档
    colorBorder: '#d1cdc7',
    colorBorderSecondary: '#e4e0da',
    colorSplit: '#e4e0da',
    // 圆角：按钮 20 签名 / 浮层 24 / 微件 6（中段 8–12 缺席是刻意的）
    borderRadius: 20,
    borderRadiusSM: 6,
    borderRadiusLG: 24,
    // 字重：标题 500（-2% 字距由 CSS 片段承担）；700 只属于眉题
    fontWeightStrong: 500,
    // 阴影 = Level 2 / Level 1 呼吸垫字面值（大弥散、低透明度）
    boxShadow: '0 24px 48px rgba(0, 0, 0, 0.08)',
    boxShadowSecondary: '0 4px 24px rgba(0, 0, 0, 0.04)',
    // 触达高：「No link or button drops below 40px」
    controlHeight: 40,
    // MarkForMC（专有族，禁外部加载）+ 规格声明回退 SofiaSans, Arial
    fontFamily:
      '"MarkForMC", "MarkOffcForMC", SofiaSans, Arial, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮标签 500 + 紧字距（「Nav link / Button label: 16px weight 500 -3%」）
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = 暖墨微染 + Ink 文字（无彩高亮）
      itemSelectedBg: 'rgba(20, 20, 19, 0.05)',
      itemSelectedColor: '#141413',
    },
    Table: {
      // Lifted Cream 表头 + Slate Gray 标签；行 hover = 暖墨 4% 微染
      headerBg: '#fcfbfa',
      headerColor: '#696969',
      headerSplitColor: 'transparent',
      rowHoverBg: 'rgba(20, 20, 19, 0.04)',
      borderColor: '#e4e0da',
    },
    Tooltip: {
      // 浮层提示 = Ink 反转面（footer 同面）
      colorBgSpotlight: '#141413',
    },
    Input: {
      // 输入边：Ink 50%（搜索框「border 1px solid Ink Black at ~50% opacity」字面）
      hoverBorderColor: 'rgba(20, 20, 19, 0.5)',
      activeBorderColor: '#141413',
      activeShadow: '0 0 0 2px rgba(56, 96, 190, 0.25)',
    },
  },
}
