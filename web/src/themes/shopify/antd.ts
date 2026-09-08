import type { ThemeConfig } from 'antd'

/**
 * shopify 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文/色板（字面 hex，禁 var()）。
 *
 * Shopifi 双轨设计语言，管理台（dashboards）取事务轨（light track）：
 * - 乳白事务轨：#fbfbf5 canvas-cream 页底 + #ffffff canvas-light 卡面，
 *   aloe #c1fbd4 / pistachio #d4f9e0 绿只作成功/增长面，不进交互色；
 * - 交互色即墨：primary = #000000 黑色实底药丸（button-primary-pill），
 *   按压态 = shade-70 #3f3f46（规格唯一命名的状态变体，hover 复用同值）；
 * - shade 阶（zinc）承担文字/边线：#3f3f46/#52525b/#71717a/#a1a1aa/#d4d4d8，
 *   hairline-light #e4e4e7 为唯一亮面 1px 边线；
 * - 近黑轨只作局部取材面：tooltip = canvas-night #000000 黑底白字。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/shopify.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#000000 黑药丸 CTA)
 *   colorPrimaryHover/Active = --primary-active (#3f3f46 shade-70 pressed)
 *   colorInfo/colorLink     = --info/--link     (#3f3f46/#000000 墨系：规格无第二交互色相)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline (#f4f4f5/#e4e4e7 锌灰薄纱+发丝线)
 *   colorSuccess            = --success         (#17824f 深化 aloe 绿，推导值)
 *   colorWarning/colorError = --warning/--error (#96660a/#b3372e 推导：标准语义克制化)
 *   colorText 四阶          = ink/shade-70/shade-60/shade-50
 *                             (#000000/#3f3f46/#52525b/#71717a)
 *   colorBgLayout           = --canvas          (#fbfbf5 事务轨 pricing 页底)
 *   colorBgContainer/Elevated = --canvas-card   (#ffffff)
 *   colorBorder             = --hairline        (#e4e4e7 hairline-light)
 *   boxShadow               = --shadow-panel    (Level 3 四层小影纸晕，规格字面值)
 *   boxShadowSecondary      = Level 4 浮层影    (0 25px 50px -12px rgba(0,0,0,.25))
 *   borderRadius 8          = 规格 rounded.md（输入框/小卡）
 *   borderRadiusLG 12       = 规格 rounded.lg（定价/特性卡）
 *   fontFamily              = --font-sans（Inter Variable 正文）
 *   fontFamilyCode          = --font-mono（规格 code 档 ui-monospace 栈）
 */
export const shopifyAntdTheme: ThemeConfig = {
  token: {
    // 黑药丸 CTA：事务轨唯一主操作（规格「button-primary-pill」）
    colorPrimary: '#000000',
    colorPrimaryHover: '#3f3f46', // shade-70（规格命名的 pressed 态；hover 无独立值，复用）
    colorPrimaryActive: '#3f3f46',
    // 墨系 info/link：规格亮轨无第二色相，「黑字 + 黑药丸」即交互语言
    colorInfo: '#3f3f46',
    colorLink: '#000000',
    colorLinkHover: '#3f3f46',
    colorLinkActive: '#3f3f46',
    // info Alert = 锌灰薄纱软面 + hairline-light 发丝线（与 --info-soft/--hairline 同源）
    colorInfoBg: '#f4f4f5',
    colorInfoBorder: '#e4e4e7',
    // 语义三色（规格无语义色码 → 推导：success 取 aloe 绿深化到文字级；warn/error 取标准语义克制化）
    colorSuccess: '#17824f',
    colorWarning: '#96660a',
    colorError: '#b3372e',
    // 文字阶梯：ink #000000 + shade-70/60/50（规格 Shade Ladder 亮面文字四级）
    colorText: '#000000',
    colorTextHeading: '#000000',
    colorTextSecondary: '#3f3f46',
    colorTextTertiary: '#52525b',
    colorTextQuaternary: '#71717a',
    // 事务轨三面：cream 页底 + 白容器 + 白浮层
    colorBgLayout: '#fbfbf5',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 边线：hairline-light 为亮面唯一 1px 边
    colorBorder: '#e4e4e7',
    colorBorderSecondary: '#ececef', // 推导：hairline-light 提亮半档（内部分隔）
    colorSplit: '#ececef',
    // 焦点环墨系（黑即交互色）
    controlOutline: 'rgba(0, 0, 0, 0.1)',
    // 圆角：rounded.md 8（输入）/ rounded.lg 12（卡）
    borderRadius: 8,
    borderRadiusSM: 4, // rounded.xs 4（hairline tags/inputs）
    borderRadiusLG: 12,
    // Level 3 签名「四层小影纸晕」+ Level 4 浮层（字面值）
    boxShadow: '0 8px 8px rgba(0, 0, 0, 0.1), 0 4px 4px rgba(0, 0, 0, 0.1), 0 2px 2px rgba(0, 0, 0, 0.1), 0 0 0 1px rgba(0, 0, 0, 0.1)',
    boxShadowSecondary: '0 25px 50px -12px rgba(0, 0, 0, 0.25)',
    // Inter Variable 正文（规格 UI 档；CJK 兜底）
    fontFamily:
      "'Inter Variable', Inter, system-ui, -apple-system, 'Segoe UI', Helvetica, Arial, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
    // body-strong 550（规格 Inter 420–550 精细字重带）
    fontWeightStrong: 550,
  },
  components: {
    Button: {
      // 药丸是唯一按钮形状（规格「The pill shape is non-negotiable」）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 按钮标签 = body-md 16px/420（规格 pill 标签字重）
      fontWeight: 420,
      // 黑药丸平坦无影（海拔留给卡面纸晕）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = button-outline-on-light：白底 + 1px 墨边 + 墨字
      defaultBg: '#ffffff',
      defaultColor: '#000000',
      defaultBorderColor: '#000000',
      defaultHoverBg: '#fbfbf5',
      defaultHoverColor: '#000000',
      defaultHoverBorderColor: '#000000',
    },
    Menu: {
      // 下拉当前项 = cream 微高亮 + 墨字（同 --primary-veil 语汇）
      itemSelectedBg: '#fbfbf5',
      itemSelectedColor: '#000000',
    },
    Table: {
      // 白底表头 + shade-60 标签 + hairline-light 分隔；行 hover = cream（规格 表格分隔线）
      headerBg: '#ffffff',
      headerColor: '#52525b',
      headerSplitColor: '#e4e4e7',
      rowHoverBg: '#fbfbf5',
      borderColor: '#e4e4e7',
    },
    Tooltip: {
      // 近黑轨取材面：黑底白字 tooltip（canvas-night）
      colorBgSpotlight: '#000000',
    },
    Segmented: {
      trackBg: '#f4f4f5',
      itemSelectedBg: '#ffffff',
    },
    Select: {
      optionSelectedBg: '#fbfbf5',
    },
    Tag: {
      // pill-tag 系列 = 药丸标签（pill-tag-mint / pill-tag-shade）
      borderRadiusSM: 999,
    },
  },
}
