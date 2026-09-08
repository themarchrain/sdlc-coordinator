import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * bugatti 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格令牌表与正文（字面 hex，禁 var()）。
 *
 * Bugatti 设计语言（纯黑极简奢侈品）：
 * - 画布 #000000（规格「Pure black canvas … does not have a light mode」）；
 * - 白即唯一品牌色（规格 primary #ffffff）；唯一非单色 = 冰蓝链接 #c3d9f3
 *   （「essentially the entire chromatic vocabulary outside black-and-white」）；
 * - 表面阶梯 #000000 → #0d0d0d（表/密数据）→ #141414（卡）→ #1f1f1f（嵌套/浮层）；
 * - 圆角二值：全局 0px，按钮药丸 999px（「rectangular for everything
 *   except buttons」——Button 组件单独钉药丸，其余组件全部方角）；
 * - 全站字重 400（「The system NEVER uses bold weight」→
 *   fontWeightStrong 压到 400，强调只靠字号/字距/大小写/字族对比）；
 * - 零阴影零渐变（「no shadows, no glassmorphism, no gradients」）；
 * - 语义：warning #d4a017 / success #5fa657 规格直译；error 规格缺位 → #c0584f
 *   （推导：与 success #5fa657 同明度的去饱和红砖，呼应规格 muted 语义克制，
 *   暗底 ≥4:1 描线对比，取舍详见 slots.ts）；
 * - 焦点一律白（规格 text-input「Focus thickens the bottom border to white」），
 *   冰蓝只留链接，不装饰 focus。
 * algorithm = darkAlgorithm（照 spotify 暗色范例派生填充/禁用态等中间色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/bugatti.css 的同名令牌手工保持同源：
 *   colorPrimary        = --primary         (#ffffff，白即品牌主色)
 *   colorPrimaryHover   = --primary         (#ffffff，白已到顶；按钮透明药丸形态由 css 片段承担)
 *   colorPrimaryActive  = --primary-active  (#e6e6e6 = 规格 body-strong 反相按压阶)
 *   colorInfo/colorLink = --info/--link     (#c3d9f3 冰蓝)
 *   colorText 四阶       = #ffffff / #cccccc / #999999 / #666666（规格文字四级原文）
 *   colorBgLayout/Container/Elevated = #000000 / #141414 / #1f1f1f（提升阶梯）
 *   colorBorder 双阶 + colorSplit = #3a3a3a hairline-strong / #262626 hairline（规格边线双色）
 *   borderRadius 0      = 规格「no 4px, no 8px, no 12px in between」
 *   fontFamily          = Bugatti Display 的系统/web-safe 栈近似（禁外部字体）
 */
export const bugattiAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 白即品牌主色（Primary：「The single brand color. White type and white CTA outlines」）
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#ffffff',
    colorPrimaryActive: '#e6e6e6',
    // 冰蓝 = 唯一非单色，只承担 info 与链接（Link）
    colorInfo: '#c3d9f3',
    colorInfoBg: 'rgba(195, 217, 243, 0.1)',
    colorInfoBorder: 'rgba(195, 217, 243, 0.35)',
    colorLink: '#c3d9f3',
    colorLinkHover: '#ffffff',
    colorLinkActive: '#ffffff',
    // 语义三色：warning/success 规格直译；error 推导（见 slots.ts 注）
    colorSuccess: '#5fa657',
    colorWarning: '#d4a017',
    colorError: '#c0584f',
    // 文字阶梯：on-dark 白 / body #cccccc / muted #999999 / muted-soft #666666（Text 四级原文）
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#cccccc',
    colorTextTertiary: '#999999',
    colorTextQuaternary: '#666666',
    // 白底黑字（规格 on-primary #000000；白主色上的文字一律近黑）
    colorTextLightSolid: '#000000',
    // 表面阶梯：画布纯黑 / 容器 #141414 卡 / 浮层 #1f1f1f 嵌套提升阶
    colorBgBase: '#000000',
    colorBgLayout: '#000000',
    colorBgContainer: '#141414',
    colorBgElevated: '#1f1f1f',
    colorBgSpotlight: '#1f1f1f',
    // 边线双阶：hairline-strong #3a3a3a（控件边/输入底线）/ hairline #262626（分隔线）
    colorBorder: '#3a3a3a',
    colorBorderSecondary: '#262626',
    colorSplit: '#262626',
    // 焦点 = 白（规格输入 focus「thickens the bottom border to white」；冰蓝不装饰 focus）
    controlOutline: 'rgba(255, 255, 255, 0.4)',
    // 圆角二值：全局方角 0（按钮药丸在 Button 组件上单独钉 999）
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 0,
    // 全站无 bold：fontWeightStrong 压到 400（「Weight restraint」）
    fontWeightStrong: 400,
    // 零阴影（depth comes entirely from photography + 明度对比）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // 字体：三体分立的系统近似（禁外部加载）——UI 控件走 Display 栈；
    // 正文衬线（Text Regular）与按钮等宽（Monospace）由 css 令牌/片段分担
    fontFamily:
      "'Bugatti Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif",
    fontFamilyCode:
      "'Bugatti Monospace', ui-monospace, 'SF Mono', 'Cascadia Mono', 'Cascadia Code', Consolas, monospace",
  },
  components: {
    Button: {
      // 药丸是唯一圆角元素（rounded.pill 9999px 只给按钮）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 按钮字重 400（规格 typography.button 400；「never bold」）
      fontWeight: 400,
      // 白主色上的文字 = 近黑（按压反相态白底黑字）
      primaryColor: '#000000',
      // 默认按钮 = 透明底 + 1px 白描边（规格 button-primary「transparent + outline」按钮版）
      defaultBg: 'transparent',
      defaultColor: '#ffffff',
      defaultBorderColor: '#ffffff',
      defaultHoverBg: '#ffffff',
      defaultHoverColor: '#000000',
      defaultHoverBorderColor: '#ffffff',
      defaultActiveBg: '#ffffff',
      defaultActiveColor: '#000000',
      defaultActiveBorderColor: '#ffffff',
      // 按钮不带投影（零阴影纪律）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#000000' },
    Steps: { colorWhite: '#000000' },
    Menu: {
      // 当前项 = 白纱微高亮 + 白字（规格导航透明无 chrome，激活靠字色）
      itemSelectedBg: 'rgba(255, 255, 255, 0.06)',
      itemSelectedColor: '#ffffff',
    },
    Table: {
      // 扁平表头：surface-soft #0d0d0d（规格「spec table rows and dense data sections」）+ muted 字
      headerBg: '#0d0d0d',
      headerColor: '#999999',
      headerSplitColor: 'transparent',
      rowHoverBg: 'rgba(255, 255, 255, 0.04)',
      rowSelectedBg: 'rgba(255, 255, 255, 0.06)',
      rowSelectedHoverBg: 'rgba(255, 255, 255, 0.08)',
      fixedHeaderSortActiveBg: 'transparent',
      borderColor: '#262626',
    },
    Modal: { contentBg: '#141414', headerBg: '#141414', footerBg: '#141414' },
    Drawer: { colorBgElevated: '#141414' },
    Tooltip: { colorBgSpotlight: '#1f1f1f' },
    Popover: { colorBgElevated: '#1f1f1f' },
    Select: {
      // 输入 = 透明底（规格 text-input「Background transparent」）
      colorBgContainer: 'transparent',
      optionSelectedBg: 'rgba(255, 255, 255, 0.06)',
      optionSelectedColor: '#ffffff',
    },
    Input: { colorBgContainer: 'transparent' },
    InputNumber: { colorBgContainer: 'transparent' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#0d0d0d', itemSelectedBg: '#1f1f1f' },
    Tag: { borderRadiusSM: 0 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
