import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * warp 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Warp 设计语言（暖棕终端暗画布）：
 * - 画布 = 暖近炭 #2b2622（比纯黑更棕，Rust 终端血统），卡/嵌面 = canvas-soft #383330，
 *   发丝线 #3f3a36；海拔靠「面阶对比 + 发丝线」，营销面无软投影；
 * - 品牌主色是暖白 #f7f5f0（off-white）—— 同时充当默认文字与 button-primary 填充，
 *   无彩色品牌强调（签名约束）；实底主按钮 = 暖白底 + 暖深字（on-primary #2b2622）；
 * - 文字阶梯坍缩为 暖白 #f7f5f0 / body-strong #dad2c1 / body #c9c0ad / mute #aea69c 四级；
 * - 语义色：规格明确「营销面无独立语义色板」→ 按暖调低饱和推导（见各值行内注释）；
 * - 极紧圆角：按钮/输入 3px（rounded.sm）、卡 4px（rounded.md）、大卡 6px（rounded.lg）；
 * - 字重纪律：只有 400/500（fontWeightStrong 压到 500，display 400 的克制由 warp.css 片段承担）；
 * - 字体：Inter 叙事 + DM Mono 代码（规格正典配对，系统栈近似，禁外部加载）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/warp.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#f7f5f0 暖白 CTA，规格「off-white IS the brand」)
 *   colorPrimaryHover  = 提亮一阶         (#ffffff，暖白唯一更亮方向)
 *   colorPrimaryActive = --primary-active (#e6e1d8 按压压深，推导值)
 *   colorInfo          = --info           (#94a7bd 推导：无彩色约束下的低饱和石板蓝)
 *   colorSuccess/Warning/Error = 语义三色（推导：规格无语义色板 → 暖调低饱和终端色）
 *   colorLink          = --link           (#f7f5f0 暖白链，ink 即 primary 的规格原文)
 *   colorText 四阶      = 暖白 / body / mute / mute (#f7f5f0/#c9c0ad/#aea69c/#aea69c)
 *   colorBgLayout      = --canvas         (#2b2622)
 *   colorBgContainer   = --canvas-card    (#383330 card-content)
 *   colorBgElevated    = --canvas-soft    (#383330；浮层同卡阶，靠发丝线+投影分离)
 *   colorBorder        = --hairline       (#3f3a36)
 *   borderRadius 3     = 规格 rounded.sm「default button radius — extremely tight」
 *   borderRadiusLG 6   = 规格 rounded.lg（大卡/面板）
 *   fontFamily         = --font-sans（Inter）；fontFamilyCode = --font-mono（DM Mono）
 */
export const warpAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 暖白一族：CTA 基 #f7f5f0 → hover 提亮到纯白 → 按压压深（推导值）
    colorPrimary: '#f7f5f0',
    colorPrimaryHover: '#ffffff',
    colorPrimaryActive: '#e6e1d8',
    // 语义三色（推导：规格「营销面回避语义色板」→ 暖调低饱和、暗底可读的终端语义色）
    colorInfo: '#94a7bd',
    colorSuccess: '#8fbf7f',
    colorWarning: '#d9a253',
    colorError: '#e07a5f',
    // 链接 = 暖白（规格：off-white 同时是默认文字与交互信号），hover 提亮
    colorLink: '#f7f5f0',
    colorLinkHover: '#ffffff',
    colorLinkActive: '#e6e1d8',
    // 文字阶梯：ink #f7f5f0 / body #c9c0ad / mute #aea69c（Text 四级坍缩）
    colorText: '#f7f5f0',
    colorTextHeading: '#f7f5f0',
    colorTextSecondary: '#c9c0ad',
    colorTextTertiary: '#aea69c',
    colorTextQuaternary: '#aea69c',
    // 签名：暖白实底上的文字 = 暖深（on-primary #2b2622，极性翻转）
    colorTextLightSolid: '#2b2622',
    // 表面阶梯：画布 #2b2622 / 容器卡 #383330 / 浮层同卡阶
    colorBgBase: '#2b2622',
    colorBgLayout: '#2b2622',
    colorBgContainer: '#383330',
    colorBgElevated: '#383330',
    colorBgSpotlight: '#3f3a36',
    // 边线双阶：发丝线 #3f3a36 / 内部分隔 #37322e（近隐）
    colorBorder: '#3f3a36',
    colorBorderSecondary: '#37322e',
    colorSplit: '#37322e',
    controlOutline: 'rgba(247, 245, 240, .3)',
    // 圆角刻度：按钮/输入 3px（rounded.sm 极紧）；卡 4px 走 CSS 令牌；大面 6px
    borderRadius: 3,
    borderRadiusSM: 2,
    borderRadiusLG: 6,
    // 海拔 = 发丝线 + 面阶（「soft drop-shadows do not appear」）；
    // 浮层仅留一道功能性分离影（ex-modal「elevated shadow」推导）
    boxShadow: 'none',
    boxShadowSecondary: '0 8px 24px rgba(0, 0, 0, .4)',
    // 字体：Inter + DM Mono 规格正典配对（系统栈近似，禁外部加载）
    fontFamily:
      "Inter, system-ui, -apple-system, 'Segoe UI', Roboto, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: "'DM Mono', ui-monospace, 'SFMono-Regular', Menlo, Consolas, monospace",
    // 字重纪律：只有 400/500（「Weights 400/500 are the working pair」）
    fontWeightStrong: 500,
  },
  components: {
    Button: {
      // 按钮字重 500（规格 button-md 14/500）；三态阴影全关（无软投影）
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // ghost 次级 = 画布底暖白字（规格 button-secondary-ghost），hover 上浮一阶到卡面
      defaultBg: '#2b2622',
      defaultColor: '#f7f5f0',
      defaultBorderColor: '#3f3a36',
      defaultHoverBg: '#383330',
      defaultHoverColor: '#f7f5f0',
      defaultHoverBorderColor: '#3f3a36',
    },
    Checkbox: { colorWhite: '#2b2622' },
    Steps: { colorWhite: '#2b2622' },
    Table: {
      // 表头 = canvas-soft 扁平面 + mute 标签（ex-data-table headerBackground）；hover 行上浮一阶
      headerBg: '#383330',
      headerColor: '#aea69c',
      headerSplitColor: 'transparent',
      rowHoverBg: '#3e3934',
      rowSelectedBg: '#3e3934',
      rowSelectedHoverBg: '#453f3a',
      fixedHeaderSortActiveBg: '#3e3934',
      borderColor: '#37322e',
    },
    Modal: { contentBg: '#383330', headerBg: '#383330', footerBg: '#383330' },
    Drawer: { colorBgElevated: '#383330' },
    Tooltip: { colorBgSpotlight: '#3f3a36' },
    Popover: { colorBgElevated: '#383330' },
    Select: { colorBgContainer: '#383330', optionSelectedBg: '#3e3934' },
    Input: { colorBgContainer: '#383330' },
    InputNumber: { colorBgContainer: '#383330' },
    Form: { labelColor: '#f7f5f0' },
    Segmented: { trackBg: '#2b2622', itemSelectedBg: '#3e3934' },
    Tag: { borderRadiusSM: 2 },
    Menu: {
      // 下拉当前项 = 面阶上浮 + 暖白（无彩色高亮）
      itemSelectedBg: '#3e3934',
      itemSelectedColor: '#f7f5f0',
    },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
