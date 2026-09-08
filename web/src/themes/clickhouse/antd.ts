import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * clickhouse 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * ClickHouse 设计语言（近纯黑 + 电光黄）：
 * - 画布阶 #0a0a0a → 软带 #121212 → 卡 #1a1a1a → 嵌套卡 #242424（卡仅比画布微亮）；
 * - 品牌黄一族：CTA 基色 #faff69、按压/悬停加深 #e6eb52（规格只定义这一个 darker
 *   变体，hover 与 active 同值）、禁用 #3a3a1f；
 * - 签名：黄底黑字（colorTextLightSolid #0a0a0a，规格 on-primary 黑字于黄）；
 * - 黄的纪律：黄只落在主 CTA / 统计数值 / 黄卡带，默认控件一律暗阶
 *   （#1a1a1a 底白字），语义色 emerald/rose/blue 独立于黄（规格「不引入第二品牌色」）；
 * - 无投影：规格 Elevation 明言系统不用阴影，深度全靠明度分层（两级 boxShadow 全关）；
 * - 圆角刻度：小件 6 / 按钮·输入 8 / 内容卡 12，药丸只给徽章（Tag 999）；
 * - 字体：Inter 一族承担全部层级（display 700），码字 JetBrains Mono；
 *   系统栈近似（规格 fallback：-apple-system/BlinkMacSystemFont/Segoe UI/Roboto）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/clickhouse.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#faff69 electric yellow)
 *   colorPrimaryHover/Active = --link-hover/--primary-active (#e6eb52 规格唯一 darker 变体)
 *   colorInfo          = --info           (#3b82f6 accent blue)
 *   colorSuccess       = --success        (#22c55e accent emerald)
 *   colorWarning       = --warning        (#f59e0b)
 *   colorError         = --error          (#ef4444 accent rose)
 *   colorLink          = --link           (#faff69 规格 text-link：暗底行内链 = 黄 + 下划线)
 *   colorText 四阶      = #ffffff / #cccccc / #888888 / #5a5a5a（规格 Text 五级坍缩）
 *   colorBgLayout      = --canvas         (#0a0a0a)
 *   colorBgContainer   = --canvas-card    (#1a1a1a)
 *   colorBgElevated    = --tooltip-bg     (#242424)
 *   colorBorder 双阶 + colorSplit = #3a3a3a / #2a2a2a（规格 hairline-strong / hairline）
 *   borderRadius 8/6/12 = 规格按钮 md / 小件 sm / 内容卡 lg
 *   boxShadow 双阶全关  = 规格「no drop shadows」
 *   fontFamily          = --font-sans；fontFamilyCode = --font-mono
 */
export const clickhouseAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 电光黄一族：base #faff69 → 按压/悬停加深 #e6eb52（规格只此一个 darker 变体）
    colorPrimary: '#faff69',
    colorPrimaryHover: '#e6eb52',
    colorPrimaryActive: '#e6eb52',
    // 语义/强调三色（emerald / rose / blue，独立于品牌黄）
    colorInfo: '#3b82f6',
    colorSuccess: '#22c55e',
    colorWarning: '#f59e0b',
    colorError: '#ef4444',
    // 行内链 = 黄（规格 text-link：yellow on dark + 下划线）
    colorLink: '#faff69',
    colorLinkHover: '#e6eb52',
    colorLinkActive: '#e6eb52',
    // 文字阶梯：白标题 / #cccccc 正文 / #888888 muted / #5a5a5a 细则（规格 Text）
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#cccccc',
    colorTextTertiary: '#888888',
    colorTextQuaternary: '#5a5a5a',
    // 签名：黄底黑字（主按钮/实底标签上的文字一律近黑）
    colorTextLightSolid: '#0a0a0a',
    // 表面阶梯：画布 #0a0a0a / 容器·卡 #1a1a1a / 嵌套·浮层 #242424
    colorBgBase: '#0a0a0a',
    colorBgLayout: '#0a0a0a',
    colorBgContainer: '#1a1a1a',
    colorBgElevated: '#242424',
    colorBgSpotlight: '#242424',
    // 边线双轨：hairline-strong #3a3a3a（输入下划线/强调）/ hairline #2a2a2a（卡 1px 边）
    colorBorder: '#3a3a3a',
    colorBorderSecondary: '#2a2a2a',
    colorSplit: '#2a2a2a',
    // 输入聚焦 = 黄加粗（规格 text-input-focused：border thickens to primary）
    controlOutline: 'rgba(250, 255, 105, .35)',
    // 圆角刻度：按钮·输入 8 / 小件 6 / 内容卡 12（规格 rounded.md/sm/lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 海拔 = 无投影（规格 Elevation：系统不用阴影，靠明度分层）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Inter 一族的系统栈近似（规格 fallback 栈 + CJK 兜底，禁外部加载）
    fontFamily:
      'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"JetBrains Mono", ui-monospace, Consolas, monospace',
    // display 一律 700（规格「Display weights stay at 700 across all sizes」）
    fontWeightStrong: 700,
  },
  components: {
    Button: {
      // 按钮字重 600（规格 button 14px/600）；三态阴影全关
      fontWeight: 600,
      primaryColor: '#0a0a0a',
      // 默认按钮 = 暗面卡按钮（规格 button-secondary：surface-card 底 + 白字）
      defaultBg: '#1a1a1a',
      defaultColor: '#ffffff',
      defaultBorderColor: '#1a1a1a',
      // hover 提亮一阶到嵌套卡 #242424（规格未定义 hover，按明度阶梯推导）
      defaultHoverBg: '#242424',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#242424',
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    // 黄底上的选中标记一律黑（黄底黑字签名）
    Checkbox: { colorWhite: '#0a0a0a' },
    Steps: { colorWhite: '#0a0a0a' },
    Table: {
      // 扁平表头：与容器同底 #1a1a1a + muted 标签 + 透明分隔缝；行 hover 提亮到 #242424
      headerBg: '#1a1a1a',
      headerColor: '#888888',
      headerSplitColor: 'transparent',
      rowHoverBg: '#242424',
      rowSelectedBg: '#242424',
      rowSelectedHoverBg: '#242424',
      fixedHeaderSortActiveBg: '#242424',
      borderColor: '#2a2a2a',
    },
    Modal: { contentBg: '#1a1a1a', headerBg: '#1a1a1a', footerBg: '#1a1a1a' },
    Drawer: { colorBgElevated: '#1a1a1a' },
    Tooltip: { colorBgSpotlight: '#242424' },
    Popover: { colorBgElevated: '#242424' },
    Select: { colorBgContainer: '#1a1a1a', optionSelectedBg: '#242424' },
    Input: { colorBgContainer: '#1a1a1a' },
    InputNumber: { colorBgContainer: '#1a1a1a' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#121212', itemSelectedBg: '#242424' },
    // 药丸只给徽章（规格「No pill except in tag badges」）
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
