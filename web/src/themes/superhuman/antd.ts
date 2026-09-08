import type { ThemeConfig } from 'antd'

/**
 * superhuman 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Superhuman 设计语言 = 三画布系统（靛蓝 hero / 白正文 / 深青收尾）在亮色
 * 应用面的投影：白画布 #FFFFFF + 暖墨 #292827（never pure black）+ 近黑靛蓝
 * #1B1938 作为主 CTA（button-primary-dark 圆角矩形 8px）；紫（violet-soft
 * #C9B4FA）与深青只落装饰/信息位，不进正文交互。字族 Super Sans VF 的
 * 替代 = Inter Variable（规格指定），子默认字重 460/540 是暖感签名。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/superhuman.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#1B1938，规格「primary surface and CTA color」)
 *   colorPrimaryActive      = --primary-active  (#0E0C1F primary-deep，规格按压态)
 *   colorPrimaryHover       = #322E56（派生：靛蓝提亮一阶，规格只给了按压态）
 *   colorInfo               = #155555（派生：info 取品牌深青 surface-teal-mid，
 *                                             三画布的第三 voice；规格无 info 色）
 *   colorInfoBg/colorInfoBorder = --info-soft 派生 teal 薄纱 + 同族描边
 *   colorSuccess/Warning/Error = 派生语义三色（规格无语义色段）：
 *     深青家族提亮 #0F7660 / 暖赭 #A16207 / 暖红 #B23E3E —— 均校准到暖墨画布
 *   colorText 四阶           = #292827 / #514F4C / #73706D / #9A9794
 *                             (ink/ink-mute/ink-faint 三级 + 中间插阶，规格 原文)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 白画布)
 *   colorBorder 双阶 + colorSplit = #E8E4DD hairline / #F0EEE9 派生浅暖
 *   borderRadius 8 / SM 6 / LG 12 = 规格 rounded.md（按钮签名）/ rounded.sm（输入）/ rounded.lg（卡）
 *   boxShadow / boxShadowSecondary = 规格 Elevation 1/2 的 box-shadow 原文
 *   fontWeightStrong 540    = 子默认字重签名（可变字体按 540 渲染，定字重回退就近取整）
 *   fontFamily              = --font-sans（Inter Variable，规格钦定的替代字族）
 */
export const superhumanAntdTheme: ThemeConfig = {
  token: {
    // 靛蓝近黑：主 CTA 填充（规格 button-primary-dark 原文）
    colorPrimary: '#1B1938',
    colorPrimaryHover: '#322E56', // 派生：靛蓝提亮一阶（规格只给按压态 primary-deep）
    colorPrimaryActive: '#0E0C1F', // primary-deep（规格 button-primary-dark-pressed）
    // info = 品牌深青（三画布的第三 voice；hero gradient stop #5A5772 同族偏绿蓝）
    colorInfo: '#155555',
    colorInfoBg: '#EEF5F3',    // 派生：teal 薄纱（与 --info-soft 同源）
    colorInfoBorder: '#C3DCD7', // 派生：teal 同族描边
    // 规格 link-on-light = 墨 + 持续下划线；后台动作链接取靛蓝保交互可供性（偏离注明）
    colorLink: '#1B1938',
    colorLinkHover: '#322E56',
    // 语义三色（派生：规格无语义段，全部校准到暖墨白画布）
    colorSuccess: '#0F7660', // 深青家族提亮档
    colorWarning: '#A16207', // 暖赭
    colorError: '#B23E3E',   // 暖红
    // 文字阶梯：ink #292827（暖墨，never pure black）/ 插阶 / ink-mute / ink-faint
    colorText: '#292827',
    colorTextSecondary: '#514F4C',
    colorTextTertiary: '#73706D',
    colorTextQuaternary: '#9A9794',
    // 白画布：页面、容器、浮层全白
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线：暖灰 hairline #E8E4DD 为主，弱线退到更浅暖调
    colorBorder: '#E8E4DD',
    colorBorderSecondary: '#F0EEE9',
    colorSplit: '#E8E4DD',
    // 圆角：8 = 按钮 rounded.md 签名；6 = 输入 rounded.sm；12 = 卡 rounded.lg
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    // 子默认字重 540（Super Sans VF 签名的 antd 等价）
    fontWeightStrong: 540,
    // 规格 Elevation 1/2 的 box-shadow 原文（subtle card lift / floating panels）
    boxShadow: '0 1px 3px rgba(0, 0, 0, 0.08)',
    boxShadowSecondary: '0 8px 24px rgba(0, 0, 0, 0.12)',
    // 派生：慢拍编辑节奏（「considered, slow-tempo」语言的时长等价）
    motionDurationFast: '0.18s',
    motionDurationMid: '0.24s',
    motionDurationSlow: '0.32s',
    // Super Sans VF 的钦定替代 Inter Variable（本地已装即按可变轴渲染）
    fontFamily:
      '"Inter Variable", Inter, system-ui, -apple-system, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮标签 16px/700（规格 typography.button-md）；CTA 平涂无阴影
      fontWeight: 700,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = canvas-soft 暖白微染 + 靛蓝墨
      itemSelectedBg: '#FAFAF8',
      itemSelectedColor: '#1B1938',
    },
    Table: {
      // canvas-soft 表头 + ink-mute 标签；行 hover = 暖白交替带（feature-row rhythm）
      headerBg: '#FAFAF8',
      headerColor: '#73706D',
      headerSplitColor: '#F0EEE9',
      rowHoverBg: '#FAFAF8',
      borderColor: '#F0EEE9',
    },
    Tooltip: {
      // tooltip = 靛蓝暗面白字（hero/nav 同源 register）
      colorBgSpotlight: '#1B1938',
    },
  },
}
