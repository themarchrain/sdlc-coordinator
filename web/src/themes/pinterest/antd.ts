import type { ThemeConfig } from 'antd'

/**
 * pinterest 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Pinterest 设计语言 = 摄影优先的发现系统：让开照片的路。暖奶油中性 chrome
 * （surface-card #F6F6F3 / surface-soft #FBFBF9）+ 纯白画布，单一饱和红
 * Pinterest Red #E60023 只承担主 CTA / 激活 tab 指示 / 品牌字（从不装饰）；
 * 双圆角语汇：16px（rounded.md 按钮/输入/卡）+ 32px（rounded.lg 大卡/模态）
 * + 药丸（搜索条/chip），永无硬角；卡面全平（全系统唯一阴影 = 模态 16px ambient）。
 * 字体 Pin Sans（专有族，禁外部加载）→ Inter 规格钦定替代；
 * 层级靠字重（400→600→700）而非色染。algorithm 未指定 = antd 默认算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/pinterest.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#E60023，规格「the brand red is precise」)
 *   colorPrimaryActive      = --primary-active  (#CC001F primary-pressed 规格字面值)
 *   colorPrimaryHover       = 派生 #E91A39（规格只记按压更深 → hover 向白 10% 提亮）
 *   colorInfo               = --info            (#435EE5 focus-outer，规格唯一 chrome 蓝)
 *   colorLink               = --link            (#211922 ink-soft，规格 link-inline 原文)
 *   colorInfoBg/colorInfoBorder = --info-soft/派生 (#EEF1FD 蓝薄纱 + #C7D0F5 淡边)
 *   colorSuccess            = #103C25（success-deep 规格字面值）
 *   colorWarning            = #8C651A（派生：规格无警示色 → 暖金与奶油中性同调）
 *   colorError              = --error           (#9E0A0A，规格「Error」原文)
 *   colorText 四阶          = Ink/Body/Mute/Ash (#000000/#33332E/#62625B/#91918C 规格原文)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 纯白画布)
 *   colorBorder 双阶 + colorSplit = #DADAD3 hairline / #E5E5E0 hairline-soft（规格字面值）
 *   borderRadius 16         = rounded.md（按钮/输入/卡的支配圆角）
 *   borderRadiusLG 32       = rounded.lg（大卡/模态；规格「md 与 lg 之间无第三值」）
 *   boxShadow/Secondary     = 模态 16px ambient（全系统唯一阴影，借给浮层）
 *   fontFamily              = --font-sans（Pin Sans → Inter 规格钦定替代）
 */
export const pinterestAntdTheme: ThemeConfig = {
  token: {
    // Pinterest Red：唯一高饱和色，只给主 CTA / 激活指示 / 品牌
    colorPrimary: '#E60023',
    // 按压 = primary-pressed 规格字面值；hover 为派生（规格未记 → 向白 10% 提亮）
    colorPrimaryHover: '#E91A39',
    colorPrimaryActive: '#CC001F',
    // info = focus-outer 蓝（规格唯一 chrome 蓝）；链接 = ink-soft 暖近黑（link-inline 原文），
    // hover 转 ink（ink-soft 只用于行内链，不外借 —— 规格 Don't）
    colorInfo: '#435EE5',
    colorLink: '#211922',
    colorLinkHover: '#000000',
    colorLinkActive: '#000000',
    // info Alert = 焦点蓝 8% 薄纱 + 淡蓝边（派生）
    colorInfoBg: '#EEF1FD',
    colorInfoBorder: '#C7D0F5',
    // 语义三色：success = success-deep 规格字面值；error = 规格 Error 原文；
    // warning 规格未给 → 派生暖金（与奶油中性同调）
    colorSuccess: '#103C25',
    colorWarning: '#8C651A',
    colorError: '#9E0A0A',
    // 文字阶梯：Ink / Body / Mute / Ash（「Text」四级原文）
    colorText: '#000000',
    colorTextSecondary: '#33332E',
    colorTextTertiary: '#62625B',
    colorTextQuaternary: '#91918C',
    // 纯白画布：页面/容器/浮层全白（surface-elevated #FFFFFF），奶油只做卡面/内嵌
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline #DADAD3（行分隔/列规）/ hairline-soft #E5E5E0（轻分隔，规格字面值）
    colorBorder: '#DADAD3',
    colorBorderSecondary: '#E5E5E0',
    colorSplit: '#E5E5E0',
    // 双圆角语汇：16 支配（按钮/输入/卡）；32 大卡；SM 8（规格 rounded.sm 罕用档）
    borderRadius: 16,
    borderRadiusSM: 8,
    borderRadiusLG: 32,
    // strong = 600（heading-lg/body-strong 600；700 留给按钮/表头/大数字，见 css 片段）
    fontWeightStrong: 600,
    // 全系统唯一阴影 = 模态 16px ambient（卡面全平）；借给 antd 浮层
    boxShadow: '0 4px 16px rgba(0, 0, 0, 0.14)',
    boxShadowSecondary: '0 4px 16px rgba(0, 0, 0, 0.14)',
    // Pin Sans（专有族，禁外部加载）→ Inter 规格钦定替代 + 规格回退链；附 CJK
    fontFamily:
      'Inter, -apple-system, system-ui, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 700（规格 typography.button-md 14px/700）；三态阴影全关（卡面全平）
      fontWeight: 700,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = surface-card 奶油面 + 红字（active-tab indicator 是红的合法场景）
      itemSelectedBg: '#F6F6F3',
      itemSelectedColor: '#E60023',
    },
    Table: {
      // 表头 = surface-card 奶油面 + body 正文色 700（规格 body-sm-strong「table-header text」）
      headerBg: '#F6F6F3',
      headerColor: '#33332E',
      headerSplitColor: 'transparent',
      rowHoverBg: '#FBFBF9',
      borderColor: '#E5E5E0',
    },
    Tooltip: {
      // 浮层提示 = surface-dark 暖近黑（比纯黑暖一档的反白面）
      colorBgSpotlight: '#262622',
    },
  },
}
