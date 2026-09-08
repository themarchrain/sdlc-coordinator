import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * spotify 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Spotify 设计语言（近黑茧房）：
 * - 画布阶 #121212 → 面板/卡 #181818 → 交互面 #1f1f1f → 浮层卡 #252525/#272727；
 * - 品牌绿一族：CTA 基色 #1db954、hover 提亮 #1ed760、按压回落 #169c46；
 * - 签名约束一：绿底黑字（colorTextLightSolid #121212，播放键 #1ed760 底 #000000 图标的按钮版）；
 * - 签名约束二：暗底上彩色只在 hover/激活出现 —— 默认控件一律暗阶（#1f1f1f 底白字），
 *   绿只落在主 CTA / 激活态 / focus；
 * - 语义色：error #f3727f、warning #ffa42b、info(announcement blue) #539df5、success 复用品牌绿；
 * - 药丸几何：全部 Button 999px，输入/小件 4px，卡 6px，面板 8px；
 * - 字体：CircularSp 无外部字体，Circular → system-ui/'Helvetica Neue'/Arial 近似栈；
 * - 海拔用重投影：卡片 0 8px 8px rgba(0,0,0,.3)、浮层 0 8px 24px rgba(0,0,0,.5)。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 xai 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/spotify.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#1db954)
 *   colorPrimaryHover  = --link-hover     (#1ed760)
 *   colorPrimaryActive = --primary-active (#169c46)
 *   colorInfo          = --info           (#539df5 announcement blue)
 *   colorSuccess       = --success        (#1ed760 品牌绿即成功色)
 *   colorWarning       = --warning        (#ffa42b)
 *   colorError         = --error          (#f3727f)
 *   colorLink          = --link           (#ffffff 白链，hover 转绿)
 *   colorText 四阶      = ink #ffffff / cbcbcb / b3b3b3 / b3b3b3
 *   colorBgLayout      = --canvas         (#121212)
 *   colorBgContainer   = --canvas-card    (#181818)
 *   colorBgElevated    = --tooltip-bg     (#272727)
 *   colorBorder        = Border Gray      (#4d4d4d) / colorBorderSecondary #2a2a2a
 *   boxShadow          = --shadow-panel   (Medium 0 8px 8px rgba(0,0,0,.3))
 *   boxShadowSecondary = Dialog 阴影      (Heavy 0 8px 24px rgba(0,0,0,.5))
 *   fontFamily         = --font-sans；fontFamilyCode = --font-mono
 */
export const spotifyAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 品牌绿一族：base #1db954 → hover 提亮 #1ed760 → 按压回落 #169c46
    colorPrimary: '#1db954',
    colorPrimaryHover: '#1ed760',
    colorPrimaryActive: '#169c46',
    // 语义色（Semantic：negative red / warning orange / announcement blue）
    colorInfo: '#539df5',
    colorSuccess: '#1ed760',
    colorWarning: '#ffa42b',
    colorError: '#f3727f',
    // 白链 + hover 转绿：暗底上彩色只在 hover/激活出现
    colorLink: '#ffffff',
    colorLinkHover: '#1ed760',
    colorLinkActive: '#1ed760',
    // 文字阶梯坍缩为白/近白/银三阶（Text）
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#cbcbcb',
    colorTextTertiary: '#b3b3b3',
    colorTextQuaternary: '#b3b3b3',
    // 签名：绿底黑字（主按钮/实底标签上的文字一律近黑）
    colorTextLightSolid: '#121212',
    // 表面阶梯：画布 #121212 / 容器 #181818 / 浮层 #272727
    colorBgBase: '#121212',
    colorBgLayout: '#121212',
    colorBgContainer: '#181818',
    colorBgElevated: '#272727',
    colorBgSpotlight: '#272727',
    // 边框双阶：Border Gray #4d4d4d / 内部分隔 #2a2a2a、#242424
    colorBorder: '#4d4d4d',
    colorBorderSecondary: '#2a2a2a',
    colorSplit: '#242424',
    controlOutline: 'rgba(29, 185, 84, .35)',
    // 圆角刻度：小件 4px / 卡 6px / 面板 8px（药丸在 Button/Tag 组件上单独钉 999）
    borderRadius: 6,
    borderRadiusSM: 4,
    borderRadiusLG: 8,
    // 海拔用重投影（暗底上薄影不可见）
    boxShadow: '0 8px 8px rgba(0, 0, 0, .3)',
    boxShadowSecondary: '0 8px 24px rgba(0, 0, 0, .5)',
    // 字体：CircularSp 近似 —— 系统/web-safe 栈，无外部字体
    fontFamily:
      "'Circular', system-ui, -apple-system, 'Segoe UI', 'Helvetica Neue', Helvetica, Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
    // 粗字重排版：antd 无标题字重全局令牌，标题/数值 700 由 spotify.css 专属片段承担
    fontWeightStrong: 700,
  },
  components: {
    Button: {
      // 药丸是品牌交互形状：全部按钮 999px
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 粗字重按钮（700）+ 绿底黑字的主 CTA
      fontWeight: 700,
      primaryColor: '#121212',
      // 默认按钮 = Dark Pill：#1f1f1f 底白字，hover 轻微提亮（无第二彩相）
      defaultBg: '#1f1f1f',
      defaultColor: '#ffffff',
      defaultBorderColor: '#1f1f1f',
      defaultHoverBg: '#2a2a2a',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#2a2a2a',
      // 按钮不带投影（海拔留给浮层）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#121212' },
    Steps: { colorWhite: '#121212' },
    Table: {
      // 扁平表头：与容器同底 #181818 + 银字（Spotify 曲目列表式），hover 行提亮一阶
      headerBg: '#181818',
      headerColor: '#b3b3b3',
      headerSplitColor: 'transparent',
      rowHoverBg: '#1f1f1f',
      rowSelectedBg: '#1f1f1f',
      rowSelectedHoverBg: '#252525',
      fixedHeaderSortActiveBg: '#1f1f1f',
      borderColor: '#242424',
    },
    Modal: { contentBg: '#252525', headerBg: '#252525', footerBg: '#252525' },
    Drawer: { colorBgElevated: '#181818' },
    Tooltip: { colorBgSpotlight: '#272727' },
    Popover: { colorBgElevated: '#272727' },
    Select: { colorBgContainer: '#1f1f1f', optionSelectedBg: '#2a2a2a' },
    Input: { colorBgContainer: '#1f1f1f' },
    InputNumber: { colorBgContainer: '#1f1f1f' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#1f1f1f', itemSelectedBg: '#2a2a2a' },
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
