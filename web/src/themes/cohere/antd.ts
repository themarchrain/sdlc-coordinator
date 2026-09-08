import type { ThemeConfig } from 'antd'

/**
 * cohere 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Cohere 设计语言 = 白画布编辑部 + 暗色产品带：主操作色不是彩色的，
 * 而是 near-black #17171C（药丸 CTA / 深页脚 / agent-console 深面板），
 * action-blue #1863DC 只做链接/分页等「数据墨水」，coral #FF7759 只做
 * 编辑部小记号（分类 chips），soft-stone #EEECE7 提供唯一暖色表面。
 * 字重近全 400/500（规格「avoid heavy bold weights」），海拔近零
 * （卡片靠发丝线 #D9D9DD–#F2F2F2 与留白分隔，阴影全关）。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/cohere.css 的同名令牌手工保持同源：
 *   colorPrimary             = --primary         (#17171C，规格 colors.primary，近黑药丸 CTA)
 *   colorPrimaryHover        = 提亮一阶           (#34343B，派生；深底按钮 hover 走 lighten)
 *   colorPrimaryActive       = cohere-black      (#000000，按压落品牌黑锚点)
 *   colorInfo/colorLink      = --info/--link     (#1863DC action-blue，编辑链接与分页色)
 *   colorInfoBg              = pale-blue wash    (#F1F5FF，规格 Blog CTA 面同源)
 *   colorSuccess/Warning     = 派生语义对（规格只给了 error）：黄绿 pale-green 家族
 *                              压到文字级 #4D7C0F；coral 家族压深 #C2410C
 *   colorError               = --error           (#B30000，规格 Semantic 原文)
 *   colorText 四阶           = Ink/Body Muted/Slate/Muted
 *                              (#212121 / #616161 / #75758A / #93939F，规格 文字四级原文)
 *   colorBgLayout/Container/Elevated = --canvas (#FFFFFF 白画布)
 *   colorBorder 双阶 + colorSplit = Hairline / Border Light (#D9D9DD / #E5E7EB)
 *   borderRadius 8 / SM 4 / LG 8 = 规格 圆角刻度（xs 4 小件 / sm 8 卡与对话框）
 *   fontWeightStrong 500     = 规格最重字重（button 500），全站无 heavy bold
 *   fontFamily               = Unica77 的文档化回退栈（Inter → Arial → system-ui，禁外部字体）
 */
export const cohereAntdTheme: ThemeConfig = {
  token: {
    // Near-Black Primary：主操作色（「Primary CTA buttons, dark footer」）
    colorPrimary: '#17171C',
    colorPrimaryHover: '#34343B', // 派生：深色按钮 hover 微提亮（规格未给 hover）
    colorPrimaryActive: '#000000', // 按压落 Cohere Black（规格「global brand anchor」）
    // info / link = action-blue：编辑链接、分页与次级强调
    colorInfo: '#1863DC',
    colorLink: '#1863DC',
    colorLinkHover: '#1257C4', // 派生：action-blue 压深一阶
    // info Alert 钉成「pale-blue 薄面 + hairline 边」（规格 Pale Blue Wash 的唯一中性复用）
    colorInfoBg: '#F1F5FF',
    colorInfoBorder: '#D9D9DD',
    // 语义三色（规格 Semantic 段只给了 error；success/warning 按暖黄绿/珊瑚家族派生）
    colorSuccess: '#4D7C0F',
    colorWarning: '#C2410C',
    colorError: '#B30000',
    // 文字阶梯：Ink / Body Muted / Slate / Muted（四级原文）
    colorText: '#212121',
    colorTextSecondary: '#616161',
    colorTextTertiary: '#75758A',
    colorTextQuaternary: '#93939F',
    // 白画布：页面、容器、浮层全白，靠发丝线与大量留白分隔
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：Hairline（标准规则线）/ Border Light（次级分隔）
    colorBorder: '#D9D9DD',
    colorBorderSecondary: '#E5E7EB',
    colorSplit: '#E5E7EB',
    // 圆角刻度：4px 小件、8px 卡/对话框为常态，22px 留给媒体/登录外壳（CSS 层承担）
    borderRadius: 8,
    borderRadiusSM: 4,
    borderRadiusLG: 8,
    // 规格最重字重即 500（button）；「size、spacing、surface contrast 做层级」
    fontWeightStrong: 500,
    // 近零海拔（规格「mostly flat」「Do not add heavy drop shadows」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Unica77 的文档化回退栈（规格 Known Gaps：用文档回退实现，禁外部字体）
    fontFamily:
      'Inter, Arial, ui-sans-serif, system-ui, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 500（规格 button 14px/500）；药丸形态（pill 32px）由 CSS 片段统一
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = Soft Stone 暖高亮 + Ink（product-card 表面语汇，不引蓝色）
      itemSelectedBg: '#EEECE7',
      itemSelectedColor: '#212121',
    },
    Table: {
      // research-table：白底 + Slate 标签 + 透明分隔缝；行 hover = 派生暖灰（stone 提亮）
      headerBg: '#FFFFFF',
      headerColor: '#75758A',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F5F3EF',
      borderColor: '#E5E7EB',
    },
    Tooltip: {
      colorBgSpotlight: '#17171C', // agent-console near-black 反转面
    },
  },
}
