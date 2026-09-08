import type { ThemeConfig } from 'antd'

/**
 * clay 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Clay 设计语言 = claymation 暖奶油画布：#FFFAF0 cream canvas 承载近黑墨字，
 * 主 CTA 是「带一点暖意的近黑」#0A0A0A（非彩色按钮），品牌电压全部让给
 * 6 色饱和单色卡（hot pink / deep teal / lavender / peach / ochre / cream）。
 * 圆角慷慨：按钮/输入 12px（rounded.md）、内容卡 16px、特色卡 24px；
 * 字重只有 400/500/600（display 500 + 负字距，标题 600，无 700）。
 * 海拔近零：靠发丝线与奶油-饱和色对比分层，阴影只在罕见悬浮态低 alpha 出现。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/clay.css 的同名令牌手工保持同源：
 *   colorPrimary             = --primary          (#0A0A0A，规格「primary: near-black」)
 *   colorPrimaryHover        = #1F1F1F            (规格 button-primary-active 字面值，
 *                                                   Clay 按压是「微提亮」而非压深)
 *   colorPrimaryActive       = #2E2E2E            (派生：提亮方向再续一档)
 *   colorInfo                = --info             (#B8A4ED，规格「lavender for AI-agent
 *                                                   products」→ info 家族挂薰衣草)
 *   colorInfoBg/colorInfoBorder = lavender 薄纱    (#F4F1FC + rgba(184,164,237,.45))
 *   colorLink                = --link             (#0A0A0A，规格 text-link = ink + 下划线)
 *   colorSuccess/Warning/Error = #22C55E/#F59E0B/#EF4444（规格 colors 语义三色原文）
 *   colorText 四阶           = ink/body/muted/muted-soft
 *                             (#0A0A0A/#3A3A3A/#6A6A6A/#9A9A9A，规格 文字四级原文)
 *   colorBgLayout/Container  = --canvas           (#FFFAF0 cream canvas)
 *   colorBgElevated          = #FFFFFF            (规格 button-on-color 白面 = 提升面)
 *   colorBorder 双阶 + colorSplit = hairline/hairline-soft (#E5E5E5/#F0F0F0)
 *   borderRadius 12          = 规格 rounded.md（按钮/输入「friendly modern but not pill」）
 *   borderRadiusLG 16        = 规格 rounded.lg（内容卡）
 *   fontWeightStrong 600     = 规格 title 层全 600
 *   boxShadow                = 低 alpha 微影（规格「rare subtle drop shadow」）
 *   fontFamily               = Inter 栈（Plain Black 定制族禁外部加载，规格 明示
 *                               Inter 为官方替身）
 */
export const clayAntdTheme: ThemeConfig = {
  token: {
    // 主 CTA = 近黑（dark-navy/black primary CTAs）
    colorPrimary: '#0A0A0A',
    colorPrimaryHover: '#1F1F1F',
    colorPrimaryActive: '#2E2E2E',
    // info 家族挂 lavender（规格 迭代指南：「lavender for AI-agent products」）
    colorInfo: '#B8A4ED',
    colorInfoBg: '#F4F1FC',
    colorInfoBorder: 'rgba(184, 164, 237, .45)',
    // 链接 = 墨色 + 下划线（规格 text-link：ink with underline；下划线见 clay.css 片段）
    colorLink: '#0A0A0A',
    colorLinkHover: '#1F1F1F',
    // 语义三色（原文）
    colorSuccess: '#22C55E',
    colorWarning: '#F59E0B',
    colorError: '#EF4444',
    // 文字阶梯：ink / body / muted / muted-soft（四级原文）
    colorText: '#0A0A0A',
    colorTextSecondary: '#3A3A3A',
    colorTextTertiary: '#6A6A6A',
    colorTextQuaternary: '#9A9A9A',
    // 奶油画布：页面/容器 = cream；浮层 = 白（规格 button-on-color 的白面即提升面）
    colorBgLayout: '#FFFAF0',
    colorBgContainer: '#FFFAF0',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline（控件边）/ hairline-soft（分隔线）
    colorBorder: '#E5E5E5',
    colorBorderSecondary: '#F0F0F0',
    colorSplit: '#F0F0F0',
    // 圆角：12 为按钮/输入默认（rounded.md）；16 给大面内容卡（rounded.lg）
    borderRadius: 12,
    borderRadiusSM: 8,
    borderRadiusLG: 16,
    // 标题层全 600（
    // 均 600；display 的 500 + 负字距在 clay.css 片段处理）
    fontWeightStrong: 600,
    // 海拔近零：仅保留低 alpha 微影给浮层（规格「rare subtle drop shadow at low alpha」）
    boxShadow: '0 6px 16px rgba(10, 26, 26, .08)',
    boxShadowSecondary: '0 8px 24px rgba(10, 26, 26, .1)',
    // Plain Black 为 Clay 定制族（禁外部加载）→ 规格 明示 Inter 为替身
    fontFamily:
      'Inter, -apple-system, "Segoe UI", Roboto, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 600（规格 typography.button 14px/600）； Clay 按钮无投影
      fontWeight: 600,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = cream card 面 + 墨字（规格 category-tab-active 同构）
      itemSelectedBg: '#F5F0E0',
      itemSelectedColor: '#0A0A0A',
    },
    Table: {
      // 奶油卡表头 + muted 标签 + 透明分隔缝；行 hover = surface-soft 微沉
      headerBg: '#F5F0E0',
      headerColor: '#6A6A6A',
      headerSplitColor: 'transparent',
      rowHoverBg: '#FAF5E8',
      borderColor: '#F0F0F0',
    },
    Tooltip: {
      // 反转面 = surface-dark 暗青近黑
      colorBgSpotlight: '#0A1A1A',
    },
  },
}
