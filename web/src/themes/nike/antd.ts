import type { ThemeConfig } from 'antd'

/**
 * nike 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Nike 设计语言 = 摄影优先 + 极端排版对比：纯黑/纯白/单一软灰的近单色 chrome，
 * 唯一"颜色"是 Nike Black #111111（主 CTA / 激活 chip / swatch dot 全是它）；
 * 色彩只留给信号（sale 红、success 绿、info 蓝），从不做装饰。
 * 形态：CTA 全 pill、容器全方角、零投影、1px 发丝线分隔、字重只有 400/500。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/nike.css 的同名令牌手工保持同源：
 *   colorPrimary/Hover/Active = --primary(-active)   (#111111，规格「the primary pill is true #111111」，
 *                                                      hover/active 同钉纯黑，按压反馈交给 tap-collapse 片段)
 *   colorInfo               = --info                (#1151ff，member-experience 信息的 callout 信号色)
 *   colorInfoBg/Border      = --info-soft/--hairline-strong
 *                                                   (#F0F4FF 信息薄纱 + #CACACB 发丝线，钉住克制形态；
 *                                                    先例：cursor/antd.ts 40-46、tesla/antd.ts)
 *   colorLink 三态          = --ink                 (#111111，规格 link-md = 墨色下划线内链，无蓝色正文链)
 *   colorSuccess/Warning/Error = 语义信号           (#007d48 / #9A6700 派生 / #D30005=sale)
 *   colorText 四阶          = ink/charcoal/mute/stone (#111111 / #39393B / #707072 / #9E9EA0，规格 文字五级原文)
 *   colorBgLayout/Container/Elevated = --canvas 家族 (#FFFFFF 纯白画布，靠发丝线分隔)
 *   colorBorder 双阶 + colorSplit = hairline 双色    (#CACACB 控件边 / #E5E5E5 分隔线，规格 原文)
 *   borderRadius 0/0/0      = 容器全 rounded.none（按钮的 pill 在 Button 组件层覆写）
 *   fontWeightStrong 500    = 全站只有 400/500（规格 字阶全部 400/500）
 *   boxShadow 两级 none     = 零投影体系（规格「no drop-shadow elevation at all」）
 *   fontFamily              = --font-sans（Helvetica Now 的系统/web-safe 栈近似，禁外部字体）
 */
export const nikeAntdTheme: ThemeConfig = {
  token: {
    // Nike Black：唯一主色。主 CTA、激活 chip、swatch dot 全是它；hover/active 保持纯黑
    // （规格 Don't：「Don't replace ink with a near-black gray for a CTA」），反馈交给按压片段
    colorPrimary: '#111111',
    colorPrimaryHover: '#111111',
    colorPrimaryActive: '#111111',
    // info = 规格 member-experience 信息信号蓝；Alert 底钉「蓝薄纱 + 发丝线」而非 antd 灰蒙派生
    colorInfo: '#1151ff',
    colorInfoBg: '#f0f4ff',
    colorInfoBorder: '#cacacb',
    // 链接 = 墨色（规格 link-md：下划线内链、正文级墨色，hover 靠下划线片段表达）
    colorLink: '#111111',
    colorLinkHover: '#111111',
    colorLinkActive: '#111111',
    // 语义三色：success/sale 为规格原值；warning 规格未给 → 克制琥珀（无橙色系语言，见 slots 注）
    colorSuccess: '#007d48',
    colorWarning: '#9a6700',
    colorError: '#d30005',
    // 文字阶梯：ink / charcoal / mute / stone（五级坍缩为 antd 四阶）
    colorText: '#111111',
    colorTextSecondary: '#39393b',
    colorTextTertiary: '#707072',
    colorTextQuaternary: '#9e9ea0',
    // 纯白画布：页面/容器/浮层全白，深度只靠发丝线（规格 Elevation「Flat 是主导处理」）
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 边线双阶：hairline #CACACB（控件边）/ hairline-soft #E5E5E5（分隔线）
    colorBorder: '#cacacb',
    colorBorderSecondary: '#e5e5e5',
    colorSplit: '#e5e5e5',
    // 容器全方角（规格 rounded.none：卡片/导航/页脚全部 0）；pill 词汇只在 Button/Tag 层
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 0,
    // 字重克制：只有 400/500，无 bold（规格 字阶全 400/500）
    fontWeightStrong: 500,
    // 零投影体系（规格 Don't：「Don't introduce drop shadows or card elevation」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Helvetica Now 的系统/web-safe 栈近似（定制族，禁外部加载）
    fontFamily:
      '"Helvetica Now Text", "Helvetica Neue", Helvetica, "Segoe UI", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 「there are no sharp-cornered buttons in the system」：三档尺寸全部 pill；
      // 字重 500 = button-md（Helvetica Now Text Medium）；三态阴影全关（零投影）
      fontWeight: 500,
      borderRadius: 9999,
      borderRadiusSM: 9999,
      borderRadiusLG: 9999,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Input: {
      // 焦点 = 2px 墨色边（search-pill-focused 语言），无 antd 蓝色光晕
      activeShadow: 'none',
    },
    Tag: {
      // badge-promo / filter-chip = pill chip 语法
      borderRadiusSM: 9999,
    },
    Menu: {
      // 选中项整体反白：filter-chip-active「the chip flips fully inverted. No middle state.」
      itemSelectedBg: '#111111',
      itemSelectedColor: '#ffffff',
    },
    Select: {
      // 下拉选项选中 = soft-cloud 软面 + 墨字（button-secondary 语言，弱于 Menu 的整体反白）
      optionSelectedBg: '#f5f5f5',
    },
    Table: {
      // 白底表头 + mute 标签（caption 级）+ 透明分隔缝；行 hover = soft-cloud
      headerBg: '#ffffff',
      headerColor: '#707072',
      headerSplitColor: 'transparent',
      rowHoverBg: '#f5f5f5',
      borderColor: '#e5e5e5',
    },
    Tooltip: {
      colorBgSpotlight: '#111111',
    },
  },
}
