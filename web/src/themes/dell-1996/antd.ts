import type { ThemeConfig } from 'antd'

/**
 * dell-1996 主题的 antd ThemeConfig —— 规格驱动，无参考实现。
 *
 * 时代语言迁移：1996 目录式网页 → 现代 antd 组件，按角色（用途对用途）映射：
 *   - 规格 button-primary（黑底白字、Helvetica Bold 全大写、方角）→ antd 主按钮：
 *     colorPrimary = 帧墨 #000000（CSS 片段补 uppercase/700）
 *   - Dell red #e91d2a 规格仅许 CTA 面板与电话号独享 → 现代 admin 里「最高注意级」
 *     = 危险/报错，故 red 落位 colorError/--error/--nav-indicator（导航当前项的
 *     3px 指示条 = 规格 ex-app-shell-row activeIndicator:{colors.primary}）
 *   - 规格 button-text-link（Mosaic 蓝 #0000ee 下划线锚点）→ colorLink
 *   - 规格 text-input（白底 1px 纯黑边）→ colorBorder = 纯黑（卡片黑框同源）
 *   - 规格 rounded.none 全场方角 → borderRadius 三档全 0
 *   - 规格「无软投影，海拔只靠发丝线」→ boxShadow 两级全关
 *   - 双字体层级：UI 控件 Helvetica（规格 ui-label/button 家族），正文 Times
 *     由 CSS --font-sans 承载（:root 默认字体），展示数字 Arial Black 走 CSS
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/dell-1996.css 的同名令牌手工保持同源：
 *   colorPrimary          = --primary       (#000000 帧墨 = button-primary 填色)
 *   colorPrimaryActive    = --primary-active(#333333 抖动灰压深)
 *   colorSuccess          = 语义绿          (#008000，slots.timeline.success 同源)
 *   colorWarning          = 暖深黄          (#8a6a00 = yellow-sticker 压深，slots 同源)
 *   colorError            = --error         (#e91d2a Dell red)
 *   colorInfo             = 帧墨（中性）    (#000000)
 *   colorInfoBg/Border    = 主题软面+发丝线 (#e4e8da pale sage / #b3bd95 sage 缎带线)
 *   colorLink             = --link          (#0000ee 经典 Mosaic 蓝)
 *   colorText 四阶         = ink/body/mute 灰阶 (#000000/#333333/#4d4d4d/#666666)
 *   colorBorder 双阶       = --hairline/--hairline-soft (#000000/#b3b3b3)
 *   colorBgLayout/Container = --canvas/--canvas-card (#ffffff 帧内纯白)
 *   borderRadius 0/0/0    = rounded.none
 *   fontFamily            = Helvetica UI 栈（--font-sans 承载正文 Times，正交分工）
 *   fontFamilyCode        = --font-mono（Courier New 打字机 mono）
 */
export const dell1996ThemeConfig: ThemeConfig = {
  token: {
    // 主操作 = 规格 button-primary：黑底白字（Dell red 另派 danger，勿混）
    colorPrimary: '#000000',
    colorPrimaryActive: '#333333',
    // info/processing 收敛为帧墨（中性）——colorInfo 为墨色时 antd 会派生灰蒙
    // Alert 底压黑字（对比度不足），按 cursor 先例钉住「主题软面 + 缎带发丝线」
    colorInfo: '#000000',
    colorInfoBg: '#e4e8da',
    colorInfoBorder: '#b3bd95',
    // 链接 = 经典 Mosaic/Netscape 3.x 蓝，恒色（时代无 hover 变色，CSS 补下划线）
    colorLink: '#0000ee',
    colorLinkHover: '#0000ee',
    colorLinkActive: '#0000ee',
    // 语义色：规格未给 success/warning，按时代 HTML 基色推导（绿 #008000 /
    // 深黄 = yellow-sticker #fcc20f 压深至可读）；error = Dell red
    colorSuccess: '#008000',
    colorWarning: '#8a6a00',
    colorError: '#e91d2a',
    // 文字阶梯：纯黑墨 + 抖动灰阶（GIF 时代 51 级灰）
    colorText: '#000000',
    colorTextSecondary: '#333333',
    colorTextTertiary: '#4d4d4d',
    colorTextQuaternary: '#666666',
    // 边框双阶：控件黑框（text-input 1px 黑边）+ 内部银灰细线
    colorBorder: '#000000',
    colorBorderSecondary: '#cccccc',
    // 填充：黑 alpha（hover/选中洗）
    colorFill: 'rgba(0, 0, 0, 0.12)',
    colorFillSecondary: 'rgba(0, 0, 0, 0.08)',
    colorFillTertiary: 'rgba(0, 0, 0, 0.05)',
    colorFillQuaternary: 'rgba(0, 0, 0, 0.03)',
    // 表面：帧内纯白画布 + 纯白卡片（层级全靠黑发丝线）
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 全场方角（rounded.none）；海拔只靠发丝线，两级阴影全关
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 0,
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // UI 控件 Helvetica；正文 Times / 展示 Arial Black 在 CSS 令牌层分工
    fontFamily: 'Helvetica, Arial, "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Courier New", Courier, monospace',
  },
  components: {
    Button: {
      // 规格 button/button-secondary：Helvetica 700 全大写、无投影（大写在 CSS 片段）
      fontWeight: 700,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Input: { activeShadow: 'none' },
    InputNumber: { activeShadow: 'none' },
    Select: { optionSelectedBg: '#e4e8da' },
    Segmented: {
      trackBg: '#e4e8da',
      itemSelectedBg: '#ffffff',
    },
    Table: {
      // 表头 = pale sage 色块条 + 黑字；行 hover = peach 缎带洗；表格纯黑框线
      headerBg: '#e4e8da',
      headerColor: '#000000',
      headerSplitColor: '#000000',
      rowHoverBg: 'rgba(230, 145, 93, 0.18)',
      borderColor: '#000000',
    },
    Tooltip: { colorBgSpotlight: '#000000' },
  },
}
