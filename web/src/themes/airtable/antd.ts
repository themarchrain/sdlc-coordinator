import type { ThemeConfig } from 'antd'

/**
 * airtable 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Airtable 设计语言（编辑式白画布）：
 * - 黑即主色：「black IS the primary」——主 CTA 是近黑 #181D26
 *   而非链接蓝；「Don't make #1b61c9 the primary button color. It is the link color.」
 *   （规格专门警告这条最常见的误读，本主题照办：primary=墨、link=蓝）。
 * - 链接蓝 #1B61C9 只承载 text-link / info 家族（press 压深 #1A3866）。
 * - 白画布三面全白，靠 hairline #DDDDDD 发丝线与留白分层（无渐变、无氛围底纹）。
 * - 品牌电压在签名卡色（coral/cream/mustard 系），语义派生时复用（见 slots/css）。
 * - 字重只有 400/500（fontWeightStrong 500，全站无 bold）；按钮字重 500。
 * - 圆角刻度：输入/小件 6px、内容卡 10px、主 CTA/签名卡 12px（Button 钉 12）。
 * - 海拔哲学「color-block first, shadow second」：阴影克制，主按钮例外带
 *   蓝调微光（「subtle blue-tinted glow at low alpha」）。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/airtable.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#181D26，规格「primary / ink 同一角色」)
 *   colorPrimaryActive      = --primary-active  (#0D1218，规格 button-primary-active)
 *   colorInfo               = --info            (#254FAD，规格 info)
 *   colorInfoBorder         = 规格 info-border  (#458FFF，聚焦输入环蓝)
 *   colorLink               = --link            (#1B61C9，规格 link「actual link blue」)
 *   colorLink(Hover/Active) = --link-hover      (#1A3866，规格 link-active)
 *   colorSuccess            = --success         (#006400，规格 success)
 *   colorWarning/Error      = 派生（规格无此二色，warning 取 mustard 文字级、
 *                             error 取签名 coral 作氧化红，见 slots.ts 注）
 *   colorText 四阶           = ink/body/muted/border-strong
 *                             (#181D26 / #333840 / #41454D / #9297A0，规格 文字四级原文)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 白画布)
 *   colorBorder 双阶 + colorSplit = hairline 家族 (#DDDDDD / #E8E8E8)
 *   borderRadius 6 / SM 6 / LG 12 = 规格 rounded.sm(6) / rounded.lg(12)
 *   fontFamily              = --font-sans（Haas Grotesk 的规格 fallback 栈，禁外部字体）
 */
export const airtableAntdTheme: ThemeConfig = {
  token: {
    // 黑即主色：近黑墨 CTA（Primary；蓝是链接色不是按钮色）
    colorPrimary: '#181D26',
    colorPrimaryHover: '#2E343F', // 派生：规格无 hover，近黑微提亮一阶（active 才压深）
    colorPrimaryActive: '#0D1218', // 规格 button-primary-active
    // 链接蓝一族：info 与 link 同蓝（Semantic「link/info 色族」）
    colorInfo: '#254FAD',
    colorInfoBg: '#EAF1FD', // 派生：info 8% 薄纱（与 --info-soft 同源）
    colorInfoBorder: '#458FFF', // 规格 info-border（聚焦输入环蓝）
    colorLink: '#1B61C9',
    colorLinkHover: '#1A3866', // 规格 link-active（规格只记录 press 压深）
    colorLinkActive: '#1A3866',
    // 语义三色：success 规格 #006400；warning/error 规格未给 →
    // warning 取签名 mustard #D9A441 压到文字级 #8F6412；error 取签名 coral（氧化红）
    colorSuccess: '#006400',
    colorWarning: '#8F6412',
    colorError: '#AA2D00',
    // 文字阶梯：Ink / Body / Muted / Border Strong（Text 四级原文）
    colorText: '#181D26',
    colorTextHeading: '#181D26',
    colorTextSecondary: '#333840',
    colorTextTertiary: '#41454D',
    colorTextQuaternary: '#9297A0',
    // 白画布：页面、容器、浮层全白，靠 hairline 发丝线与留白分层
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线：hairline #DDDDDD（控件/表格线）+ 派生 #E8E8E8（更安静的分隔线）
    colorBorder: '#DDDDDD',
    colorBorderSecondary: '#E8E8E8',
    colorSplit: '#E8E8E8',
    // 圆角：输入/小件 6（rounded.sm）；大面卡 12（rounded.lg）
    borderRadius: 6,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 字重克制：只有 400/500，无 bold
    fontWeightStrong: 500,
    // 海拔克制（color-block first, shadow second）；浮层只留可读性所需的薄影
    boxShadow: '0 4px 16px rgba(24, 29, 38, .10)',
    boxShadowSecondary: '0 8px 24px rgba(24, 29, 38, .12)',
    // Haas Grotesk 的规格 fallback 栈（原文）+ CJK 补齐
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Oxygen, Ubuntu, Cantarell, "Fira Sans", "Droid Sans", "Helvetica Neue", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 主 CTA = 12px 圆角（规格 rounded.lg「Primary CTA buttons」）；小按钮回落 6
      borderRadius: 12,
      borderRadiusSM: 6,
      borderRadiusLG: 12,
      // 按钮字重 500（规格 typography.button 16px/500）
      fontWeight: 500,
      // 主按钮带蓝调微光（「subtle blue-tinted glow」）；次按钮白底发丝线扁平
      primaryShadow: '0 2px 8px rgba(27, 97, 201, .18)',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 当前项 = surface-soft 微高亮 + 墨字（白系导航不叠彩）
      itemSelectedBg: '#F8FAFC',
      itemSelectedColor: '#181D26',
    },
    Table: {
      // 白底表头 + muted 标签 + 透明分隔缝；行 hover = surface-soft（规格 表格 hairline 分隔）
      headerBg: '#FFFFFF',
      headerColor: '#41454D',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F8FAFC',
      borderColor: '#E8E8E8',
    },
    Tooltip: {
      // 白画布上的提示 = 墨底白字（ink 反转面）
      colorBgSpotlight: '#181D26',
    },
  },
}
