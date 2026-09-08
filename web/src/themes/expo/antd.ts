import type { ThemeConfig } from 'antd'

/**
 * expo 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Expo 设计语言 = 安静自信的开发者平台：纯白画布 + 近黑墨 #171717、
 * 唯一品牌电压 = 纯黑 CTA #000000（按压 #1A1A1A），蓝 #0D74CE 只做
 * 行内链接色（绝不落 CTA）；天空蓝渐变只存在于登录展示板（hero 专属
 * atmospheric wash）。Inter 单族（display 600 / body 400）+ JetBrains Mono
 * 码面；CTA 8px / 卡 12px，药丸只给徽章；海拔单档 0 4px 12px 4% 墨。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/expo.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#000000，规格「single brand voltage: pure black」)
 *   colorPrimaryActive      = --primary-active  (#1A1A1A，规格 button-primary-active 字面值)
 *   colorPrimaryHover       = 派生 #262626      (黑 CTA hover 微提亮，按压回落 #1A1A1A)
 *   colorInfo / colorLink   = --info / --link   (#0D74CE，Text Link Blue，行内链接语汇)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline-strong（蓝薄纱 + 面板强边）
 *   colorSuccess/Warning/Error = 语义三色（字面：
 *                              #16A34A / #AB6400 / #EB8E90 软红）
 *   colorText 四阶           = Ink/Body/Muted/Muted Soft (#171717 / #60646C / #999999 / #CCCCCC)
 *   colorBg 三面             = #FFFFFF 纯白（页面/容器/浮层同底，发丝线分隔）
 *   colorBorder 双阶 + colorSplit = hairline-strong/hairline/hairline-soft
 *                                              (#DCDEE0 控件与卡边 / #F0F0F3 / #F5F5F7)
 *   borderRadius 8           = 规格 rounded.md（CTA/输入开发者方言）
 *   borderRadiusLG 12        = 规格 rounded.lg（卡/码块主导圆角）
 *   fontWeightStrong 600     = 规格「display stays at 600，never 700」
 *   boxShadow 双档           = 单一软影层 0 4px 12px 4%（规格 Elevation 唯一阴影档）
 *   fontFamily               = --font-sans（Inter 直用 + 系统栈回退）
 */
export const expoAntdTheme: ThemeConfig = {
  token: {
    // 纯黑 CTA：唯一品牌电压（「Black is the only CTA fill」）
    colorPrimary: '#000000',
    colorPrimaryHover: '#262626',
    colorPrimaryActive: '#1a1a1a',
    // 蓝 = Text Link Blue：行内链接与 info 信号，绝不落 CTA
    colorInfo: '#0d74ce',
    colorInfoBg: '#f0f7fe',
    colorInfoBorder: '#dcdee0',
    colorLink: '#0d74ce',
    colorLinkHover: '#476cff', // Legal Link Blue 作 hover 提亮（规格第二蓝）
    // 语义三色（字面值；Error 为规格软红）
    colorSuccess: '#16a34a',
    colorWarning: '#ab6400',
    colorError: '#eb8e90',
    // 文字阶梯：Ink / Body / Muted / Muted Soft（四级原文）
    colorText: '#171717',
    colorTextSecondary: '#60646c',
    colorTextTertiary: '#999999',
    colorTextQuaternary: '#cccccc',
    // 纯白画布：页面、容器、浮层同底，靠发丝线与留白分隔
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 边线三级：Hairline Strong（控件/卡边）/ Hairline / Hairline Soft
    colorBorder: '#dcdee0',
    colorBorderSecondary: '#f0f0f3',
    colorSplit: '#f0f0f3',
    // 圆角：8px CTA/输入（rounded.md 开发者方言）；12px 卡（rounded.lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 展示字重封顶 600（「reads cleaner than 700」）
    fontWeightStrong: 600,
    // 海拔单档：唯一软影层（「single shadow tier」）
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.04)',
    boxShadowSecondary: '0 4px 12px rgba(0, 0, 0, 0.04)',
    // Inter 直用（规格单 sans 族），加 CJK 系统回退
    fontFamily:
      'Inter, -apple-system, system-ui, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 500（规格 button 14px/500）；黑 CTA 平面无光晕
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = Surface Strong 微高亮 + Ink（badge-pill 同底）
      itemSelectedBg: '#f0f0f3',
      itemSelectedColor: '#171717',
    },
    Table: {
      // 白底表头 + Body 标签 + 透明分隔缝；行 hover = canvas-soft 微沉
      headerBg: '#fafafa',
      headerColor: '#60646c',
      headerSplitColor: 'transparent',
      rowHoverBg: '#fafafa',
      borderColor: '#f0f0f3',
    },
    Tooltip: {
      colorBgSpotlight: '#171717',
    },
  },
}
