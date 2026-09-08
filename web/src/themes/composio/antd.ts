import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * composio 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Composio 设计语言（近黑画布 + 深电蓝电压）：
 * - 亮度阶梯海拔：canvas #0f0f0f → 卡 #181818 → elevated/终端面板 #222222 →
 *   下拉/浮层 surface-strong #2a2a2a → 终端网格/代码凹陷 canvas-deep #000000；
 *   海拔不靠投影，靠亮度步进（浮层阴影仅为近黑底上的功能性分离，推导值）；
 * - 品牌电压一族：primary #0007cd（CTA/wordmark）→ hover #1a26ff（spotlight glow
 *   提亮）→ 按压 #0005a3；蓝保持稀缺，青 #00d4ff / 紫 #7b3aed 仅限数据可视化插图；
 * - 文字阶梯：ink #ffffff / body #a8a8a8 / mute #888888 / muted-soft #666666；
 * - 深蓝 #0007cd 在近黑底上对比不足 → 文字级交互（链接/info）用提亮蓝
 *   #4d79ff（推导值，primary-glow 同色相的文字变体）；
 * - 语义色：success #33d17a / error #ff4d4d（规格）；warning 规格未给 →
 *   推导暗底琥珀 #f5b83d；
 * - 圆角刻度：CTA/输入 8px（rounded.md「not full pills」的 developer 方言）、
 *   卡 12px、面板/终端网格 16px、徽章药丸 9999；
 * - 字体：abcDiatype 为授权字体 → 规格 Note 指定开源替身 Inter（500 + 负字距）；
 *   代码面 JetBrains Mono。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/composio.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#0007cd Composio Blue)
 *   colorPrimaryHover  = #1a26ff          (--primary-glow 规格提亮态)
 *   colorPrimaryActive = --primary-active (#0005a3 规格按压态)
 *   colorInfo/colorLink = --info/--link   (#4d79ff 推导文字级提亮蓝)
 *   colorSuccess       = --success        (#33d17a 规格)
 *   colorWarning       = --warning        (#f5b83d 推导：规格未给 warning)
 *   colorError         = --error          (#ff4d4d 规格)
 *   colorText 四阶      = ink/body/mute/muted-soft (#ffffff/#a8a8a8/#888888/#666666)
 *   colorBgLayout      = --canvas         (#0f0f0f)
 *   colorBgContainer   = --canvas-card    (#181818 surface-card)
 *   colorBgElevated    = surface-strong   (#2a2a2a，规格「Dropdown menus」)
 *   colorBorder 三阶    = #333/#222/#1a1a1a（规格 hairline-strong/hairline/hairline-soft）
 *   borderRadius 8     = 规格 rounded.md「every CTA — not full pills」
 *   borderRadiusLG 12  = 规格 rounded.lg（toolkit 卡/代码块）
 *   fontFamily         = --font-sans（Inter = abcDiatype 开源替身）
 *   fontFamilyCode     = --font-mono（JetBrains Mono）
 */
export const composioAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // Composio Blue 一族：#0007cd → hover 提亮到 spotlight glow #1a26ff → 按压回落 #0005a3
    colorPrimary: '#0007cd',
    colorPrimaryHover: '#1a26ff',
    colorPrimaryActive: '#0005a3',
    // 语义色：success/error 规格；warning 推导暗底琥珀
    colorInfo: '#4d79ff',
    colorSuccess: '#33d17a',
    colorWarning: '#f5b83d',
    colorError: '#ff4d4d',
    // 链接 = 提亮蓝（深蓝 #0007cd 在近黑底上对比不足，推导文字变体）
    colorLink: '#4d79ff',
    colorLinkHover: '#6b93ff',
    colorLinkActive: '#4d79ff',
    // 文字阶梯：白 / 软灰 #a8a8a8 / muted #888888 / muted-soft #666666
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#a8a8a8',
    colorTextTertiary: '#888888',
    colorTextQuaternary: '#666666',
    // 蓝底白字（规格 on-primary #ffffff）
    colorTextLightSolid: '#ffffff',
    // 亮度阶梯：画布 #0f0f0f / 卡 #181818 / 浮层 surface-strong #2a2a2a
    colorBgBase: '#0f0f0f',
    colorBgLayout: '#0f0f0f',
    colorBgContainer: '#181818',
    colorBgElevated: '#2a2a2a',
    colorBgSpotlight: '#2a2a2a',
    // 边线三阶：hairline-strong #333（控件边）/ hairline #222 / hairline-soft #1a1a1a
    colorBorder: '#333333',
    colorBorderSecondary: '#222222',
    colorSplit: '#1a1a1a',
    controlOutline: 'rgba(26, 38, 255, .45)',
    // 圆角刻度：CTA/输入 8 / 小件 6 / 卡 12（药丸只留给 Tag 徽章）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 海拔 = 亮度步进（规格「avoid drop shadows」）；近黑底上仅留功能性浮层分离影（推导值）
    boxShadow: '0 6px 24px rgba(0, 0, 0, .45)',
    boxShadowSecondary: '0 12px 40px rgba(0, 0, 0, .55)',
    // abcDiatype 为授权字体 → 规格 Note 指定 Inter 替身；代码面 JetBrains Mono
    fontFamily:
      "Inter, ui-sans-serif, system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: "'JetBrains Mono', 'Fira Code', ui-monospace, 'SFMono-Regular', Menlo, Consolas, monospace",
    // 规格排版：display 500 / title 600（500 的负字距展示由 composio.css 片段承担）
    fontWeightStrong: 600,
  },
  components: {
    Button: {
      // CTA 14/500（规格 typography.button）；无投影（海拔靠亮度步进）
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 次级 = button-secondary-dark：#222222 elevated 底白字，hover 上浮 surface-strong
      defaultBg: '#222222',
      defaultColor: '#ffffff',
      defaultBorderColor: '#222222',
      defaultHoverBg: '#2a2a2a',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#2a2a2a',
    },
    Table: {
      // 扁平表头：与卡同底 #181818 + muted 标签；hover 行亮度上浮一阶
      headerBg: '#181818',
      headerColor: '#888888',
      headerSplitColor: 'transparent',
      rowHoverBg: '#202020',
      rowSelectedBg: '#222222',
      rowSelectedHoverBg: '#2a2a2a',
      fixedHeaderSortActiveBg: '#202020',
      borderColor: '#1a1a1a',
    },
    Modal: { contentBg: '#222222', headerBg: '#222222', footerBg: '#222222' },
    Drawer: { colorBgElevated: '#181818' },
    Tooltip: { colorBgSpotlight: '#2a2a2a' },
    Popover: { colorBgElevated: '#2a2a2a' },
    Select: { colorBgContainer: '#181818', optionSelectedBg: '#333333' },
    Input: { colorBgContainer: '#181818' },
    InputNumber: { colorBgContainer: '#181818' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#0f0f0f', itemSelectedBg: '#2a2a2a' },
    Tag: { borderRadiusSM: 999 },
    Menu: {
      // 下拉当前项 = 蓝电压薄纱 + 白（规格「Composio Blue stays scarce」的激活场景）
      itemSelectedBg: 'rgba(26, 38, 255, .28)',
      itemSelectedColor: '#ffffff',
    },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
