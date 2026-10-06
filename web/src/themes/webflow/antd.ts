import type { ThemeConfig } from 'antd'

/**
 * webflow 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Webflow 设计语言（白画布 + 近黑转换色 + 五色类别系统）：
 * - 两色转换层级：Ink Black #080808 是唯一主 CTA / 标题 / wordmark 色
 *   （「Near-black is the conversion colour」），白底发丝线为次级按钮；
 * - 五停 chromatic 类别色（紫/粉/蓝/橙/绿）只作整面填充（类别卡），不作按钮底
 *   → 管理台里落在 brand-mark 三条、图表墨水与 badge 蓝族，不进交互件；
 * - 蓝族三档：accent-blue-info #146EF5（badge-info）/ accent-blue-deep #006ACC
 *   （emphasis links）→ info 与 link 各就各位；
 * - 字重天花板 600：display 600 / 正文与按钮 500 / caption 550，全站无 700+；
 * - 圆角纪律：按钮·徽章 4px（rounded.sm「tight engineered」）、卡片 8px（rounded.md），
 *   品牌永不用药丸 CTA（Tag 也保持 4px）；
 * - 海拔 = 五停分层投影（原文配方），
 *   modal 级用 Level 4 Heavy Modal 原文配方。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/webflow.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#080808 Ink Black，规格 primary/ink 同值)
 *   colorPrimaryHover/Active = 墨阶微亮两档      (#333333 / #1F1F1F，近黑无法压深的推导位)
 *   colorInfo               = --info            (#146EF5 accent-blue-info「badge-info blue」)
 *   colorInfoBg             = --info-soft 同源  (#EEF4FE = #146EF5 8% 薄纱铺白)
 *   colorInfoBorder         = --hairline        (#D8D8D8 规格 hairline，badge-info-soft 白底发丝线)
 *   colorLink               = --link            (#006ACC accent-blue-deep「emphasis links」)
 *   colorSuccess/Warning/Error = 语义三色
 *                             (#00A31A / #B37500 / #EE1D36；绿/黄为白画布可读性的压深推导，
 *                              红 #EE1D36 为规格字面)
 *   colorText 四阶          = #080808 / #363636 / #5A5A5A / #898989（规格 Text 六级坍缩）
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 白画布)
 *   colorBorder 双阶 + colorSplit = #D8D8D8 / #EBEBEB（规格 hairline + 推导浅分隔）
 *   borderRadius 4          = 「canonical button + badge radius」
 *   borderRadiusLG 8        = 「card chrome 8px」
 *   fontWeightStrong 500    = 「weight ceiling 600，正文强调 500」
 *   boxShadow               = 五停分层投影原文
 *   boxShadowSecondary      = 原文
 *   fontFamily              = --font-sans（WF Visual Sans 的 Inter 近似，规格替身首选）
 */
export const webflowAntdTheme: ThemeConfig = {
  token: {
    // Ink Black：唯一主 CTA / 标题色（「the brand's primary conversion colour」）
    colorPrimary: '#080808',
    // 近黑无法再压深 → hover/active 沿墨阶微亮（Webflow 黑按钮 hover 泛灰的推导位）
    colorPrimaryHover: '#333333',
    colorPrimaryActive: '#1F1F1F',
    // info = badge-info 蓝（「Info Blue — info badge / notification」）
    colorInfo: '#146EF5',
    // info Alert 钉成「蓝 8% 薄纱 + 发丝线」（badge-info-soft：白底 + info 蓝字）
    colorInfoBg: '#EEF4FE',
    colorInfoBorder: '#D8D8D8',
    // 链接 = accent-blue-deep（「the deeper blue used for emphasis links」）
    colorLink: '#006ACC',
    colorLinkHover: '#0057A6',
    colorLinkActive: '#0057A6',
    // 语义三色：红为规格字面；绿/黄按白画布文字对比度压深推导
    //（#00D722/#FFAE13 是整面填充位的全饱和值，见 css --success/--warning）
    colorSuccess: '#00A31A',
    colorWarning: '#B37500',
    colorError: '#EE1D36',
    // 文字阶梯：Ink / Body / Body-Mid / Mute（规格 Text 六级坍缩为 antd 四阶）
    colorText: '#080808',
    colorTextHeading: '#080808',
    colorTextSecondary: '#363636',
    colorTextTertiary: '#5A5A5A',
    colorTextQuaternary: '#898989',
    // 白画布：页面、容器、浮层全白，靠发丝线与分层投影分隔
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline #D8D8D8（控件/卡 chrome/分隔）+ 推导浅一档 #EBEBEB
    colorBorder: '#D8D8D8',
    colorBorderSecondary: '#EBEBEB',
    colorSplit: '#EBEBEB',
    // 圆角：4px 按钮/徽章/小件；8px 卡片与面板
    borderRadius: 4,
    borderRadiusSM: 4,
    borderRadiusLG: 8,
    // 字重纪律：strong 只到 500，600 留给标题（css 片段钉 display 位）
    fontWeightStrong: 500,
    // Level 2 五停分层投影原文（featured cards 的唯一海拔语言）
    boxShadow: '0 54px 22px rgba(0, 0, 0, 0.01), 0 30px 18px rgba(0, 0, 0, 0.04), 0 13px 13px rgba(0, 0, 0, 0.08), 0 3px 7px rgba(0, 0, 0, 0.09)',
    // Level 4 Heavy Modal 原文（modal / dialog 级强调）
    boxShadowSecondary: '0 24px 24px rgba(0, 0, 0, 0.26), 0 6px 13px rgba(0, 0, 0, 0.29)',
    // WF Visual Sans 的开源替身首选 = Inter
    fontFamily:
      'Inter, system-ui, -apple-system, "Segoe UI", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(8, 8, 8, 0.08)' },
    Button: {
      // 按钮字重 500（规格 button-md 16px/500）；黑 CTA 扁平无阴影
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = 灰面微高亮 + Ink 墨（ex-app-shell-row：active 指示 = primary 近黑）
      itemSelectedBg: '#F5F5F5',
      itemSelectedColor: '#080808',
    },
    Table: {
      // 白底扁平表头 + Body-Mid 标签；行 hover = 浅灰；行线 = hairline
      headerBg: '#FFFFFF',
      headerColor: '#5A5A5A',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F5F5F5',
      borderColor: '#D8D8D8',
    },
    Tooltip: {
      colorBgSpotlight: '#080808',
    },
  },
}
