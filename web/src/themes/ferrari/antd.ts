import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * ferrari 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Ferrari 设计语言（cinematic editorial，暗色）：
 * - 画布阶梯：近黑微暖 #181818（永不纯黑）→ 卡/面板 #303030（canvas-elevated）
 *   → 卡内交互面 #3c3c3c（规格暗灰渐变起点）→ 浮层同阶；
 * - 唯一品牌电压 Rosso Corsa #da291c：只落主 CTA / brand-mark（Cavallino）/
 *   导航指示条（F1 race-position highlight），其余 chrome 一律无彩；
 * - 文字四级：白 #ffffff / body #969696 / muted-soft #8f8f8f / muted #666666；
 * - 锐角哲学：CTA 全 0px（rounded.none），输入 4px（rounded.sm），
 *   药丸只归 badge（Tag borderRadiusSM 999）；
 * - display 恒 500 不加粗（fontWeightStrong 500；骨架标题的 500/700 分层见 ferrari.css 片段）；
 * - 焦点环 = Hypersail Yellow（规格「global focus-ring color」，黄色的合法场景）；
 * - 海拔 = 亮度阶梯 + 单一 soft 阴影 0 4px 8px rgba(0,0,0,.1)（规格唯一 documented 阴影层）；
 * - 语义：规格只有 info #4c98b9 / success #03904a / validation red #f13a2c。
 *   #f13a2c 直译为 error（站点唯一警示红）；warning 无独立规格值且与 error 撞色，
 *   推导琥珀 #e8a13c（暖调、#181818 上 8:1）——见 ferrari.css --warning 注。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/ferrari.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#da291c Rosso Corsa)
 *   colorPrimaryHover  = --(规格 hover)    (#9d2211，规格明文的压暗 hover)
 *   colorPrimaryActive = --primary-active (#b01e0a，规格按压态)
 *   colorInfo/colorInfoBg/colorInfoBorder = --info/--info-soft/--hairline 强线近似
 *   colorSuccess       = --success        (#03904a)
 *   colorWarning       = --warning        (#e8a13c 推导琥珀)
 *   colorError         = --error          (#f13a2c 规格 validation red)
 *   colorLink          = --link           (#ffffff 白链；hover 转红 = --link-hover)
 *   colorText 四阶      = 白 #ffffff / #969696 / #8f8f8f / #666666（规格 Text 四级）
 *   colorBgLayout      = --canvas         (#181818)
 *   colorBgContainer   = --canvas-card    (#303030)
 *   colorBgElevated    = --tooltip-bg     (#3c3c3c)
 *   colorBorder        = 强线             (#4a4a4a 推导；规格 hairline #303030 在 #303030 容器上不可见)
 *   colorBorderSecondary/colorSplit = #303030 / #242424
 *   borderRadius 4     = 规格 text-input rounded.sm；borderRadiusLG 0 = 卡/面板锐角
 *   fontFamily         = --font-sans（FerrariSans 授权字体 → Inter 500 规格书面替代栈）
 */
export const ferrariAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // Rosso Corsa 一族：base #da291c → hover 压暗 #9d2211 → 按压 #b01e0a（三值皆规格明文）
    colorPrimary: '#da291c',
    colorPrimaryHover: '#9d2211',
    colorPrimaryActive: '#b01e0a',
    // info = 规格 semantic-info；Alert 钉「蓝薄纱 + 强线」（同 --info-soft）
    colorInfo: '#4c98b9',
    colorInfoBg: 'rgba(76, 152, 185, .14)',
    colorInfoBorder: 'rgba(76, 152, 185, .35)',
    // 语义三色：success 直译；warning 推导琥珀（规格红 #f13a2c 已让位 error）；error = 规格 validation red
    colorSuccess: '#03904a',
    colorWarning: '#e8a13c',
    colorError: '#f13a2c',
    // 白链 + hover 转红：Rosso 稀缺原则，红只在交互瞬间出现（规格 Decorative Depth：CTA hover 用红）
    colorLink: '#ffffff',
    colorLinkHover: '#da291c',
    colorLinkActive: '#b01e0a',
    // 文字四级坍缩：Ink 白 / body #969696 / muted-soft #8f8f8f / muted #666666（规格 Text 原文）
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#969696',
    colorTextTertiary: '#8f8f8f',
    colorTextQuaternary: '#666666',
    // 红底白字（规格 on-primary #ffffff）
    colorTextLightSolid: '#ffffff',
    // 表面阶梯：画布 #181818 / 容器 #303030 / 浮层 #3c3c3c（规格暗灰渐变起点）
    colorBgBase: '#181818',
    colorBgLayout: '#181818',
    colorBgContainer: '#303030',
    colorBgElevated: '#3c3c3c',
    colorBgSpotlight: '#3c3c3c',
    // 边线双阶：控件强线 #4a4a4a（推导中点）/ 规格 hairline #303030；分隔 #242424
    colorBorder: '#4a4a4a',
    colorBorderSecondary: '#303030',
    colorSplit: '#242424',
    // 焦点环 = Hypersail Yellow（规格全局 focus-ring 色，黄色唯二合法场景之一）
    controlOutline: 'rgba(255, 242, 0, .3)',
    // 圆角：输入/表单 4px（规格 rounded.sm）；卡/面板 0px（rounded.none 锐角哲学）；CTA 在 Button 单独钉 0
    borderRadius: 4,
    borderRadiusSM: 2,
    borderRadiusLG: 0,
    // 海拔 = 亮度阶梯为主，阴影只用规格唯一 soft 层（hovered cards 0 4px 8px）
    boxShadow: '0 4px 8px rgba(0, 0, 0, .1)',
    boxShadowSecondary: '0 4px 8px rgba(0, 0, 0, .1)',
    // FerrariSans 为授权字体；规格书面替代 = Inter 500（系统/web-safe 近似栈）
    fontFamily:
      "'FerrariSans', Inter, -apple-system, system-ui, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
    // display 恒 500 不加粗（规格「Display weight stays at 500 — never bold」）
    fontWeightStrong: 500,
  },
  components: {
    Button: {
      // CTA = 锐角 0px（规格 button-primary rounded.none，品牌签名）；标签 700（typography.button）
      borderRadius: 0,
      borderRadiusSM: 0,
      borderRadiusLG: 0,
      fontWeight: 700,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认按钮 = 规格 button-outline-on-dark：透明底 + 1px 白边白字，hover 落到提升面
      defaultBg: 'transparent',
      defaultColor: '#ffffff',
      defaultBorderColor: '#ffffff',
      defaultHoverBg: '#303030',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#ffffff',
    },
    Menu: {
      // 下拉当前项 = 提升面 + 白字（导航激活靠 css 片段的红色指示条表达）
      itemSelectedBg: '#303030',
      itemSelectedColor: '#ffffff',
    },
    Table: {
      // 扁平表头：画布同底 + 灰字 + 发丝线；hover 行半阶提亮；选中行 = 红纱（race-position 语义）
      headerBg: '#181818',
      headerColor: '#8f8f8f',
      headerSplitColor: 'transparent',
      rowHoverBg: '#1f1f1f',
      rowSelectedBg: 'rgba(218, 41, 28, .12)',
      rowSelectedHoverBg: 'rgba(218, 41, 28, .16)',
      fixedHeaderSortActiveBg: '#1f1f1f',
      borderColor: '#3c3c3c',
    },
    Tooltip: { colorBgSpotlight: '#3c3c3c' },
    Modal: { contentBg: '#303030', headerBg: '#303030', footerBg: '#303030' },
    Drawer: { colorBgElevated: '#303030' },
    Popover: { colorBgElevated: '#3c3c3c' },
    // 输入 = 规格 text-input-on-dark：canvas 底 #181818（卡面上形成下沉井）+ 4px 圆角
    Select: { colorBgContainer: '#181818', optionSelectedBg: '#303030' },
    Input: { colorBgContainer: '#181818' },
    InputNumber: { colorBgContainer: '#181818' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#242424', itemSelectedBg: '#3c3c3c' },
    // badge-pill 是唯一药丸几何（规格原文）→ 小号 Tag 钉 999
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
