import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * framer 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Framer 设计语言（暗色画板 / poster 编辑）：
 * - 近纯黑画布 #090909 → surface-1 #141414（卡/次级药丸）→ surface-2 #1c1c1c
 *   （选中/ Featured / 交互面）→ 浮层 #262626；
 * - 主 CTA = 白药丸：#ffffff 底 + #000000 字（规格 primary/on-primary 原文），
 *   按压在实站是 transform 缩放而非压深，此处以极轻变暗近似（hover #f0f0f0 / active #e0e0e0，推导值）；
 * - 唯一彩色信号 Sky Blue #0099ff：只给链接 / focus 环 / 选中指示，绝不做底色
 *   （规格 Do：「never as a background or button fill」）；
 * - 文字二元律：ink #ffffff 或 ink-muted #999999，禁止中间灰（规格 Don't）；
 * - 语义三色仅 success 在规格内（#22c55e），warning/error 无规格对应物 →
 *   取渐变家族的 sunset-orange #ff7a3d / coral #ff5577（调色板 = 单色 + 一蓝 + 渐变族，推导值）；
 * - 字重窄带：display 500 / body 400 / caption 500，层级靠字号 + 负字距（fontWeightStrong=500，全站无 bold）；
 * - 药丸几何：按钮一律 100px 圆角；控件 10px（rounded.md）；卡 20px（rounded.xl）；
 * - 海拔 = 光缘 + 投影双层：inset 0 .5px 0 rgba(255,255,255,.10) + 0 10px 30px rgba(0,0,0,.25)（规格 Level 2 原文）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/framer.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#ffffff 白药丸)
 *   colorPrimaryActive = --primary-active (#e0e0e0 推导压深)
 *   colorInfo/colorLink = --info/--link   (#0099ff accent blue)
 *   colorSuccess       = --success        (#22c55e 规格 checkmark 绿)
 *   colorWarning       = --warning        (#ff7a3d gradient-orange，推导)
 *   colorError         = --error          (#ff5577 gradient-coral，推导)
 *   colorText 四阶      = ink #ffffff / #999999 / #999999 / #666666（禁用档推导）
 *   colorBgLayout      = --canvas         (#090909)
 *   colorBgContainer   = --canvas-card    (#141414 surface-1)
 *   colorBgElevated    = surface-2        (#1c1c1c)
 *   colorBorder        = --hairline       (#262626) / secondary #1a1a1a
 *   boxShadow          = --shadow-panel   (Level 2 光缘 + 投影)
 *   fontFamily         = --font-sans（Inter Variable 近似栈，禁外部字体）
 */
export const framerAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 白药丸是全站唯一主 CTA 形状
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#f0f0f0',
    colorPrimaryActive: '#e0e0e0',
    // 唯一彩色信号：链接 / focus / 选中指示专用（Brand & Accent）
    colorInfo: '#0099ff',
    colorLink: '#0099ff',
    colorLinkHover: '#33adff',
    colorLinkActive: '#0099ff',
    // 语义三色（success 规格；warning/error 自渐变家族推导）
    colorSuccess: '#22c55e',
    colorWarning: '#ff7a3d',
    colorError: '#ff5577',
    // 文字二元律：ink 或 ink-muted，无中间灰（规格 Don't）
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#999999',
    colorTextTertiary: '#999999',
    colorTextQuaternary: '#666666',
    // 白底黑字（primary 药丸 / 实底标签文字一律近黑）
    colorTextLightSolid: '#000000',
    // 表面阶梯：画布 #090909 / 容器 surface-1 #141414 / 浮层 surface-2 #1c1c1c
    colorBgBase: '#090909',
    colorBgLayout: '#090909',
    colorBgContainer: '#141414',
    colorBgElevated: '#1c1c1c',
    colorBgSpotlight: '#262626',
    // 边线双阶：hairline #262626 / hairline-soft #1a1a1a（规格 Surface 原文）
    colorBorder: '#262626',
    colorBorderSecondary: '#1a1a1a',
    colorSplit: '#1a1a1a',
    // Level 3 蓝染环：唯一彩色深度信号（规格 Elevation 表 rgba(0,153,255,0.15)）
    controlOutline: 'rgba(0, 153, 255, .2)',
    // 圆角刻度：控件 10（rounded.md）/ 小件 6（rounded.sm）/ 大面 20（rounded.xl）
    borderRadius: 10,
    borderRadiusSM: 6,
    borderRadiusLG: 20,
    // 海拔 = 光缘 + 投影（规格 Level 2 原文；浮层再加深一档）
    boxShadow: 'inset 0 .5px 0 rgba(255, 255, 255, .1), 0 10px 30px rgba(0, 0, 0, .25)',
    boxShadowSecondary: 'inset 0 .5px 0 rgba(255, 255, 255, .1), 0 16px 40px rgba(0, 0, 0, .4)',
    // Inter Variable 近似 —— 系统/web-safe 栈（GT Walsheim 为授权字体，禁外部加载）
    fontFamily:
      'Inter, system-ui, -apple-system, "Segoe UI", "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
    // 字重窄带：层级靠字号 + 负字距，全站无 bold
    fontWeightStrong: 500,
  },
  components: {
    Button: {
      // 药丸是品牌交互形状：全部按钮 100px（规格 rounded.pill）
      borderRadius: 100,
      borderRadiusSM: 100,
      borderRadiusLG: 100,
      // 按钮字 14/500/-0.14px（规格 typography.button；字距在 framer.css 片段钉住）
      fontWeight: 500,
      primaryColor: '#000000',
      // 默认按钮 = 炭灰药丸：surface-1 底白字，hover 提亮一阶（selected = lift，规格 Pricing Tabs）
      defaultBg: '#141414',
      defaultColor: '#ffffff',
      defaultBorderColor: '#141414',
      defaultHoverBg: '#1c1c1c',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#1c1c1c',
      // 按钮不带投影（海拔留给浮层卡片）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#000000' },
    Steps: { colorWhite: '#000000' },
    Table: {
      // 扁平对比表：表头与容器同底 + ink-muted 标签 + 近隐分隔（规格 comparison-row）
      headerBg: '#141414',
      headerColor: '#999999',
      headerSplitColor: 'transparent',
      rowHoverBg: '#1c1c1c',
      rowSelectedBg: '#1c1c1c',
      rowSelectedHoverBg: '#242424',
      fixedHeaderSortActiveBg: '#1c1c1c',
      borderColor: '#1a1a1a',
    },
    Modal: { contentBg: '#1c1c1c', headerBg: '#1c1c1c', footerBg: '#1c1c1c' },
    Drawer: { colorBgElevated: '#141414' },
    Tooltip: { colorBgSpotlight: '#262626' },
    Popover: { colorBgElevated: '#1c1c1c' },
    // 选中 = 提升（surface-2），不是颜色（规格 pricing-tab-selected）
    Select: { colorBgContainer: '#1c1c1c', optionSelectedBg: '#1c1c1c' },
    Input: { colorBgContainer: '#1c1c1c' },
    InputNumber: { colorBgContainer: '#1c1c1c' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#141414', itemSelectedBg: '#1c1c1c' },
    Tag: { borderRadiusSM: 100 },
    Pagination: { itemActiveBg: 'transparent' },
    Menu: { itemSelectedBg: '#1c1c1c', itemSelectedColor: '#ffffff' },
  },
}
