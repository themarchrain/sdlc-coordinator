import type { ThemeConfig } from 'antd'

/**
 * elevenlabs 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * ElevenLabs 设计语言（暖纸编辑杂志）：
 * - 离白画布 #f5f5f5 / 亮带 #fafafa / 纯白卡 #ffffff / 强表面 #f0efed；
 * - 主 CTA = 近黑墨药丸：#292524 底白字，按压回落 #0c0a09（规格 primary/primary-active 原文）；
 * - 无饱和 CTA 色：柔和渐变球（mint/peach/lavender/sky/rose）只做氛围装饰，
 *   绝不做按钮填充 / 文字色 / 组件底色（规格 Don't）；
 * - 文字阶梯：ink #0c0a09 / body #4e4e4e / body-strong #292524 / muted #777169 /
 *   muted-soft #a8a29e（stone 暖灰族）；
 * - 语义：success #16a34a / error #dc2626 规格；warning 与 info 无规格对应物 →
 *   warning 取暖琥珀 #b45309、info 取墨族中性 #44403c（编辑感「不引入饱和彩相」推导）；
 * - 字重纪律：display 恒 300（Waldenburg Light），body 400/500，永不 bold（fontWeightStrong=500）；
 * - 药丸几何：按钮/徽章一律 999px；输入 8px（rounded.md）；卡 16px（rounded.xl）；
 * - 海拔 = 发丝线 + 单层软投影 0 4px 16px rgba(0,0,0,.04)（规格 Soft drop 原文）；
 * - Tooltip 反转为墨底白字（ink 药丸的同构）。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/elevenlabs.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#292524 墨药丸)
 *   colorPrimaryActive = --primary-active (#0c0a09 规格按压)
 *   colorInfo          = --info           (#44403c 墨族中性，推导)
 *   colorLink          = --link           (#292524 墨链接 = 规格 button-tertiary-text)
 *   colorSuccess       = --success        (#16a34a 规格确认绿)
 *   colorWarning       = --warning        (#b45309 暖琥珀，推导)
 *   colorError         = --error          (#dc2626 规格 error 红)
 *   colorText 五阶      = ink #0c0a09 / #292524 / #4e4e4e / #777169 / #a8a29e（stone 阶梯）
 *   colorBgLayout      = --canvas         (#f5f5f5 离白画布)
 *   colorBgContainer   = --canvas-card    (#ffffff surface-card)
 *   colorBgSpotlight   = --tooltip-bg     (#292524 墨浮层)
 *   colorBorder        = --hairline-strong(#d6d3d1 规格 input 边) / secondary #e7e5e4
 *   boxShadow          = --shadow-panel   (规格 Soft drop 0 4px 16px rgba(0,0,0,.04))
 *   fontFamily         = --font-sans（Inter 即规格正文族）
 */
export const elevenlabsAntdTheme: ThemeConfig = {
  token: {
    // 墨药丸：主 CTA 唯一色
    colorPrimary: '#292524',
    colorPrimaryHover: '#1c1917',
    colorPrimaryActive: '#0c0a09',
    // info / link 走墨族：编辑感无饱和彩相（link = 规格 button-tertiary-text 墨链接）
    colorInfo: '#44403c',
    colorLink: '#292524',
    colorLinkHover: '#0c0a09',
    colorLinkActive: '#0c0a09',
    // 语义三色（success/error 规格；warning 暖琥珀推导）
    colorSuccess: '#16a34a',
    colorWarning: '#b45309',
    colorError: '#dc2626',
    // 文字阶梯：ink / body-strong / body / muted / muted-soft（Text 原文）
    colorText: '#292524',
    colorTextHeading: '#0c0a09',
    colorTextSecondary: '#4e4e4e',
    colorTextTertiary: '#777169',
    colorTextQuaternary: '#a8a29e',
    // 墨底白字（primary 药丸 / 实底标签文字）
    colorTextLightSolid: '#ffffff',
    // 表面阶梯：画布 #f5f5f5 / 容器白卡 / 浮层白卡 / 墨浮层
    colorBgBase: '#f5f5f5',
    colorBgLayout: '#f5f5f5',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    colorBgSpotlight: '#292524',
    // 边线双阶：hairline-strong #d6d3d1（规格 input 边）/ hairline #e7e5e4
    colorBorder: '#d6d3d1',
    colorBorderSecondary: '#e7e5e4',
    colorSplit: '#f0efed',
    controlOutline: 'rgba(41, 37, 36, .12)',
    // 圆角刻度：输入 8（rounded.md）/ 小件 6（rounded.sm）/ 大面 16（rounded.xl）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 16,
    // 海拔 = 单层软投影（规格 Soft drop 原文；浮层加深一档，推导）
    boxShadow: '0 4px 16px rgba(0, 0, 0, .04)',
    boxShadowSecondary: '0 8px 24px rgba(28, 25, 23, .08)',
    // Inter 即规格正文族；Waldenburg 授权字体只用于 display（CSS 层片段表达）
    fontFamily:
      'Inter, system-ui, -apple-system, "Segoe UI", "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
    // 字重纪律：body 400/500，永不 bold（规格 Don't：bolding shifts the brand voice）
    fontWeightStrong: 500,
  },
  components: {
    Button: {
      // 药丸是品牌交互形状：全部按钮 999px（规格 rounded.pill）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 按钮字 15px/500/0 字距（规格 typography.button）
      fontWeight: 500,
      fontSize: 15,
      // 默认按钮 = 透明描边药丸：1px hairline-strong 边 + 墨字（规格 button-outline）
      defaultBg: 'transparent',
      defaultColor: '#0c0a09',
      defaultBorderColor: '#d6d3d1',
      defaultHoverBg: '#fafafa',
      defaultHoverColor: '#0c0a09',
      defaultHoverBorderColor: '#777169',
      // 安静按钮：零投影（规格海拔只给悬浮卡）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#ffffff' },
    Steps: { colorWhite: '#ffffff' },
    Table: {
      // 编辑感表格：亮带表头 + muted 标签 + 发丝线行缝（规格 voice-row 语汇）
      headerBg: '#fafafa',
      headerColor: '#777169',
      headerSplitColor: 'transparent',
      rowHoverBg: '#fafafa',
      rowSelectedBg: '#f0efed',
      rowSelectedHoverBg: '#e7e5e4',
      fixedHeaderSortActiveBg: '#fafafa',
      borderColor: '#f0efed',
    },
    Modal: { contentBg: '#ffffff', headerBg: '#ffffff', footerBg: '#ffffff' },
    Drawer: { colorBgElevated: '#ffffff' },
    Tooltip: { colorBgSpotlight: '#292524' },
    Popover: { colorBgElevated: '#ffffff' },
    // 选中 = 白卡提升（surface-strong 轨道上的 lift，规格 badge/选中语汇）
    Select: { colorBgContainer: '#ffffff', optionSelectedBg: '#f0efed' },
    Input: {
      colorBgContainer: '#ffffff',
      // 规格 text-input：focus 边框转浓墨（thickens to 2px ink）
      activeBorderColor: '#292524',
    },
    InputNumber: { colorBgContainer: '#ffffff' },
    Form: { labelColor: '#292524' },
    Segmented: { trackBg: '#f0efed', itemSelectedBg: '#ffffff' },
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
    Menu: { itemSelectedBg: '#f0efed', itemSelectedColor: '#0c0a09' },
  },
}
