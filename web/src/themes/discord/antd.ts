import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * discord 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Discord 设计语言（暗色，深靛蓝画布）：
 * - 主操作 = Blurple（规格 colors.primary #5865f2）：主 CTA / 频段 / stat 卡；
 *   hover 提亮 #6d78f5、active 压暗 #4752c4（blurple-deep，Discord 官方暗阶）。
 *   渐变（blurple→magenta）是品牌核心但 antd 不吃渐变 —— 主色给字面 hex，
 *   渐变在 themes/css/discord.css 作用域片段里表达（登录 hero CTA）。
 * - info = 规格 link cyan #00b0f4（饱和强调色，非中性 → 不钉 infoBg/infoBorder，
 *   由 darkAlgorithm 派生青调暗面）；
 * - 语义色：success = 规格 green #35ed7e（语义槽位是状态色，非「通用点缀」）；
 *   warning/error 规格未给 → 从 Discord 产品状态语言推导（idle 黄 #f0b132 / dnd 红 #f23f43）；
 * - 画布阶：canvas #0a0d3a / soft #141847 / card #1e2353（规格 surface-indigo）/
 *   elevated #262b62（推导抬升阶）；tooltip = 规格 surface-black #000000；
 * - 文字阶坍缩为三阶：ink #ffffff / body #d4d8ef（靛调白）/ mute #9ba1cf（靛调灰）；
 * - 发丝线：#2a2f68（抬升靛蓝主线）/ 规格 hairline #23272a（onyx 安静阶）/ #3a3f7d 强调阶；
 * - 圆角按规格按钮刻度：controls rounded.sm 12px / 小钮 rounded.xs 6px / 大钮 rounded.lg 16px，
 *   Tag 走 rounded.pill 50px（「pill caps, badges」）；
 * - 字重 800（ABC Ginto Nord 重展示字声部）；无投影（规格「depth 来自色/渐变/圆角」）；
 * - 字体 gg sans → 'Helvetica Neue'/Arial/system-ui web-safe 近似（禁外部字体）。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/discord.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#5865f2)
 *   colorPrimaryHover  = blurple 提亮一阶  (#6d78f5)
 *   colorPrimaryActive = --blurple-deep   (#4752c4)
 *   colorInfo          = --info           (#00b0f4 = 规格 link)
 *   colorSuccess       = --success        (#35ed7e = 规格 green)
 *   colorWarning       = --warning        (#f0b132)
 *   colorError         = --error          (#f23f43)
 *   colorLink          = --link           (#00b0f4 = 规格 link)
 *   colorText          = --body           (#d4d8ef)
 *   colorTextHeading   = --ink            (#ffffff)
 *   colorBgLayout      = --canvas         (#0a0d3a)
 *   colorBgContainer   = --canvas-card    (#1e2353)
 *   colorBgElevated    = elevated 阶      (#262b62)
 *   colorBorder        = --hairline       (#2a2f68)
 *   colorBorderSecondary / colorSplit = 规格 hairline (#23272a)
 *   fontFamily         = --font-sans；fontFamilyCode = --font-mono
 */
export const discordAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // Blurple 是系统主操作色
    colorPrimary: '#5865f2',
    colorPrimaryHover: '#6d78f5',
    colorPrimaryActive: '#4752c4',
    // info = 规格 link cyan（饱和强调色，darkAlgorithm 派生青调暗面）
    colorInfo: '#00b0f4',
    // 语义三色：success = 规格 green；warning/error 从产品状态语言推导
    colorSuccess: '#35ed7e',
    colorWarning: '#f0b132',
    colorError: '#f23f43',
    // 文字链接 = 规格 link cyan（暗面上的行内链接色）
    colorLink: '#00b0f4',
    colorLinkHover: '#4cc3f7',
    colorLinkActive: '#0090c8',
    // 文字阶梯：ink / body / mute 三阶（靛调派生）
    colorText: '#d4d8ef',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#d4d8ef',
    colorTextTertiary: '#9ba1cf',
    colorTextQuaternary: '#9ba1cf',
    // 画布阶：深靛蓝页面 / surface-indigo 容器 / 抬升靛蓝浮层
    colorBgBase: '#0a0d3a',
    colorBgLayout: '#0a0d3a',
    colorBgContainer: '#1e2353',
    colorBgElevated: '#262b62',
    colorBgSpotlight: '#262b62',
    // 发丝线：#2a2f68 主线 / 规格 hairline #23272a 安静阶
    colorBorder: '#2a2f68',
    colorBorderSecondary: '#23272a',
    colorSplit: '#23272a',
    controlOutline: 'rgba(88, 101, 242, 0.35)',
    // 圆角按规格按钮刻度：sm 12 / xs 6 / lg 16（「Nothing is sharp」）
    borderRadius: 12,
    borderRadiusSM: 6,
    borderRadiusLG: 16,
    // 重展示字（ABC Ginto Nord 700-800）压轻正文（ggsans 400-500）
    fontWeightStrong: 800,
    // gg sans → web-safe 近似（禁外部字体）
    fontFamily: '"Helvetica Neue", Helvetica, Arial, system-ui, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
  },
  components: {
    Button: {
      // 规格 button 刻度：primary rounded.sm 12 / ghost-sm xs 6 / white-ghost lg 16
      borderRadius: 12,
      borderRadiusSM: 6,
      borderRadiusLG: 16,
      // button-ghost：surface-indigo 底 + 白字（规格 ghost 按钮）
      defaultBorderColor: 'transparent',
      defaultColor: '#ffffff',
      defaultBg: '#1e2353',
      // button-primary：blurple 底 + on-primary 白字
      primaryColor: '#ffffff',
      // typography.link/link-lg 字重 500
      fontWeight: 500,
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#ffffff' },
    Switch: { handleBg: '#ffffff', handleShadow: 'none' },
    Steps: { colorWhite: '#ffffff' },
    Table: {
      headerBg: '#141847',
      headerColor: '#9ba1cf',
      headerSplitColor: 'transparent',
      headerBorderRadius: 12,
      rowHoverBg: '#262b62',
      rowSelectedBg: 'rgba(88, 101, 242, 0.16)',
      rowSelectedHoverBg: 'rgba(88, 101, 242, 0.24)',
      fixedHeaderSortActiveBg: '#141847',
      borderColor: '#2a2f68',
    },
    Modal: { contentBg: '#1e2353', headerBg: '#1e2353', footerBg: '#1e2353' },
    Drawer: { colorBgElevated: '#1e2353' },
    Tooltip: { colorBgSpotlight: '#000000' },
    Popover: { colorBgElevated: '#262b62' },
    Select: { colorBgContainer: '#141847', optionSelectedBg: 'rgba(88, 101, 242, 0.28)' },
    Input: { colorBgContainer: '#141847' },
    InputNumber: { colorBgContainer: '#141847' },
    Form: { labelColor: '#d4d8ef' },
    Segmented: { trackBg: '#141847', itemSelectedBg: '#1e2353', itemSelectedColor: '#ffffff' },
    // 规格 rounded.pill 50px：「pill caps, badges, avatar chips」
    Tag: { borderRadius: 50, borderRadiusSM: 50 },
    Pagination: { itemActiveBg: 'transparent' },
    Menu: { itemSelectedBg: 'rgba(88, 101, 242, 0.16)', itemSelectedColor: '#ffffff' },
  },
}
