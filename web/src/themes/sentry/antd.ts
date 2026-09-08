import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * sentry 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Sentry 设计语言（紫罗兰午夜 + electric lime）：
 * - 表面阶梯：midnight 画布 #1F1633（surface-canvas-dark）→ night 卡 #150F23
 *   （surface-night：暗画布上的卡 / code block）→ 最深 #120D20（推导：json 凹陷
 *   与浮层，保持规格「code 比宿面深一档」的语义）；卡带 hairline-violet #362D59 边；
 * - primary 取舍（重要）：规格令牌表 primary = #150F23（黑紫），但它同时是最深表面
 *   —— 在 #1F1633 暗画布上作 colorPrimary 只有 1.09:1，按钮面不可见。规格自己给出
 *   解法：「single primary CTA … filled white with dark type on dark surfaces」
 *   （button-inverted）。故 colorPrimary = #ffffff、colorTextLightSolid = #1F1633
 *   （ink-deep），按压回落 #F0F0F0/#EFEFEF（规格 press 令牌）；#150F23 保留在
 *   表面阶梯（canvas-card / 浮层）继续当「brand voice」；
 * - 点缀纪律：electric lime #C2EF4E 只作高亮 chip / 状态 / 进度，绝不作按钮底
 *   （规格 Don't「never use it as a button background」）；hot pink #FA7FAA 承担
 *   error 标点；violet #6A5FC1 承担 info / 链接强调；规格未定义语义色，
 *   warning 取 lime 与 pink 之间的琥珀 #E8B93E（推导值，见 slots.ts 注）；
 * - 焦点环 = ring-focus #9DC1F5（「the only blue in the system」）；
 * - 按钮语汇：button-cap 14px/700 大写 + 0.2px 字距（大写由 sentry.css 片段承担）、
 *   8px 圆角（rounded.md）、primary 带 level-3 暗紫 glow（规格 Elevation 签名）；
 * - 字体：Rubik 全 UI + Monaco 代码（规格两族 + 系统回退，禁外部加载）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/sentry.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#FFFFFF 暗极性主操作 = button-inverted)
 *   colorPrimaryHover  = --login-primary-hover 档 (#F0F0F0 surface-press-light)
 *   colorPrimaryActive = --primary-active (#EFEFEF surface-press-stronger)
 *   colorInfo          = --info           (#6A5FC1 accent-violet)
 *   colorSuccess       = --success        (#C2EF4E accent-lime，正向高亮即成功)
 *   colorWarning       = --warning        (#E8B93E 推导琥珀)
 *   colorError         = --error          (#FA7FAA accent-pink)
 *   colorLink          = --link           (#FFFFFF 规格 link-on-dark：下划线即全部可供性)
 *   colorText 四阶      = 白 / on-dark-muted #BDB8C0 / 推导 #9A93AD / violet-mid #79628C
 *   colorBgLayout      = --canvas         (#1F1633)
 *   colorBgContainer   = --canvas-card    (#150F23 night)
 *   colorBgElevated    = --canvas-card    (#150F23) / Spotlight = --tooltip-bg (#120D20)
 *   colorBorder        = hairline-violet  (#362D59) / Secondary #2C2447（推导）
 *   borderRadius 8     = 规格 rounded.md（primary/inverted 按钮、code block）
 *   fontFamily         = --font-sans（Rubik）；fontFamilyCode = --font-mono（Monaco）
 */
export const sentryAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 主操作 = 白底墨紫字（规格 button-inverted：暗画布上的 single primary CTA）
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#f0f0f0',
    colorPrimaryActive: '#efefef',
    // 语义色（只定义了 focus ring，以下为族内推导，见文件头注）
    colorInfo: '#6a5fc1',
    colorSuccess: '#c2ef4e',
    colorWarning: '#e8b93e',
    colorError: '#fa7faa',
    // 链接 = 白 + 持续下划线（规格 link-on-dark：「the underline is the entire affordance」）
    colorLink: '#ffffff',
    colorLinkHover: '#ffffff',
    colorLinkActive: '#bdb8c0',
    // 文字阶梯：on-primary 白 / on-dark-muted 薰衣草灰 / 推导 / violet-mid
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#bdb8c0',
    colorTextTertiary: '#9a93ad', // 推导：on-dark-muted 与 violet-mid 之间
    colorTextQuaternary: '#79628c',
    // 白按钮上的文字 = ink-deep（规格 button-inverted textColor）
    colorTextLightSolid: '#1f1633',
    // 表面阶梯：midnight 画布 / night 容器 / 最深浮层
    colorBgBase: '#1f1633',
    colorBgLayout: '#1f1633',
    colorBgContainer: '#150f23',
    colorBgElevated: '#150f23',
    colorBgSpotlight: '#120d20',
    // 边线双阶：hairline-violet（规格暗卡边）/ 内部分隔（推导中间紫）
    colorBorder: '#362d59',
    colorBorderSecondary: '#2c2447',
    colorSplit: '#2a2144',
    controlOutline: 'rgba(157, 193, 245, .45)',
    // 圆角刻度：输入 6 / 按钮·代码 8 / 卡 12（规格 rounded.sm / md / xl）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 海拔 = 无投影（规格 Don't：「no drop shadows on cards on dark canvas」），
    // primary 的 level-3 暗紫 glow 在 Button.primaryShadow 单独给
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // 字体：Rubik UI + Monaco 代码（规格两族 + 系统/web-safe 回退）
    fontFamily:
      'Rubik, -apple-system, system-ui, "Segoe UI", Helvetica, Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: 'Monaco, Menlo, "Ubuntu Mono", Consolas, monospace',
    // UI 强调 = 600（规格 body-strong / heading-sm；display 的 chunky 由 css 片段承担）
    fontWeightStrong: 600,
  },
  components: {
    Button: {
      // 按钮 8px（规格 rounded.md：primary/inverted 按钮）
      borderRadius: 8,
      borderRadiusSM: 8,
      borderRadiusLG: 8,
      // button-cap：14px/700（大写 + 0.2px 字距由 sentry.css 片段承担）
      fontWeight: 700,
      primaryColor: '#1f1633',
      // level-3 签名光晕：暗紫自身成为阴影，在按钮四周压出画布晕影
      primaryShadow: 'rgb(21, 15, 35) 0 0 8px 6px',
      // 默认按钮 = ghost-on-dark：on-dark-faint 半透实底 + 白字（规格令牌）
      defaultBg: '#3f3849',
      defaultColor: '#ffffff',
      defaultBorderColor: '#3f3849',
      defaultHoverBg: '#4c455e',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#4c455e',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#1f1633' },
    Steps: { colorWhite: '#1f1633' },
    Table: {
      // 扁平表头：night 同底 + on-dark-muted 标签，hover 行提亮一档（推导紫阶）
      headerBg: '#150f23',
      headerColor: '#bdb8c0',
      headerSplitColor: 'transparent',
      rowHoverBg: '#231b3a',
      rowSelectedBg: '#251e3e',
      rowSelectedHoverBg: '#2a2144',
      fixedHeaderSortActiveBg: '#231b3a',
      borderColor: '#2c2447',
    },
    Modal: { contentBg: '#150f23', headerBg: '#150f23', footerBg: '#150f23' },
    Drawer: { colorBgElevated: '#150f23' },
    Tooltip: { colorBgSpotlight: '#120d20' },
    Popover: { colorBgElevated: '#150f23' },
    Select: {
      colorBgContainer: '#1f1633',
      optionSelectedBg: '#422082', // 规格 select-violet：下拉选中 = deep violet 面
    },
    Input: { colorBgContainer: '#1f1633' },
    InputNumber: { colorBgContainer: '#1f1633' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#1f1633', itemSelectedBg: '#3f3849' },
    Tag: { borderRadiusSM: 4 }, // 规格 pill-neutral-dark = rounded.xs 4px
    Pagination: { itemActiveBg: 'transparent' },
  },
}
