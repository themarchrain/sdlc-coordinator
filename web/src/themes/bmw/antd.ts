import type { ThemeConfig } from 'antd'

/**
 * bmw 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * BMW corporate 设计语言（亮轨 #ffffff 画布，与 bmw-m 暗色主题刻意区分）：
 * - 白画布 + 近黑墨 #262626，暗军蓝 #1A2129 只留给 hero 带（此处承担 tooltip）；
 * - BMW corporate blue #1C69D4 是唯一主操作色（规格「the single brand action color」），
 *   按压 = 规格 primary-active #0653B6；M 三色条纹不进本主题任何槽位
 *   （规格「never in the corporate site's main language」→ 与 bmw-m 分工）；
 * - 矩形 0px 圆角即品牌（规格「rectangular, 0px corner」；modal 12px 属极罕例外，
 *   不值得为此放开大面）；
 * - 字重签名 700/300（fontWeightStrong 700；Button 700），
 *   BMW Type Next Latin → Inter 系统/web-safe 栈近似（规格 官方替身）；
 * - 全站无投影（规格「never uses a drop shadow」→ 两级阴影全关）；
 * - 语义三色取规格原生值 success #22C55E / warning #F59E0B / error #DC2626。
 * 动效时长：规格 Known Gaps 明言未提取 → 不钉住，随 antd 默认。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/bmw.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#1C69D4，规格 primary)
 *   colorPrimaryHover       = 蓝提亮一阶        (#3E7EDD，规格未记 hover → 派生)
 *   colorPrimaryActive      = --primary-active  (#0653B6，规格 primary-active)
 *   colorInfo/colorLink     = --info/--link     (#1C69D4，蓝即唯一交互信号)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline-strong
 *                                                (#E9F2FC 蓝 6% 薄纱 + #CCCCCC 强控件边)
 *   colorSuccess/Warning/Error = --success/--warning/--error（规格 Semantic 组原值）
 *   colorText 四阶           = ink/body/muted/muted-soft
 *                             (#262626 / #3C3C3C / #6B6B6B / #9A9A9A，规格文字四级原文)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF，白画布)
 *   colorBorder 双阶 + colorSplit = hairline/hairline-soft
 *                             (#E6E6E6 规格 hairline / #F0F0F0 半步软线派生)
 *   borderRadius 0          = 规格「rectangular, 0px corner」
 *   fontFamily              = --font-sans（Inter = 规格 记载的 BMW Type Next 替身）
 */
export const bmwAntdTheme: ThemeConfig = {
  token: {
    // BMW corporate blue：唯一主操作色
    colorPrimary: '#1C69D4',
    colorPrimaryHover: '#3E7EDD',
    colorPrimaryActive: '#0653B6',
    // info / link 与 primary 同蓝：蓝是唯一交互信号
    colorInfo: '#1C69D4',
    colorLink: '#1C69D4',
    colorLinkHover: '#3E7EDD',
    colorLinkActive: '#0653B6',
    // 钉住 info Alert 为「蓝薄纱软面 + hairline-strong 控件边」（先例：tesla/antd.ts）
    colorInfoBg: '#E9F2FC',
    colorInfoBorder: '#CCCCCC',
    // 语义三色：规格 Semantic 组原值（confirmation / warning callouts / validation）
    colorSuccess: '#22C55E',
    colorWarning: '#F59E0B',
    colorError: '#DC2626',
    // 文字阶梯：ink #262626 / body #3C3C3C / muted #6B6B6B / muted-soft #9A9A9A（规格四级原文）
    colorText: '#262626',
    colorTextHeading: '#262626',
    colorTextSecondary: '#3C3C3C',
    colorTextTertiary: '#6B6B6B',
    colorTextQuaternary: '#9A9A9A',
    // 白画布：页面、容器、浮层全白，靠发丝线与色块对比分层
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline #E6E6E6（输入/表格分隔）/ 半步软线 #F0F0F0（派生）
    colorBorder: '#E6E6E6',
    colorBorderSecondary: '#F0F0F0',
    colorSplit: '#F0F0F0',
    // focus 光环 = 蓝 28%（与 --focus-ring 同源）
    controlOutline: 'rgba(28, 105, 212, 0.28)',
    // 圆角一律 0：矩形轮廓即品牌（「rectangular for everything」）
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 0,
    // 重展示字 700 / Light 300 正文是编辑签名
    fontWeightStrong: 700,
    // 全站无投影（「never uses a drop shadow」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // BMW Type Next Latin → Inter 官方替身（规格「Note on Font Substitutes」）
    fontFamily:
      'Inter, system-ui, -apple-system, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
  },
  components: {
    Button: {
      // 按钮字重 700（规格 typography.button 14px/700/0.5px，字距见 CSS 片段）
      fontWeight: 700,
      borderRadius: 0,
      borderRadiusSM: 0,
      borderRadiusLG: 0,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = surface-card #FAFAFA 微高亮 + ink 墨（规格「subtle background highlight」族）
      itemSelectedBg: '#FAFAFA',
      itemSelectedColor: '#262626',
    },
    Table: {
      // 白底表头 + muted 标签 + 透明分隔缝；行 hover = #FAFAFA；分隔 = hairline
      headerBg: '#FFFFFF',
      headerColor: '#6B6B6B',
      headerSplitColor: 'transparent',
      headerBorderRadius: 0,
      rowHoverBg: '#FAFAFA',
      borderColor: '#E6E6E6',
    },
    Tooltip: {
      // tooltip = 暗军蓝 hero 带面（规格 surface-dark 的浮层化身）
      colorBgSpotlight: '#1A2129',
    },
  },
}
