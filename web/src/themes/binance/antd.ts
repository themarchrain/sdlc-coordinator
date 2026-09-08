import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * binance 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格令牌表（字面 hex，禁 var()）。
 *
 * Binance 设计语言（深近黑金融平台）：
 * - 画布 canvas-dark #0b0e11（「never pure black」）；色块分层：
 *   卡/下拉 #1e2329 → 嵌套/浮层 #2b3139（「borders feel like surface steps」）；
 * - Binance Yellow #FCD535 承载全部主 CTA/品牌强调，黑字黄底
 *   （on-primary #181a20，系统签名组合）；按压 #f0b90b（primary-active）；
 * - 涨绿跌红贯穿：trading-up #0ecb81 / trading-down #f6465d 承担任务态
 *   success/error（骨架必须的语义对；详见 slots.ts 取舍注）；
 * - 文字：on-dark #ffffff（标题）/ body #eaecef（正文「deliberately not
 *   pure white」）/ muted-strong #929aa5 / muted #707a8a 四阶；
 * - 圆角刻度 2/4/6/8/12：控件 6、输入/内容卡 8、提升容器 12、药丸只给
 *   顶部注册类 CTA（登录主按钮，由 css 片段承担）；
 * - 字重克制：标题 600、按钮 600、正文 400（信任字号与黄电压而非粗字重）；
 * - 扁平色块分层：卡面零阴影，浮层仅一层 faint 阴影保可用性
 *   （规格「faint shadow … used sparingly」的浮层版，推导）；
 * - focus = info-ring #3b82f6 50% 纱（规格字面值）。
 * algorithm = darkAlgorithm（照 spotify 暗色范例派生填充/禁用态等中间色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/binance.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#fcd535 Binance Yellow)
 *   colorPrimaryHover/Active = --primary-active 报按压档 (#f0b90b；规格只定义一档 active 变体)
 *   colorInfo          = --info           (#3b82f6 规格 info/ring)
 *   colorSuccess       = --success        (#0ecb81 trading-up)
 *   colorWarning       = --warning        (#f19d27 推导：橙移琥珀，避开品牌黄，见 slots.ts 注)
 *   colorError         = --error          (#f6465d trading-down)
 *   colorLink          = --link           (#fcd535 规格 text-link 黄链)
 *   colorText 四阶      = #eaecef / #929aa5 / #707a8a / #707a8a
 *   colorBgLayout/Container/Elevated = #0b0e11 / #1e2329 / #2b3139（色块分层）
 *   colorBorder 三件    = --hairline      (#2b3139 hairline-on-dark)
 *   borderRadius 6     = 规格 rounded.md（标准 CTA/主输入）
 *   fontFamily         = BinanceNova → Inter 近似（规格替代字体）；Plex 承数字
 */
export const binanceAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // Binance Yellow：唯一品牌色，承载全部主 CTA（Brand & Accent）
    colorPrimary: '#fcd535',
    colorPrimaryHover: '#f0b90b',
    colorPrimaryActive: '#f0b90b',
    // 语义：success/error = 涨绿跌红；warning = 推导橙移琥珀（见 slots.ts 注）
    colorInfo: '#3b82f6',
    colorInfoBg: 'rgba(59, 130, 246, 0.12)',
    colorInfoBorder: 'rgba(59, 130, 246, 0.35)',
    colorSuccess: '#0ecb81',
    colorWarning: '#f19d27',
    colorError: '#f6465d',
    // 黄链（规格 text-link：黄在暗/亮两种画布上都成立）
    colorLink: '#fcd535',
    colorLinkHover: '#f0b90b',
    colorLinkActive: '#f0b90b',
    // 文字阶梯：body #eaecef / muted-strong #929aa5 / muted #707a8a（三级 + on-dark 白由 heading 承担）
    colorText: '#eaecef',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#929aa5',
    colorTextTertiary: '#707a8a',
    colorTextQuaternary: '#707a8a',
    // 签名：黄底黑字（规格「Black text on yellow primary CTAs」，禁反转为白字）
    colorTextLightSolid: '#181a20',
    // 色块分层：画布 #0b0e11 / 容器卡 #1e2329 / 浮层 #2b3139
    colorBgBase: '#0b0e11',
    colorBgLayout: '#0b0e11',
    colorBgContainer: '#1e2329',
    colorBgElevated: '#2b3139',
    colorBgSpotlight: '#2b3139',
    // 边线 = hairline-on-dark #2b3139（与提升面同 hex，「边即表面台阶」）
    colorBorder: '#2b3139',
    colorBorderSecondary: '#2b3139',
    colorSplit: '#2b3139',
    // focus = info-ring 50% 纱（「0 0 0 2px info-ring at 50% alpha」）
    controlOutline: 'rgba(59, 130, 246, 0.5)',
    // 圆角刻度：控件 6（rounded.md）/ 小件 4 / 提升容器 12（rounded.xl）
    borderRadius: 6,
    borderRadiusSM: 4,
    borderRadiusLG: 12,
    // 扁平色块分层：卡面零阴影；浮层留一层 faint 阴影保分离（推导）
    boxShadow: 'none',
    boxShadowSecondary: '0 8px 24px rgba(0, 0, 0, 0.4)',
    // 字体：BinanceNova → Inter 近似（规格替代字体，禁外部加载）；数字族 BinancePlex 由 fontFamilyCode/css 承担
    fontFamily:
      "'BinanceNova', Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode:
      "'BinancePlex', 'JetBrains Mono', 'IBM Plex Sans', 'Cascadia Code', Consolas, monospace",
    // 字重克制：标题 600（规格 title 档 600；display 700 由 css 片段给登录大字）
    fontWeightStrong: 600,
  },
  components: {
    Button: {
      // 标准 CTA 圆角 6（规格 button-primary rounded.md）；药丸只给登录主 CTA（css 片段）
      borderRadius: 6,
      borderRadiusSM: 4,
      borderRadiusLG: 8,
      // 按钮字重 600（规格 typography.button 600）
      fontWeight: 600,
      // 黄底黑字（系统签名组合）
      primaryColor: '#181a20',
      // 默认按钮 = button-secondary-on-dark：卡面色 #1e2329 + 白字（规格字面值）
      defaultBg: '#1e2329',
      defaultColor: '#ffffff',
      defaultBorderColor: '#1e2329',
      defaultHoverBg: '#2b3139',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#2b3139',
      defaultActiveBg: '#2b3139',
      defaultActiveColor: '#ffffff',
      defaultActiveBorderColor: '#2b3139',
      // 卡面扁平：按钮不带投影
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#181a20' },
    Steps: { colorWhite: '#181a20' },
    Menu: {
      // 当前项 = 黄纱 + 黄字（黄 = 主信号；规格「hovered nav items」升一阶的语汇）
      itemSelectedBg: 'rgba(252, 213, 53, 0.08)',
      itemSelectedColor: '#fcd535',
    },
    Table: {
      // 扁平表头：透明底 + muted 列头（规格 muted 角色「table column headers」）+ 发丝行线
      headerBg: 'transparent',
      headerColor: '#707a8a',
      headerSplitColor: 'transparent',
      rowHoverBg: 'rgba(255, 255, 255, 0.04)',
      rowSelectedBg: '#2b3139',
      rowSelectedHoverBg: '#2b3139',
      fixedHeaderSortActiveBg: 'transparent',
      borderColor: '#2b3139',
    },
    Modal: { contentBg: '#1e2329', headerBg: '#1e2329', footerBg: '#1e2329' },
    Drawer: { colorBgElevated: '#1e2329' },
    Tooltip: { colorBgSpotlight: '#2b3139' },
    Popover: { colorBgElevated: '#2b3139' },
    Select: {
      // 输入 = search-input-on-dark：卡面色 #1e2329（规格字面值）
      colorBgContainer: '#1e2329',
      optionSelectedBg: 'rgba(255, 255, 255, 0.06)',
      optionSelectedColor: '#ffffff',
    },
    Input: { colorBgContainer: '#1e2329' },
    InputNumber: { colorBgContainer: '#1e2329' },
    Form: { labelColor: '#929aa5' },
    Segmented: { trackBg: '#1e2329', itemSelectedBg: '#2b3139' },
    Tag: { borderRadiusSM: 4 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
