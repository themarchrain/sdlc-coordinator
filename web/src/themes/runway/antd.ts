import type { ThemeConfig } from 'antd'

/**
 * runway 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Runway 设计语言 = 电影节场刊式编辑极简：纸白画布 + 近黑墨 #030303，
 * 黑色药丸是唯一主行动语言（primary #000000 + rounded.full），无第二交互色；
 * 五级灰阶（ink → graphite → slate → stone → hairline）承担全部 UI，
 * slate #676F7B 是唯一带色温的中性（tertiary 元数据专用）。
 * 圆角法则严格二分：按钮一律药丸、格子/表行/表单一律方角（规格「Never mix」）；
 * 零阴影零光晕（深度来自摄影与色阶，不来自投影）；标题一律 400 常规字重 +
 * 负字距（编辑排印扛层级，见 css 片段）。algorithm 未指定 = antd 默认算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/runway.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#000000，规格「single brand action colour」黑药丸)
 *   colorPrimaryHover/Active = 黑压不动 → 提亮两阶 (#1A1A1A ink-soft / #404040 graphite，
 *                             黑无法再深，按压走「soften one click」推导)
 *   colorInfo               = --info            (#030303 墨；单色系统里墨即唯一交互信号)
 *   colorLink               = --link            (#030303 ink，规格 text-link「text {colors.ink}」原文)
 *   colorInfoBg/colorInfoBorder = alert-banner 规格 (#FFFFFF 画布底 + #C9CCD1 hairline-soft 边)
 *   colorSuccess/Warning/Error = 派生（规格 明言「不引入信号色」→ 后台语义必需，
 *                             取标准语义的低饱和化：#2F7A4D / #946F12 / #B3261E）
 *   colorText 四阶          = Ink/Graphite/Slate/Stone (#030303/#404040/#676F7B/#939393 规格原文)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 纸白画布)
 *   colorBorder 双阶 + colorSplit = #C9CCD1 hairline-soft（列规线/控件边）/ #E7EAF0 hairline
 *   borderRadius 0          = 规格 rounded.none（表单/表格/格子方角，全局方角）
 *   Button borderRadius 9999 = 规格 rounded.full（按钮药丸；方角格子 + 药丸按钮的严格二分）
 *   borderRadiusLG 16       = 规格 rounded.lg（alert-banner / 大容器）
 *   fontFamily              = --font-sans（abcNormal → Inter 规格钦定替代，禁外部加载）
 */
export const runwayAntdTheme: ThemeConfig = {
  token: {
    // 黑色药丸：唯一主行动色（「Brand & Accent」Black）
    colorPrimary: '#000000',
    // 黑压不动 → 派生提亮两阶（规格 ink-soft「one click softer」/ graphite）
    colorPrimaryHover: '#1A1A1A',
    colorPrimaryActive: '#404040',
    // info / 墨：单色系统里墨即交互信号；info Alert = alert-banner 形态（白底 + hairline-soft 边）
    colorInfo: '#030303',
    colorInfoBg: '#FFFFFF',
    colorInfoBorder: '#C9CCD1',
    // 链接 = ink 字（规格 button-text-link「text {colors.ink}, underline-on-active」）；
    // hover 的 graphite 微移是派生反馈，下划线信号由 css 片段承担
    colorLink: '#030303',
    colorLinkHover: '#404040',
    colorLinkActive: '#030303',
    // 语义三色（明言不引入信号色 → 后台必需，取低饱和推导）
    colorSuccess: '#2F7A4D',
    colorWarning: '#946F12',
    colorError: '#B3261E',
    // 文字阶梯：Ink / Graphite / Slate / Stone（「Text」四级原文）
    colorText: '#030303',
    colorTextSecondary: '#404040',
    colorTextTertiary: '#676F7B',
    colorTextQuaternary: '#939393',
    // 纸白画布：页面/容器/浮层全白，靠发丝线与留白分层
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline-soft #C9CCD1（1px 列规线/控件边）/ hairline #E7EAF0（分隔线）
    colorBorder: '#C9CCD1',
    colorBorderSecondary: '#E7EAF0',
    colorSplit: '#E7EAF0',
    // 圆角二分：全局方角（表单/表格/格子 = rounded.none），药丸只由 Button 组件承担
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 16,
    // strong = 600（规格 body-strong/button 600；标题 400 常规字重由 css 片段钉住）
    fontWeightStrong: 600,
    // 零阴影（「avoids drop shadows entirely」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // abcNormal（专有族，禁外部加载）→ Inter 规格钦定替代 + 系统/web-safe 栈
    fontFamily:
      'Inter, system-ui, -apple-system, "Segoe UI", "Helvetica Neue", Helvetica, Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Select: { optionSelectedBg: 'rgba(3, 3, 3, 0.08)' },
    Button: {
      // 药丸是按钮唯一形态（「rounded.full 9999px — every primary button」）
      borderRadius: 9999,
      borderRadiusSM: 9999,
      borderRadiusLG: 9999,
      // 按钮字重 600（规格 typography.button 14px/600）
      fontWeight: 600,
      // 三态阴影全关（零阴影体系）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = hairline infill + 墨字（规格「featured tier 用 infill 而非彩边」语汇）
      itemSelectedBg: '#E7EAF0',
      itemSelectedColor: '#030303',
    },
    Table: {
      // 白底表头 + slate 标签 + hairline 分隔；行 hover = featured infill 减半一档（派生 #F4F6F9）
      headerBg: '#FFFFFF',
      headerColor: '#676F7B',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F4F6F9',
      borderColor: '#E7EAF0',
    },
    Tooltip: {
      // 浮层提示 = 墨面倒置（黑墨白字的编辑反白面）
      colorBgSpotlight: '#030303',
    },
  },
}
