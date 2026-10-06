import type { ThemeConfig } from 'antd'

/**
 * mobbin 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Mobbin 设计语言 = 画廊白单色系：近黑墨 #141414 在纯白画布上，结构由
 * 「极浅中性染阶」承担（soft #f3f3f3 / field #f0f0f0 / hairline #e0e0e0），
 * 零阴影、零渐变、零彩色 chrome；交互语汇 = stadium 药丸（按钮/徽标 9999px
 * 由 CSS 片段承担，输入走 rounded.sm 16px）；卡片几何 24px；标题 Saans 652
 * （替代映射 650）/ 正文 456。唯一彩色 Electric Blue #0066FF 规格只给
 * 「商业信号」（badge-popular），管理台语境下迁移为 info/success/link 的
 * 承担色（先例 vercel：交互蓝一身三职）；error/warning 为派生克制语义色
 * （明言「不设语义色板」→ 浏览器默认语义的克制化，先例 tesla/notion）。
 * algorithm 未指定 = antd 默认算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/mobbin.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#141414 Ink Black，品牌色即近黑墨；主 CTA = 墨底白字药丸)
 *   colorPrimaryHover/Active = 墨阶推导         (#262626 Ink-soft 微提亮 / #000000 压向纯黑)
 *   colorInfo/colorSuccess/colorLink = --info/--success/--link
 *                                                (#0066FF，accent = 唯一正向/信息信号)
 *   colorLinkHover          = --link-hover      (#0052CC 派生：蓝压深一阶)
 *   colorWarning/colorError = 派生克制语义      (#A16207 / #C8102E)
 *   colorText 四阶          = Ink/Ink-soft/Muted/Faint (#141414/#262626/#707070/#ADADAD，Text 原文四级)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 画廊白)
 *   colorBorder 双阶 + colorSplit = Hairline/Hairline-soft (#E0E0E0/#F0F0F0，规格 边线双色)
 *   borderRadius 16         = 规格 rounded.sm（输入/瓷砖/小件 16px）
 *   borderRadiusLG 24       = 规格 rounded.md（内容卡 24px；按钮药丸 9999px 由 CSS 片段钉住）
 *   fontWeightStrong 650    = Saans 652 标题的替代映射（规格 字体替代注记 652 → 650）
 *   fontFamily              = --font-sans（Saans 商业字体禁外部加载 → 规格替代栈 Inter + 系统 sans）
 */
export const mobbinAntdTheme: ThemeConfig = {
  token: {
    // Ink Black：品牌色即近黑墨 —— 主 CTA 是墨底白字药丸（「The brand color」）
    colorPrimary: '#141414',
    colorPrimaryHover: '#262626', // Ink-soft：hover 沿墨阶微提亮（规格 未记 hover，推导）
    colorPrimaryActive: '#000000', // 派生：按压压向纯黑（规格 未记 active，推导）
    // Electric Blue：系统唯一彩色 —— 规格定位「sole positive-emphasis signal」，
    // 迁移为 info/success/link 三职承担（先例 vercel #0070f3 同法）
    colorInfo: '#0066FF',
    colorSuccess: '#0066FF',
    colorLink: '#0066FF',
    colorLinkHover: '#0052CC', // 派生：蓝压深一阶
    // 语义两色派生：规格 不设语义色板（状态留在单色阶）→ 浏览器默认语义的克制化
    colorWarning: '#A16207',
    colorError: '#C8102E',
    // 文字四阶：Ink / Ink-soft / Muted / Faint（Text 四级原文）
    colorText: '#141414',
    colorTextSecondary: '#262626',
    colorTextTertiary: '#707070',
    colorTextQuaternary: '#ADADAD',
    // 画廊白：页面/容器/浮层全白，靠染阶与发丝线分层（「Default page and card background」）
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：Hairline（16% 控件边）/ Hairline-soft（白上白卡的最浅缘）
    colorBorder: '#E0E0E0',
    colorBorderSecondary: '#F0F0F0',
    colorSplit: '#F0F0F0',
    // 圆角：输入/小件 = rounded.sm 16；大面卡 = rounded.md 24（药丸由 CSS 片段承担）
    borderRadius: 16,
    borderRadiusSM: 16,
    borderRadiusLG: 24,
    // 标题 652 的替代映射（规格 字体替代注记：652 → 650）
    fontWeightStrong: 650,
    // 零阴影体系（「essentially shadow-free」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Saans 商业字体禁外部加载 → 规格 fallback 栈 + Inter 替代（首位保留 Saans 供本地持有者）
    fontFamily:
      'Saans, Inter, -apple-system, "Segoe UI", "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(20, 20, 20, 0.08)' },
    Button: {
      // 按钮标签 = link 语汇（16px/600）；三态阴影全关（药丸圆角由 CSS 片段钉 9999px）
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = soft 染阶 + 墨字（规格 nav-pill / segmented 语汇：染阶即选中态）
      itemSelectedBg: '#F3F3F3',
      itemSelectedColor: '#141414',
    },
    Table: {
      // ex-data-table-cell：表头 soft 面 + Muted 标签 + 透明分隔缝；行 hover = soft 染阶
      headerBg: '#F3F3F3',
      headerColor: '#707070',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F3F3F3',
      borderColor: '#E0E0E0',
    },
    Tooltip: {
      // 反转面：墨底白字（「Inverse」层 = ink 填充 + on-primary 字）
      colorBgSpotlight: '#141414',
    },
  },
}
