import type { ThemeConfig } from 'antd'

/**
 * cal 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Cal.com 设计语言 = 友好现代 SaaS 的单色动作层：白画布 + 近黑主 CTA
 * #111111（按压 #242424；按钮 8px 圆角、600 字重、扁平无阴影），
 * Cal Sans 展示字族 600 + 负字距（Inter 600 替代，永不 700），
 * 灰面三档 #F8F9FA/#F5F5F5/#E5E7EB，品牌蓝 #3B82F6 仅稀缺内联时刻
 * （链接 hover、徽章高亮），深色面 #101010 只属于页脚与 featured 逆转。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/cal.css 的同名令牌手工保持同源：
 *   colorPrimary        = --primary         (#111111，规格「near-black, not blue」)
 *   colorPrimaryActive  = --primary-active  (#242424，规格 primary-active 原文)
 *   colorPrimaryHover   = #1A1A1A           (派生：按压前的极轻提亮，规格只记录 Default/Active)
 *   colorInfo           = --info            (#3B82F6，brand-accent 作 info)
 *   colorLink           = --link            (#111111，内联链接单色；hover 转蓝见 colorLinkHover)
 *   colorSuccess/Warning/Error = 语义三色   (#10B981 / #F59E0B / #EF4444，规格直书)
 *   colorText 四阶      = ink/body/muted/muted-soft
 *                        (#111111 / #374151 / #6B7280 / #898989，规格 文字四级原文)
 *   colorBgLayout/Container/Elevated = --canvas (#FFFFFF，白画布)
 *   colorBorder 双阶 + colorSplit = #E5E7EB / #F3F4F6 (规格 hairline 双色)
 *   borderRadius 8      = 规格 rounded.md「Standard CTA buttons / text inputs」
 *   borderRadiusLG 12   = 规格 rounded.lg「Content cards」
 *   boxShadow 双档      = 规格 微投影 0 1px 2px rgba(0,0,0,.05) / 0 4px 12px rgba(0,0,0,.08)
 *   fontWeightStrong 600 = Cal Sans「never 700, never 500」
 *   fontFamily          = --font-sans（Inter 系统栈，禁外部字体）
 * 展示字族/负字距与药丸套药丸导航由 themes/css/cal.css 片段承担。
 */
export const calAntdTheme: ThemeConfig = {
  token: {
    // 近黑主 CTA：单色动作层（「button is near-black, not blue」）
    colorPrimary: '#111111',
    colorPrimaryHover: '#1A1A1A', // 派生：按压前的极轻提亮（规格只记录 Default/Active）
    colorPrimaryActive: '#242424',
    // info = brand-accent 蓝（稀缺内联时刻）；链接单色、hover 才转蓝
    colorInfo: '#3B82F6',
    colorLink: '#111111',
    colorLinkHover: '#3B82F6',
    colorLinkActive: '#242424',
    // info Alert = 蓝薄纱软面 + hairline 发丝线
    colorInfoBg: '#EFF6FF',
    colorInfoBorder: '#E5E7EB',
    // 语义三色（规格 colors 直书）
    colorSuccess: '#10B981',
    colorWarning: '#F59E0B',
    colorError: '#EF4444',
    // 文字阶梯：ink / body / muted / muted-soft（四级原文）
    colorText: '#111111',
    colorTextSecondary: '#374151',
    colorTextTertiary: '#6B7280',
    colorTextQuaternary: '#898989',
    // 白画布：页面、容器、浮层全白，卡靠灰底/发丝线/微投影分层
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline（输入边/表分隔）/ hairline-soft（浅分隔）
    colorBorder: '#E5E7EB',
    colorBorderSecondary: '#F3F4F6',
    colorSplit: '#F3F4F6',
    // 圆角：按钮/输入 8（rounded.md）；小件 6（rounded.sm）；内容卡 12（rounded.lg）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // Cal Sans 法则：展示字重封顶 600（规格「never 700, never 500」）
    fontWeightStrong: 600,
    // 微投影双档（small drop shadows on elevated cards）
    boxShadow: '0 1px 2px rgba(0, 0, 0, 0.05)',
    boxShadowSecondary: '0 4px 12px rgba(0, 0, 0, 0.08)',
    // Inter 系统栈（Cal Sans 授权不可用，规格文档替代 Inter 600）
    fontFamily:
      'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(17, 17, 17, 0.08)' },
    Button: {
      // 按钮字重 600（规格 typography.button 14px/600）；扁平无阴影（投影留给卡）
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = surface-card 灰 + 墨字（单色动作层，不用蓝）
      itemSelectedBg: '#F5F5F5',
      itemSelectedColor: '#111111',
    },
    Table: {
      // surface-soft 表头 + muted 标签；行 hover = 同款极软灰
      headerBg: '#F8F9FA',
      headerColor: '#6B7280',
      headerSplitColor: '#E5E7EB',
      rowHoverBg: '#F8F9FA',
      borderColor: '#F3F4F6',
    },
    Tooltip: {
      // 深 footer 面作 tooltip 底（surface-dark，白字）
      colorBgSpotlight: '#101010',
    },
  },
}
