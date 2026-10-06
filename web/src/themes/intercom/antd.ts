import type { ThemeConfig } from 'antd'

/**
 * intercom 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Intercom 设计语言 = 暖奶油编辑感：奶油画布 #F5F1EC（规格明令禁纯白）+ 炭墨
 * #111111 一族文字；**系统主色是炭墨本身**（button-primary = 墨底白字），
 * Fin Orange #FF5600 只留给 Fin AI 徽标级强调，绝不作通用交互色。
 * 海拔 = 白卡浮在奶油上的明度差（「Intercom resists drop shadows」→ 两级阴影全关）。
 * 算法未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/intercom.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#111111，规格「Charcoal is the system primary」)
 *   colorPrimaryHover       = 炭墨微提亮         (#383631，派生：墨底按钮 hover 惯例)
 *   colorPrimaryActive      = inverse-canvas     (#000000，规格 button-primary-pressed 字面)
 *   colorInfo/colorLink     = Brand Blue        (#0007CB，规格 brand-blue 字面)
 *   colorInfoBg/colorInfoBorder = 蓝 6% 薄纱 + Hairline (#EDEFFA 派生 / #D3CEC6 字面)
 *   colorSuccess            = semantic-success   (#0BDF50，规格 字面，同 report-green)
 *   colorWarning            = 克制琥珀           (#A16207，派生：规格无警示色)
 *   colorError              = semantic-error     (#C41C1C，规格 字面)
 *   colorText 四阶          = Ink/Ink Muted/Ink Subtle/Ink Tertiary
 *                             (#111111 / #626260 / #7B7B78 / #9C9FA5，规格 文字四级原文)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#F5F1EC 奶油 + #FFFFFF 白卡)
 *   colorBorder 双阶 + colorSplit = Hairline / Hairline Soft (#D3CEC6 / #EBE7E1，规格 发丝线双色)
 *   borderRadius 8          = rounded.md（规格「All buttons, form inputs」8px）
 *   borderRadiusLG 12       = rounded.lg（pricing/feature 卡 12px）
 *   fontWeightStrong 500    = 规格「Weight 500 carries display… without bold」
 *   fontFamily              = Saans → Inter 替代栈（规格「Font Substitutes」钦点，
 *                             禁外部字体加载；SaansMono 同理并入系统 mono）
 */
export const intercomAntdTheme: ThemeConfig = {
  token: {
    // 炭墨即主色：默认 CTA = 墨底白字
    colorPrimary: '#111111',
    colorPrimaryHover: '#383631',
    colorPrimaryActive: '#000000',
    // info / link = Brand Blue（「Brand Blue」；report 色板不上品牌面）
    colorInfo: '#0007CB',
    colorLink: '#0007CB',
    colorLinkHover: '#0005A3',
    // 钉住 info Alert 为「蓝薄纱软面 + 奶油发丝线」的克制 info 形态
    colorInfoBg: '#EDEFFA',
    colorInfoBorder: '#D3CEC6',
    // 语义三色：error/success 取 规格 semantic 字面；warning 无规格 → 克制琥珀（派生）
    colorSuccess: '#0BDF50',
    colorWarning: '#A16207',
    colorError: '#C41C1C',
    // 文字阶梯：Ink / Ink Muted / Ink Subtle / Ink Tertiary（四级原文）
    colorText: '#111111',
    colorTextSecondary: '#626260',
    colorTextTertiary: '#7B7B78',
    colorTextQuaternary: '#9C9FA5',
    // 奶油画布 + 白卡：页面奶油、容器白，海拔靠明度差承担
    colorBgLayout: '#F5F1EC',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：Hairline（卡外线）/ Hairline Soft（FAQ/页脚软分隔）
    colorBorder: '#D3CEC6',
    colorBorderSecondary: '#EBE7E1',
    colorSplit: '#EBE7E1',
    // 圆角刻度：8px 交互件（rounded.md）、12px 卡（rounded.lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 字重克制：500 承载展示、400 正文，无 bold
    fontWeightStrong: 500,
    // 零投影体系：海拔 = 白上奶油的表面切换（「resists drop shadows」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Saans（定制族）→ Inter 替代栈（规格「Font Substitutes」：Inter weight 500 最近似）
    fontFamily:
      "'Saans', 'Saans Fallback', Inter, ui-sans-serif, system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Select: { optionSelectedBg: 'rgba(17, 17, 17, 0.08)' },
    Button: {
      // 按钮标签 15px/500（规格 typography.button）；零阴影
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // button-secondary = 白底 + 1px Hairline（规格 字面）
      defaultBg: '#FFFFFF',
      defaultBorderColor: '#D3CEC6',
    },
    Menu: {
      // 当前项 = 「lift onto white」：白底高亮 + 炭墨（规格 pricing-tab-selected 同构）
      itemSelectedBg: '#FFFFFF',
      itemSelectedColor: '#111111',
    },
    Table: {
      // 白底表头 + Ink Muted 标签 + 透明分隔缝；行 hover = 奶油透出
      headerBg: '#FFFFFF',
      headerColor: '#626260',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F5F1EC',
      borderColor: '#EBE7E1',
    },
    Tooltip: {
      // 反转提示面 = 炭墨（规格 inverse 家族的界面级落点）
      colorBgSpotlight: '#111111',
    },
  },
}
