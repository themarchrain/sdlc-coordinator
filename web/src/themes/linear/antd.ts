import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * linear 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文或其声明的产品色板（字面 hex，禁 var()）。
 *
 * Linear 设计语言（近黑软件工艺文档）：
 * - 画布 #010102 = 全场最深暗面（近纯黑 + 一丝蓝调）；四级表面阶梯
 *   surface-1 #0f1011 → surface-2 #141516 → surface-3 #18191a → surface-4 #191a1b；
 * - 淡薰衣草蓝一族：基 #5e6ad2、hover 提亮 #828fff、按压/focus #5e69d1（白字 on-primary）；
 * - 语义色：营销面唯一直给 success #27a644；error/warning/info 取规格 Known Gaps 声明的
 *   Linear 产品色标签（红 #eb5757 / 橙 #f2994a / 蓝 #4ea7fc）——管理台是产品面而非营销面；
 * - 圆角刻度：按钮/输入 8px（rounded.md）、小件 6px、卡 12px、状态徽章 pill 9999
 *   （规格 Don't：CTA 不做药丸）；
 * - 字重纪律：显示 600 / 正文 400 / 按钮 500，抗 700+（规格「resists 700+ display weights」）；
 * - 海拔不用投影：表面阶梯 + 发丝线 + 顶部白缘高光（inset 0 1px 0 rgba(255,255,255,.04)）；
 * - 字体：Linear Display/Text 为私有族，按规格 fallback（SF Pro Display/-apple-system）
 *   + 推荐替代 Inter 组系统栈；mono 用推荐替代 JetBrains Mono。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/linear.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#5e6ad2 淡薰衣草蓝，唯一彩色)
 *   colorPrimaryHover  = --link-hover     (#828fff 提亮)
 *   colorPrimaryActive = --primary-active (#5e69d1 = 规格 primary-focus，按压态)
 *   colorInfo          = --info           (#4ea7fc Linear 产品蓝，推导)
 *   colorSuccess       = --success        (#27a644 规格唯一营销语义色)
 *   colorWarning       = --warning        (#f2994a Linear 产品橙，推导)
 *   colorError         = --error          (#eb5757 Linear 产品红，推导)
 *   colorLink          = --link           (#5e6ad2 链接强调 = 薰衣草合法用法)
 *   colorText 四阶      = ink #f7f8f8 / #d0d6e0 / #8a8f98 / #62666d（规格文字四级）
 *   colorBgLayout      = --canvas         (#010102)
 *   colorBgContainer   = --canvas-card    (#0f1011 surface-1)
 *   colorBgElevated    = --tooltip-bg     (#191a1b surface-4)
 *   colorBorder        = hairline-strong  (#34343a 控件边) / secondary #23252a（卡发丝线）
 *   controlOutline     = --focus-ring     (rgba(94,105,209,.5) = 2px primary-focus @50%)
 *   boxShadow          = 顶部白缘高光     (规格「subtle white edge highlight」替代投影)
 *   fontFamily         = --font-sans；fontFamilyCode = --font-mono
 */
export const linearAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 淡薰衣草蓝一族：base #5e6ad2 → hover 提亮 #828fff → 按压 #5e69d1（规格 primary-hover/focus）
    colorPrimary: '#5e6ad2',
    colorPrimaryHover: '#828fff',
    colorPrimaryActive: '#5e69d1',
    // 语义色（success = 规格营销唯一语义；其余 = 规格 Known Gaps 的产品色标签，推导）
    colorInfo: '#4ea7fc',
    colorSuccess: '#27a644',
    colorWarning: '#f2994a',
    colorError: '#eb5757',
    // 链接强调 = 薰衣草（规格 Do：「brand mark, primary CTA, focus ring, link emphasis」）
    colorLink: '#5e6ad2',
    colorLinkHover: '#828fff',
    colorLinkActive: '#5e69d1',
    // 文字阶梯 = 规格四级原文：ink / ink-muted / ink-subtle / ink-tertiary
    colorText: '#f7f8f8',
    colorTextHeading: '#f7f8f8',
    colorTextSecondary: '#d0d6e0',
    colorTextTertiary: '#8a8f98',
    colorTextQuaternary: '#62666d',
    // 薰衣草底白字（规格 on-primary；与 spotify 绿底黑字相反的签名）
    colorTextLightSolid: '#ffffff',
    // 表面阶梯：画布 #010102 / 容器 surface-1 #0f1011 / 浮层 surface-4 #191a1b
    colorBgBase: '#010102',
    colorBgLayout: '#010102',
    colorBgContainer: '#0f1011',
    colorBgElevated: '#191a1b',
    colorBgSpotlight: '#191a1b',
    // 边框双阶：hairline-strong #34343a（控件）/ hairline #23252a（卡/分隔）
    colorBorder: '#34343a',
    colorBorderSecondary: '#23252a',
    colorSplit: '#23252a',
    // 规格 Level 4：2px primary-focus @50% 的 focus ring
    controlOutline: 'rgba(94, 105, 209, .5)',
    // 圆角刻度：按钮/输入 8（rounded.md）/ 小件 6 / 卡 12（rounded.lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 海拔不用投影：浮卡顶缘白高光 + 浮层黑纱分离（规格 semantic-overlay 纯黑）
    boxShadow: 'inset 0 1px 0 rgba(255, 255, 255, .05)',
    boxShadowSecondary: '0 16px 48px rgba(0, 0, 0, .55)',
    // 字体：Linear 私有族 → 规格 fallback + 推荐替代 Inter；mono → JetBrains Mono
    fontFamily:
      "'Inter', 'SF Pro Display', -apple-system, system-ui, 'Segoe UI', Roboto, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: "'JetBrains Mono', ui-monospace, 'SF Mono', Menlo, Consolas, monospace",
    // 显示字重 600（规格「Pair display weight 600 with body weight 400 — resists 700+」）
    fontWeightStrong: 600,
  },
  components: {
    Button: {
      // 按钮 8px 圆角 + 14px/500（规格 button typography + rounded.md；Don't 药丸）
      borderRadius: 8,
      borderRadiusSM: 8,
      borderRadiusLG: 8,
      fontWeight: 500,
      // 默认按钮 = 规格 button-secondary：surface-1 炭底 + 浅灰墨 + 1px 发丝线，hover 提升 surface-2
      defaultBg: '#0f1011',
      defaultColor: '#f7f8f8',
      defaultBorderColor: '#23252a',
      defaultHoverBg: '#141516',
      defaultHoverColor: '#f7f8f8',
      defaultHoverBorderColor: '#34343a',
      // 无投影（海拔交给表面阶梯与发丝线）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 当前项 = surface-2 提升 + 浅灰墨（规格「selected = surface lift」）
      itemSelectedBg: '#141516',
      itemSelectedColor: '#f7f8f8',
    },
    Table: {
      // 扁平表头：与容器同底 surface-1 + ink-subtle 银字；行 hover 提升 surface-2
      headerBg: '#0f1011',
      headerColor: '#8a8f98',
      headerSplitColor: 'transparent',
      rowHoverBg: '#141516',
      rowSelectedBg: '#141516',
      rowSelectedHoverBg: '#18191a',
      fixedHeaderSortActiveBg: '#141516',
      borderColor: '#23252a',
    },
    Modal: { contentBg: '#191a1b', headerBg: '#191a1b', footerBg: '#191a1b' },
    Drawer: { colorBgElevated: '#0f1011' },
    Tooltip: { colorBgSpotlight: '#191a1b' },
    Popover: { colorBgElevated: '#18191a' }, // 下拉菜单 = surface-3（规格 Elevation Level 3）
    Select: { colorBgContainer: '#0f1011', optionSelectedBg: '#141516' },
    Input: {
      // 输入 = 规格 text-input：surface-1 底，静止边 hairline-strong，focus 环走 controlOutline
      colorBgContainer: '#0f1011',
      hoverBorderColor: '#3e3e44',
      activeBorderColor: '#5e69d1',
    },
    InputNumber: { colorBgContainer: '#0f1011' },
    Form: { labelColor: '#d0d6e0' },
    Segmented: {
      // 规格 pricing-tab：canvas 轨 + surface-2 选中（selected = surface lift）
      trackBg: '#0f1011',
      itemSelectedBg: '#141516',
      itemSelectedColor: '#f7f8f8',
    },
    Tag: { borderRadiusSM: 999 }, // status-badge = 规格 pill
    Pagination: { itemActiveBg: 'transparent' },
  },
}
