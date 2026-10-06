import type { ThemeConfig } from 'antd'

/**
 * mintlify 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Mintlify 设计语言 = 营销级文档基础设施：白画布 + 近黑墨，黑色药丸 CTA
 * （{colors.primary} #0A0A0A + rounded.full）主导一切转化位；Mint 绿
 * （#00D4A4 一族）只落在强调 CTA、激活指示与 focus 信号（text-input-focused
 * 2px 品牌绿）；Inter 承载全部 UI 散文、Geist Mono 只进代码；卡片一律 12px
 * （rounded.lg），紧凑件 8px，按钮徽章一律全药丸；海拔克制（卡片 Level 2
 * 0 4px 12px 8%、浮层 Level 3 深扩散投影），平面文档卡靠发丝线。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/mintlify.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#0A0A0A，规格 黑药丸 button-primary)
 *   colorPrimaryHover/Active = --primary-active (#1C1C1E charcoal，规格 button-primary-pressed
 *                                                「lifts to charcoal」；规格 no-hover 政策下
 *                                                hover/active 同落此档)
 *   colorInfo               = 深薄荷 #00B48A    (brand-green-deep，focus/激活信号族)
 *   colorInfoBg/colorInfoBorder = 薄荷薄纱       (brand-green-soft #7CEBCB 预混的确认面)
 *   colorLink               = --link            (#3772CF brand-tag「文档引用色」，骨架全局
 *                                                锚点角色借用；规格 button-link 本体是墨色透明底)
 *   colorSuccess/Warning/Error = 语义三色       (#1BA673 brand-annotate 绿 / #C37D0D brand-warn /
 *                                                #D45656 brand-error，全部规格原生)
 *   colorText 四阶           = Ink/Charcoal/Slate/Steel (#0A0A0A / #3A3A3C / #5A5A5C / #888888，
 *                                                四级原文)
 *   colorBgLayout/Container/Elevated = --canvas (#FFFFFF，Canvas White)
 *   colorBorder 双阶 + colorSplit = Hairline/Hairline-Soft (#E5E5E5 / #EDEDED)
 *   borderRadius 8 / SM 6 / LG 12 = rounded.md / sm / lg（刻度）
 *   fontWeightStrong 600    = 规格 排版全部标题字重 600（与 tesla 的 500 相反）
 *   boxShadow               = Level 2 卡片阴影原文 rgba(0,0,0,.08) 0 4px 12px
 *   boxShadowSecondary      = Level 3 mockup 深扩散原文 0 24px 48px -8px
 *   motionDuration 三档      = Known Gaps「150–200ms ease」推荐值
 *   fontFamily              = --font-sans（Inter + 规格回退栈，禁外部加载）
 */
export const mintlifyAntdTheme: ThemeConfig = {
  token: {
    // 黑药丸主 CTA：营销流主导动作（「Black-pill primary buttons dominate」）
    colorPrimary: '#0A0A0A',
    // 规格 no-hover 政策：hover/active 都落在文档化的 pressed charcoal（「lifts to charcoal」）
    colorPrimaryHover: '#1C1C1E',
    colorPrimaryActive: '#1C1C1E',
    // info = 深薄荷（brand-green-deep）：focus/激活信号是薄荷的法定场景
    colorInfo: '#00B48A',
    colorInfoBg: '#E7FBF6',
    colorInfoBorder: '#A6F0DF',
    // link = brand-tag 文档引用蓝（规格唯一为「引用」定义的色相；推导位）
    colorLink: '#3772CF',
    colorLinkHover: '#2F61B0',
    colorLinkActive: '#2F61B0',
    // 语义三色：全部规格原生（annotate 绿 / warn 琥珀 / error 红）
    colorSuccess: '#1BA673',
    colorWarning: '#C37D0D',
    colorError: '#D45656',
    // 文字阶梯：Ink / Charcoal / Slate / Steel（四级原文）
    colorText: '#0A0A0A',
    colorTextSecondary: '#3A3A3C',
    colorTextTertiary: '#5A5A5C',
    colorTextQuaternary: '#888888',
    // 白画布：页面、容器、浮层全白，靠发丝线与留白分隔
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：Hairline（控件边）/ Hairline-Soft（行分隔）
    colorBorder: '#E5E5E5',
    colorBorderSecondary: '#EDEDED',
    colorSplit: '#EDEDED',
    // 圆角：8 = rounded.md（输入/紧凑件）；6 = rounded.sm（徽章）；12 = rounded.lg（卡片）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 标题一律 600（规格 排版表：hero-display 到 heading-5 全部 600）
    fontWeightStrong: 600,
    // 海拔：Level 2 标准卡阴影 + Level 3 mockup 深扩散（原文）
    boxShadow: '0 4px 12px rgba(0, 0, 0, .08)',
    boxShadowSecondary: '0 24px 48px -8px rgba(0, 0, 0, .12)',
    // Known Gaps：动画时序未提取，按规格推荐 150–200ms
    motionDurationFast: '0.15s',
    motionDurationMid: '0.18s',
    motionDurationSlow: '0.2s',
    // Inter + 规格回退栈（-apple-system, BlinkMacSystemFont, 'Segoe UI'）
    fontFamily:
      'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(10, 10, 10, 0.08)' },
    Button: {
      // 全药丸（rounded.full）：规格 Do「Use rounded.full to every button and pill」
      borderRadius: 9999,
      // button-md 14px/500；三态阴影全关（营销按钮平面）
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = surface 微高亮 + 墨字（规格 sidebar-nav-item-active「bg surface, text ink」）
      itemSelectedBg: '#F7F7F7',
      itemSelectedColor: '#0A0A0A',
    },
    Table: {
      // 白底表头 + Steel 标签（「Steel: table headers」）；行 hover = surface-soft
      headerBg: '#FFFFFF',
      headerColor: '#5A5A5C',
      headerSplitColor: 'transparent',
      rowHoverBg: '#FAFAFA',
      borderColor: '#EDEDED',
    },
    Tooltip: {
      colorBgSpotlight: '#0A0A0A',
    },
  },
}
