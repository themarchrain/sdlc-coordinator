import type { ThemeConfig } from 'antd'

/**
 * replicate 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Replicate 设计语言 = 「AI 实验室笔记 × 印刷杂志」：暖白 cream 画布
 * #F9F7F3（规格铁律：页面级永不纯白），卡/输入/浮层 = 唯一出现白的地方
 * （surface-card #FFFFFF）；热橙 #EA2804 是戳记（stamp），只给主 CTA /
 * hero / 行内链接三职，按压回落 primary-deep #C01F00；文字阶梯
 * Ink #202020 → Body #3A3A3A → Charcoal #575757 → Ash #8D8D8D（规格四档）；
 * 边线 = 低对比暖墨 hairline rgba(32,32,32,.12)，聚焦环是功能性蓝
 * （规格 ring-focus 系）；海拔 = 色彩分层（cream→bone→dark 反转），
 * 阴影只留给漂浮浮层（规格唯一软投影 0 8px 24px rgba(32,32,32,.08)）。
 * 每个交互面全药丸（rounded.full 是规格「friendly precision」）→
 * Button 钉 9999；内容卡停在 10/16。algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/replicate.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#EA2804，橙戳记 CTA)
 *   colorPrimaryActive      = --primary-active  (#C01F00，规格 button-primary-pressed)
 *   colorPrimaryHover       = #FF6A3D（规格 hero-glow 浅橙；no-hover policy 下
 *                              取规格内的浅橙作悬停档）
 *   colorInfo               = --info            (#3B82F6，规格 ring-focus 的实心档)
 *   colorLink               = --link            (#EA2804，规格 link = primary 同橙)
 *   colorSuccess            = --success         (#2B9A66，规格 badge-success)
 *   colorWarning/colorError = 派生（规格无此二色，见行内注释）
 *   colorText 四阶          = Ink / Body / Charcoal / Ash（规格文字阶梯）
 *   colorBgLayout           = --canvas          (#F9F7F3 cream，页面级)
 *   colorBgContainer/Elevated = --canvas-card   (#FFFFFF，白只上卡面)
 *   colorBorder 双阶 + colorSplit = hairline 族 rgba(32,32,32,.12/.08)
 *   borderRadius 10 / LG 16 = 规格 rounded.md / rounded.lg（内容卡「never sharp,
 *                              never full」）；Button 药丸在 components 钉
 *   controlHeight 44        = 规格 button/text-input height 44px（WCAG AAA）
 *   fontFamily              = --font-sans（basier-square 的规格替身 Inter 栈）
 */
export const replicateAntdTheme: ThemeConfig = {
  token: {
    // 橙戳记主 CTA
    colorPrimary: '#EA2804',
    // 规格 no-hover policy：只记 pressed #C01F00；hover 取规格 hero-glow 浅橙
    colorPrimaryHover: '#FF6A3D',
    colorPrimaryActive: '#C01F00',
    // info = 聚焦环蓝（规格 ring-focus rgba(59,130,246,.5) 的实心档）
    colorInfo: '#3B82F6',
    // 链接 = 品牌橙（规格 link 与 primary 同色，「pulling links into the brand accent」）
    colorLink: '#EA2804',
    colorLinkHover: '#C01F00', // 文字链接悬停压深（浅橙在 cream 上不可读）
    colorLinkActive: '#C01F00',
    // info Alert = 聚焦蓝 8% 水彩 + 同蓝 35% 边（功能性蓝，非装饰）
    colorInfoBg: 'rgba(59, 130, 246, 0.08)',
    colorInfoBorder: 'rgba(59, 130, 246, 0.35)',
    // 语义三色：success = 规格 badge-success；warning/error 规格未给，
    // 按暖调推导（ochre 琥珀 / 品红偏冷的红，均与橙主色拉开）
    colorSuccess: '#2B9A66',
    colorWarning: '#8A6A1E',
    colorError: '#CF222E',
    // 文字阶梯（规格：Ink / Body / Charcoal / Ash）
    colorText: '#202020',
    colorTextSecondary: '#3A3A3A',
    colorTextTertiary: '#575757',
    colorTextQuaternary: '#8D8D8D',
    // cream 画布 + 白卡面（规格：白只出现在卡/输入/浮层）
    colorBgLayout: '#F9F7F3',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线：hairline 暖墨 alpha（控件边 .12 / 分隔 .08，规格单轨 + 半档推导）
    colorBorder: 'rgba(32, 32, 32, 0.12)',
    colorBorderSecondary: 'rgba(32, 32, 32, 0.08)',
    colorSplit: 'rgba(32, 32, 32, 0.12)',
    // 圆角：默认 10 = rounded.md（model 卡/代码井）；SM 6 = rounded.sm；
    // LG 16 = rounded.lg（pricing 档）。按钮/输入药丸在 components 与 css 片段钉
    borderRadius: 10,
    borderRadiusSM: 6,
    borderRadiusLG: 16,
    // 字重：basier 600 = 按钮/标题档（规格 button-md 600），正文 400 不抬 500
    fontWeightStrong: 600,
    // 海拔 = 色彩分层；仅存的软投影给浮层（规格 Level 4 soft drop 原值）
    boxShadow: '0 8px 24px rgba(32, 32, 32, 0.08)',
    boxShadowSecondary: '0 8px 24px rgba(32, 32, 32, 0.08)',
    // 规格 touch targets：按钮与输入 44px（WCAG AAA）
    controlHeight: 44,
    // basier-square（专有族，禁外部加载）→ 规格替身 Inter + 系统/CJK 栈
    fontFamily:
      '"basier-square", Inter, "Segoe UI", system-ui, -apple-system, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 全药丸（规格 Do：「Set every interactive element to rounded.full」）
      borderRadius: 9999,
      // 按钮字重 600（规格 button-md 16px/600）；平面体系三态阴影全关
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = bone 内嵌面（规格 Level 2「bone inset」分层语汇）
      itemSelectedBg: '#F3F0E8',
      itemSelectedColor: '#202020',
    },
    Table: {
      // bone 表头 + Charcoal 标签 + 透明缝；行 hover = cream 回落
      headerBg: '#F3F0E8',
      headerColor: '#575757',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F9F7F3',
      borderColor: 'rgba(32, 32, 32, 0.08)',
    },
    Tooltip: {
      colorBgSpotlight: '#202020', // surface-dark 代码井同面
    },
  },
}
