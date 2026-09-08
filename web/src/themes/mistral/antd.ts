import type { ThemeConfig } from 'antd'

/**
 * mistral 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Mistral AI 设计语言 = 大气日落编辑风：暖奶油黄画布（cream #FFF8E0）+
 * 白卡（card-base 白底、beige-deep #E6D5A8 边）+ 饱和橙 #FA520F 唯一 CTA
 * （按压回落 Orange Deep #CC3A05）。几何 sober/editorial：按钮 8px、卡片 12px、
 * 禁药丸（badge 才用 full）。字重 500/600，展示级交给近衬线 PP Editorial Old
 * （CSS 令牌 --font-display，Georgia/Times 近似）。海拔克制：控件级 1-2px 微影，
 * 悬浮层才上 mockup/modal 级投影。algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/mistral.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#FA520F，规格「saturated orange primary CTA」)
 *   colorPrimaryActive      = --primary-active  (#CC3A05，规格 button-primary-pressed 字面值)
 *   colorPrimaryHover       = 派生 #FF6B2E      (规格 无 hover 政策；亮一档的落日橙，active 才压深)
 *   colorInfo               = --info            (#1F1F1F，badge-dark 编辑墨代替 info 彩色 —— 调色板
 *                                              纪律「不引入日落系之外的强调色」)
 *   colorInfoBg/colorInfoBorder = --info-soft / beige-deep (#FFF8E0 + #E6D5A8，cream 编辑式 notice)
 *   colorSuccess/Warning/Error = 派生语义三色（规格 仅给 link，语义态按日落调色板推导：
 *                              暖调绿 / 芥末琥珀 / 深落日红，文字级对比度）
 *   colorText 四阶           = Ink/Ink Tint/Steel/Stone (#1F1F1F / #3D3D3D / #6A6A6A / #8A8A8A)
 *   colorBgLayout            = --canvas         (#FFF8E0，暖奶油画布)
 *   colorBgContainer/Elevated = --canvas-card   (#FFFFFF，白卡靠 beige-deep 边分隔)
 *   colorBorder 双阶 + colorSplit = hairline-strong/hairline/hairline-soft
 *                                              (#C7C7C7 控件边 / #E5E5E5 / #EDEDED，规格 边线三级)
 *   borderRadius 8           = 规格 rounded.md（按钮/输入「not pills」）
 *   borderRadiusLG 12        = 规格 rounded.lg（卡片主导圆角）
 *   boxShadow                = 规格 Level 1 微影；boxShadowSecondary = Level 4 modal
 *   fontFamily               = --font-sans（Inter 直用 + 系统栈回退）
 *   motion 三档 150–200ms    = 规格 Known Gaps「recommend 150–200ms ease」
 */
export const mistralAntdTheme: ThemeConfig = {
  token: {
    // Mistral Orange：唯一 CTA 色（Brand & Accent）
    colorPrimary: '#fa520f',
    colorPrimaryHover: '#ff6b2e',
    colorPrimaryActive: '#cc3a05',
    // info = 编辑墨（badge-dark 语汇）：日落调色板外的彩色一律不引入
    colorInfo: '#1f1f1f',
    colorInfoBg: '#fff8e0',
    colorInfoBorder: '#e6d5a8',
    // link 与 primary 同橙（Semantic：link matches primary orange）
    colorLink: '#fa520f',
    colorLinkHover: '#ff6b2e',
    // 语义三色（规格 未给 → 按日落调色板推导的文字级语义色）
    colorSuccess: '#1e7b45',
    colorWarning: '#a16207',
    colorError: '#c53030',
    // 文字阶梯：Ink / Ink Tint / Steel / Stone（Text 四级原文）
    colorText: '#1f1f1f',
    colorTextSecondary: '#3d3d3d',
    colorTextTertiary: '#6a6a6a',
    colorTextQuaternary: '#8a8a8a',
    // 暖奶油画布 + 白卡：卡靠 beige-deep 边与留白分隔，不靠阴影
    colorBgLayout: '#fff8e0',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 边线三级：Hairline Strong（控件边）/ Hairline / Hairline Soft（Surface）
    colorBorder: '#c7c7c7',
    colorBorderSecondary: '#e5e5e5',
    colorSplit: '#ededed',
    // 圆角：8px 按钮/输入（rounded.md，非药丸）；12px 卡（rounded.lg 主导）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 字重：UI 500/600（caption-bold 600 / heading 500）
    fontWeightStrong: 600,
    // 海拔克制：默认 Level 1 微影；浮层用 Level 4 modal（字面值）
    boxShadow: '0 1px 2px rgba(0, 0, 0, 0.04)',
    boxShadowSecondary: '0 16px 48px -8px rgba(0, 0, 0, 0.12)',
    // 150–200ms ease（规格 Known Gaps 推荐区间）
    motionDurationFast: '0.15s',
    motionDurationMid: '0.18s',
    motionDurationSlow: '0.2s',
    // Inter 直用（规格 UI prose 指定族），加 CJK 系统回退
    fontFamily:
      'Inter, system-ui, -apple-system, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 500（规格 button-md 14px/500）；三态阴影全关（平面编辑感）
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = cream-deeper 微高亮 + Ink（规格 badge-cream 同底）
      itemSelectedBg: '#fff0c2',
      itemSelectedColor: '#1f1f1f',
    },
    Table: {
      // 白底表头 + Steel 标签 + 透明分隔缝；行 hover = cream 轻染
      headerBg: '#fffaeb',
      headerColor: '#6a6a6a',
      headerSplitColor: 'transparent',
      rowHoverBg: '#fff8e0',
      borderColor: '#e5e5e5',
    },
    Tooltip: {
      colorBgSpotlight: '#1f1f1f',
    },
  },
}
