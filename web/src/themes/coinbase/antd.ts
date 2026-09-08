import type { ThemeConfig } from 'antd'

/**
 * coinbase 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Coinbase 设计语言 = 机构级金融品牌的编辑级冷静：白画布 + 墨阶文字 +
 * 唯一品牌电压 Coinbase Blue #0052FF（稀缺使用，按压 #003ECC），
 * 全药丸 CTA（rounded.pill）、24px 大圆卡、单一阴影层 0 4px 12px rgba(0,0,0,.04)，
 * 交易语义 up/down 只作文字色（#05B169 / #CF202F），绝不作按钮底色。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/coinbase.css 的同名令牌手工保持同源：
 *   colorPrimary              = --primary        (#0052FF，规格「single brand voltage」)
 *   colorPrimaryActive        = --primary-active (#003ECC，规格 primary-active 原文)
 *   colorPrimaryHover         = #0045E5          (派生：primary 与 active 中点，规格未记录 hover)
 *   colorInfo/colorLink       = --info/--link    (#0052FF，蓝即唯一动作色)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline
 *                                                (#EEF3FF 蓝薄纱 + #DEE1E6 发丝线)
 *   colorSuccess/Error        = 语义 up/down     (#05B169 / #CF202F，规格「text color only」)
 *   colorWarning              = #9A6F00          (派生：accent-yellow #F4B000 深化为文字级琥珀，
 *                                                 规格无 warning 令牌)
 *   colorText 四阶            = ink/body/muted/muted-soft
 *                              (#0A0B0D / #5B616E / #7C828A / #A8ACB3，规格 文字四级原文)
 *   colorBgLayout/Container/Elevated = --canvas  (#FFFFFF，白画布)
 *   colorBorder 双阶 + colorSplit = #DEE1E6 / #EEF0F3 (规格 hairline 双色)
 *   borderRadius 12           = 规格 rounded.md「Form inputs」
 *   borderRadiusLG 24         = 规格 rounded.xl「Feature cards / product-UI mockups」
 *   boxShadow 单层            = 规格 唯一阴影档「0 4px 12px rgba(0,0,0,.04)」
 *   fontFamily                = --font-sans（CoinbaseSans 的 Inter 替代栈，禁外部字体）
 * 按钮药丸几何（rounded.pill）由 themes/css/coinbase.css 的 .ant-btn 片段承担
 * （antd Button 组件 token 无对应半径档，9999px 药丸交由 CSS 更稳）。
 */
export const coinbaseAntdTheme: ThemeConfig = {
  token: {
    // Coinbase Blue：唯一品牌电压
    colorPrimary: '#0052FF',
    colorPrimaryHover: '#0045E5', // 派生：primary→active 中点（规格只记录 Default/Active）
    colorPrimaryActive: '#003ECC',
    // info / link 与 primary 同蓝：蓝是唯一动作色信号
    colorInfo: '#0052FF',
    colorLink: '#0052FF',
    colorLinkHover: '#0045E5',
    colorLinkActive: '#003ECC',
    // 钉住 info Alert 为「蓝薄纱软面 + hairline 发丝线」
    colorInfoBg: '#EEF3FF',
    colorInfoBorder: '#DEE1E6',
    // 交易语义：up/down 仅文字色
    colorSuccess: '#05B169',
    colorError: '#CF202F',
    // 派生：accent-yellow #F4B000 深化为文字级琥珀（规格无 warning，黄仅插画）
    colorWarning: '#9A6F00',
    // 文字阶梯：ink / body / muted / muted-soft（四级原文）
    colorText: '#0A0B0D',
    colorTextSecondary: '#5B616E',
    colorTextTertiary: '#7C828A',
    colorTextQuaternary: '#A8ACB3',
    // 白画布：页面、容器、浮层全白，靠发丝线与留白分隔
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline（控件边/分隔）/ hairline-soft（浅分隔）
    colorBorder: '#DEE1E6',
    colorBorderSecondary: '#EEF0F3',
    colorSplit: '#EEF0F3',
    // 圆角：输入 12（rounded.md）；小件 8（rounded.sm）；大面卡 24（rounded.xl）
    borderRadius: 12,
    borderRadiusSM: 8,
    borderRadiusLG: 24,
    // 标题/强调 600（title-md/sm 600）；展示级 400 由 css 片段另行压低
    fontWeightStrong: 600,
    // 系统唯一阴影档（「Single shadow tier」）
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.04)',
    boxShadowSecondary: '0 4px 12px rgba(0, 0, 0, 0.04)',
    // CoinbaseSans 的 Inter 系统栈替代（规格 字体为授权族，禁外部加载）
    fontFamily:
      '"Coinbase Sans", Inter, -apple-system, system-ui, "Segoe UI", Roboto, Helvetica, Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 600（规格 typography.button 16px/600）；三态阴影全关（单一阴影层不落在按钮上）
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = 蓝薄纱微高亮 + 墨字（蓝稀缺：底纱极淡）
      itemSelectedBg: 'rgba(0, 82, 255, 0.08)',
      itemSelectedColor: '#0A0B0D',
    },
    Table: {
      // 软灰带表头（surface-soft #F7F7F7）+ body 级标签；行 hover = 软灰带
      headerBg: '#F7F7F7',
      headerColor: '#5B616E',
      headerSplitColor: '#DEE1E6',
      rowHoverBg: '#F7F7F7',
      borderColor: '#EEF0F3',
    },
    Tooltip: {
      // 深近黑 editorial 面作 tooltip 底（surface-dark，白字）
      colorBgSpotlight: '#0A0B0D',
    },
  },
}
