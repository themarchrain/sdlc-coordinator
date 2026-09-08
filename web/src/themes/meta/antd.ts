import type { ThemeConfig } from 'antd'

/**
 * meta 主题的 antd ThemeConfig —— 自设计语言 派生
 * （frontmatter 令牌表 + 正文角色句）。
 *
 * Meta 硬件商务设计语言，管理台取「产品内」轨：stark white #FFFFFF 画布 +
 * Soft Cloud #F1F4F7 内嵌面；双轨主按钮里的产品内购买操作 = 钴蓝
 * #0064E0（button-buy-cta，规格 Do「in-product purchase actions」——管理台
 * 属产品内表面，黑药丸 button-primary 只属营销页），按压 = Deep Cobalt
 * #0457CB（pressed 字面值）；描边次 CTA（button-secondary 2px 墨边）由
 * 默认按钮钉住。全药丸按钮（rounded.full 100px，规格 Don't「buttons are
 * NEVER squared」）、24-32px 大圆角卡、平坦海拔（Level 2 粘性面板影只作
 * 浮层）、Optimistic VF 展示字（ss01+ss02 见 css 片段）、负字距 -0.14~-0.16px、
 * 500/300/400 三档字重节奏。algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/meta.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#0064E0 钴蓝 buy-cta)
 *   colorPrimaryHover       = #0091FF           (推导：Soft Cobalt 提亮档作 hover)
 *   colorPrimaryActive      = --primary-active  (#0457CB Deep Cobalt pressed 字面值)
 *   colorInfo/colorLink     = --info/--link     (#0064E0/#0064E0 钴蓝即产品内交互色)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline
 *                                                (#D9F0FE Soft Cobalt 15% 展开实底 + #CED0D4 Hairline Gray)
 *   colorSuccess/Warning/Error = 语义三色（success #31A24C 规格字面值；
 *                                warning = attention #F2A918 深化到文字级；error = critical #E41E3F 字面值）
 *   colorText 四阶           = Deep Ink #0A1317 / Ink #1C1E21 / Charcoal #444950 / Stone #8595A4
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 三面全白)
 *   colorBorder 双阶 + colorSplit = Hairline Gray #CED0D4 / Hairline Soft #DEE3E9
 *   borderRadius 8          = 规格 rounded.lg（text-input 8px）
 *   Button borderRadius 100 = 规格 rounded.full 100px 全药丸
 *   borderRadiusLG 16       = 规格 rounded.xl（feature/FAQ 卡）
 *   boxShadow               = 规格 Level 2 粘性面板影字面值
 *   controlHeight 40        = 规格触达高（药丸 40–44px / 圆形钮 40px）
 *   fontWeightStrong 700    = 规格 subtitle-lg / body-md-bold 的强调档
 *   fontFamily              = --font-sans（Optimistic VF 专有 → 规格回退栈 Montserrat 等近似）
 */
export const metaAntdTheme: ThemeConfig = {
  token: {
    // Cobalt Primary：产品内购买/主操作 CTA（规格 button-buy-cta）
    colorPrimary: '#0064E0',
    colorPrimaryHover: '#0091FF', // 推导：Soft Cobalt 提亮档作 hover
    colorPrimaryActive: '#0457CB', // Deep Cobalt：pressed 字面值
    // info/link = 钴蓝一族（Deep Cobalt 即规格 active link color）
    colorInfo: '#0064E0',
    colorLink: '#0064E0',
    colorLinkHover: '#0457CB',
    colorLinkActive: '#0457CB',
    // info Alert = Soft Cobalt 15% 信息薄纱（规格「translucent background tint … at 15% alpha」）+ Hairline Gray
    colorInfoBg: '#D9F0FE',
    colorInfoBorder: '#CED0D4',
    // 语义三色：success 规格字面值；warning 深化；critical 字面值
    colorSuccess: '#31A24C',
    colorWarning: '#97690F',
    colorError: '#E41E3F',
    // 文字阶梯：Deep Ink / Ink / Charcoal / Stone（规格 Text 六级的亮面四级）
    colorText: '#0A1317',
    colorTextSecondary: '#1C1E21',
    colorTextTertiary: '#444950',
    colorTextQuaternary: '#8595A4',
    // 三面全白：画布、容器、浮层（stark white canvas）
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：Hairline Gray（输入 1px 边）/ Hairline Soft（卡分隔）
    colorBorder: '#CED0D4',
    colorBorderSecondary: '#DEE3E9',
    colorSplit: '#DEE3E9',
    // 焦点环 = 钴蓝薄纱（text-input-focused 的 2px fb-blue 语汇派生）
    controlOutline: 'rgba(0, 100, 224, 0.12)',
    // 圆角：输入 rounded.lg 8 / 卡 rounded.xl 16；按钮药丸由 Button 组件钉住
    borderRadius: 8,
    borderRadiusSM: 4,
    borderRadiusLG: 16,
    // 触达高：药丸 40–44px / 圆形钮 40px（规格 Touch Targets）
    controlHeight: 40,
    // 强调档 700（subtitle-lg / body-md-bold / button-md 均为 700）
    fontWeightStrong: 700,
    // Level 2 粘性面板影（规格字面值）；系统整体平坦，海拔只属 commerce 浮层
    boxShadow: '0 1px 4px 0 rgba(20, 22, 26, 0.3)',
    boxShadowSecondary: '0 1px 4px 0 rgba(20, 22, 26, 0.3)',
    // Optimistic VF 专有（禁外部加载）→ 规格回退栈 Montserrat/Helvetica/Noto Sans 近似
    fontFamily:
      "'Optimistic VF', Optimistic, Montserrat, Helvetica, Arial, 'Noto Sans', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Button: {
      // rounded.full 100px 全药丸（规格 Don't「buttons are NEVER squared」）；标签 700
      borderRadius: 100,
      borderRadiusSM: 100,
      borderRadiusLG: 100,
      fontWeight: 700,
      // 平坦系统：按钮无影（海拔只属粘性购买面板）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = button-secondary 描边次 CTA：墨字 + Deep Ink 边
      defaultBg: 'transparent',
      defaultColor: '#0A1317',
      defaultBorderColor: '#0A1317',
      defaultHoverBg: '#F1F4F7', // hover = Soft Cloud 微沉
      defaultHoverColor: '#0A1317',
      defaultHoverBorderColor: '#444950',
    },
    Menu: {
      // 下拉当前项 = Soft Cloud 底 + Deep Ink 字（surface-soft 工具面语汇）
      itemSelectedBg: '#F1F4F7',
      itemSelectedColor: '#0A1317',
    },
    Table: {
      // tech-specs-table：白底 + hairline-soft 行分隔 + charcoal 标签；行 hover = Soft Cloud
      headerBg: '#FFFFFF',
      headerColor: '#444950',
      headerSplitColor: '#DEE3E9',
      rowHoverBg: '#F1F4F7',
      borderColor: '#DEE3E9',
    },
    Tooltip: {
      // Deep Ink 反转面（promo-banner 的 ink-deep 深带语汇）
      colorBgSpotlight: '#0A1317',
    },
    Segmented: {
      trackBg: '#F1F4F7',
      itemSelectedBg: '#FFFFFF',
    },
    Select: {
      optionSelectedBg: '#F1F4F7',
    },
    Tag: {
      // badge 全药丸（badge-promo/attention/success/critical 均 rounded.full）
      borderRadiusSM: 100,
    },
  },
}
