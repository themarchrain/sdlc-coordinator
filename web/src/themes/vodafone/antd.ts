import type { ThemeConfig } from 'antd'

/**
 * vodafone 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Vodafone 设计语言 = 白画布 + 唯一红：品牌红 #E60000 包揽全部 CTA / 演讲符红章 /
 * destructive 信号（「the primary red doubles as validation / destructive signal，
 * no second accent colour」）；近黑墨 #25282B 承担暗带与文字，灰阶只有 body/mute 两级软灰。
 * 交互件一律药丸（CTA 60px / 徽标 32px），卡面 6px；全站零软阴影
 * （规格 Don't：「Don't drop a soft drop-shadow on cards」）。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/vodafone.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#E60000，规格「single brand accent」)
 *   colorPrimaryHover/Active = 红压深两阶        (#CC0000 / #B30000，规格「never desaturated」→ 只压明度)
 *   colorInfo               = --info            (#25282B，规格禁第二强调色 → 近黑墨承担 info)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline-strong
 *                                                (#F2F2F2 软面 + #BEBEBE mute 边，钉住「软面 + mute 边」)
 *   colorLink               = --link            (#E60000，outline-red 按钮同源：白底红字)
 *   colorSuccess/Warning/Error = 语义三色（派生：规格无语义段；error = 品牌红字面值）
 *   colorText 四阶           = ink #25282B / #525252(派生中点) / body #7E7E7E / mute #BEBEBE
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF，白画布)
 *   colorBorder 双阶 + colorSplit = mute #BEBEBE（规格边色）/ #E6E6E6（派生发丝线）
 *   borderRadius 6          = 规格 rounded.card「canonical content radius」
 *   fontFamily              = --font-sans（Vodafone 定制族的 Inter 800 替代栈，禁外部加载）
 */
export const vodafoneAntdTheme: ThemeConfig = {
  token: {
    // Vodafone Red：唯一品牌色（「never desaturated」）
    colorPrimary: '#E60000',
    colorPrimaryHover: '#CC0000',
    colorPrimaryActive: '#B30000',
    // info = 近黑墨：规格只有红一个强调色，中性墨承担 info（css --info 同源）
    colorInfo: '#25282B',
    colorInfoBg: '#F2F2F2',
    colorInfoBorder: '#BEBEBE',
    // 链接 = 红（规格 button-outline-red：白底红字的交互语汇）
    colorLink: '#E60000',
    colorLinkHover: '#CC0000',
    // 语义三色（规格无语义段：error = 品牌红兼任 destructive；success/warning = 标准语义克制化）
    colorSuccess: '#0E8A3E',
    colorWarning: '#9A6700',
    colorError: '#E60000',
    // 文字阶梯坍缩为规格三级：ink / body / mute（#525252 为 ink↔body 中点派生）
    colorText: '#25282B',
    colorTextSecondary: '#525252',
    colorTextTertiary: '#7E7E7E',
    colorTextQuaternary: '#BEBEBE',
    // 白画布：页面、容器、浮层全白，靠发丝线与极性翻转分隔
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：mute #BEBEBE（规格边色）/ #E6E6E6（派生：mute 叠白 40% 发丝线）
    colorBorder: '#BEBEBE',
    colorBorderSecondary: '#E6E6E6',
    colorSplit: '#E6E6E6',
    // 圆角：卡/输入/面板一律 6px（规格 rounded.card）；药丸只属于按钮与徽标（见 Button）
    borderRadius: 6,
    borderRadiusSM: 6,
    borderRadiusLG: 6,
    // 强调字重 700（规格 display-sm/caption-strong 的 700 档；hero 800 见 css 片段）
    fontWeightStrong: 700,
    // 零软阴影体系（「surface contrast, not shadow」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Vodafone 定族禁外部加载 → 规格「Font Substitutes」指定的 Inter 800 替代栈
    fontFamily:
      '"Inter", system-ui, -apple-system, "Segoe UI", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // CTA 药丸（规格 pill-lg 60px 在 32–40px 控件高上 = 全药丸）；标签 400（规格 button-md 18/400）
      borderRadius: 9999,
      fontWeight: 400,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Input: {
      // 输入 hover = body 灰、聚焦 = 红（红即唯一 CTA affordance）
      hoverBorderColor: '#7E7E7E',
      activeBorderColor: '#E60000',
    },
    Menu: {
      // 下拉当前项 = canvas-soft 微高亮 + ink 墨（规格 badge-chip 底色语汇）
      itemSelectedBg: '#F2F2F2',
      itemSelectedColor: '#25282B',
    },
    Table: {
      // 表头 = canvas-soft 底（ex-data-table headerBackground）+ 次级墨；行 hover = canvas-soft
      headerBg: '#F2F2F2',
      headerColor: '#525252',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F2F2F2',
      borderColor: '#E6E6E6',
    },
    Tooltip: {
      // 墨底反转面（规格 ink 暗带的最小表达）
      colorBgSpotlight: '#25282B',
    },
  },
}
