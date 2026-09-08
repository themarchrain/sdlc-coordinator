import type { ThemeConfig } from 'antd'

/**
 * hp 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * HP 设计语言（白纸企业-消费目录系统，亮色）：
 * - 纯白画布 #ffffff + 深墨 #1a1a1a 承载一切正文面；Cloud #f7f7f7 /
 *   Fog #e8e8e8 灰带做段落节奏；蓝是唯一信号；
 * - HP Electric Blue #024ad8 = 唯一 CTA 填充/链接/chevron 装饰/激活指示
 *   （每视口至多两处火焰）；按压/visited = Deep Navy #0e3191（规格原文）；
 *   hover 未记载 → 取同族亮阶 Bright Blue #296ef9（规格「暗板内亮蓝」变体挪用）；
 * - 文字阶梯：Ink #1a1a1a / Charcoal #3d3d3d（次级）/ Graphite #636363（小字），
 *   Steel #c2c2c2 只做暗板上的弱化与控件强边（白底上对比不足，不入文字阶）；
 * - 语义色：error = Bloom Deep #b3262b（规格 error/discount 色原文）；
 *   success = Storm Deep #356373（规格「neutral status accent」原文）；
 *   warning 为派生琥珀 #8a5a00（规格无 warning 色码，取白底可读的克制变体）；
 * - 双层圆角哲学：按钮/输入 4px「sharp」，卡/浮层 16px「soft」
 *   （Button 单独钉 4，防 borderRadiusLG 16 泄漏到大按钮）；
 * - 阴影只有 Soft Lift 0 2px 8px rgba(26,26,26,.08) 与
 *   Floating Modal 0 8px 24px rgba(26,26,26,.12)（规格 Elevation 表原文）；
 * - 字体 Forma DJR Micro（专有）→ 官方回退 Arial；展示字重恒 500
 *   （规格签名：hero 也不升 600/700）；按钮 label 600 大写 +0.7px 字距
 *   （大写与字距由 themes/css/hp.css 片段承担）。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/hp.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#024ad8 HP Electric Blue)
 *   colorPrimaryHover  = --link-hover     (#296ef9 Bright Blue，派生 hover)
 *   colorPrimaryActive = --primary-active (#0e3191 Deep Navy，规格 pressed)
 *   colorInfo          = --info           (#024ad8 蓝即唯一信号)
 *   colorSuccess       = --success        (#356373 Storm Deep)
 *   colorWarning       = --warning        (#8a5a00 派生琥珀)
 *   colorError         = --error          (#b3262b Bloom Deep)
 *   colorLink 三态      = --link/--link-hover/--primary-active（visited=Deep Navy 规格）
 *   colorText 四阶      = Ink/Charcoal/Graphite/Graphite (#1a1a1a/#3d3d3d/#636363)
 *   colorBgLayout      = --canvas         (#ffffff)
 *   colorBgContainer   = colorBgElevated = --canvas-card (#ffffff Paper)
 *   colorBorder        = Steel #c2c2c2（规格 text-input 默认边）/ Secondary #e8e8e8
 *   boxShadow          = Soft Lift        (0 2px 8px rgba(26,26,26,.08))
 *   boxShadowSecondary = Floating Modal   (0 8px 24px rgba(26,26,26,.12))
 *   fontFamily         = --font-sans（Forma DJR Micro → Arial 官方回退）
 */
export const hpAntdTheme: ThemeConfig = {
  token: {
    // HP Electric Blue：唯一信号；hover = 亮蓝变体，按压 = Deep Navy（规格 pressed 原文）
    colorPrimary: '#024ad8',
    colorPrimaryHover: '#296ef9',
    colorPrimaryActive: '#0e3191',
    // info = 蓝（蓝即唯一信号）；语义三色见类头注释
    colorInfo: '#024ad8',
    colorSuccess: '#356373',
    colorWarning: '#8a5a00',
    colorError: '#b3262b',
    // 链接：基蓝 / hover 亮蓝 / active=visited Deep Navy（规格 visited-link 原文）
    colorLink: '#024ad8',
    colorLinkHover: '#296ef9',
    colorLinkActive: '#0e3191',
    // 文字阶梯：Ink / Charcoal / Graphite（Steel 对比不足不入文字阶）
    colorText: '#1a1a1a',
    colorTextHeading: '#1a1a1a',
    colorTextSecondary: '#3d3d3d',
    colorTextTertiary: '#636363',
    colorTextQuaternary: '#636363',
    // 白纸体系：页面/容器/浮层全白（Paper = Canvas 同白，靠发丝线与 Soft Lift 分层）
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 边线双阶：Steel（控件/强调边）/ Fog（分隔）
    colorBorder: '#c2c2c2',
    colorBorderSecondary: '#e8e8e8',
    colorSplit: '#e8e8e8',
    // 规格 text-input-focused：聚焦换 1px Ink 边、无 halo（activeShadow 在 Input 组件钉 none）
    controlOutline: 'transparent',
    // 双层圆角：控件 4 sharp / 卡与浮层 16 soft（Button 单独钉 4 防 LG 泄漏）
    borderRadius: 4,
    borderRadiusSM: 4,
    borderRadiusLG: 16,
    // 展示字重恒 500（规格「resist the urge to bump weight at hero scale」）
    fontWeightStrong: 500,
    // 阴影只此两档：Soft Lift / Floating Modal（规格 Elevation 表字面值）
    boxShadow: '0 2px 8px rgba(26, 26, 26, 0.08)',
    boxShadowSecondary: '0 8px 24px rgba(26, 26, 26, 0.12)',
    // Forma DJR Micro（专有）→ 规格 fallback Arial，补 CJK 回退
    fontFamily:
      '"Forma DJR Micro", Arial, Helvetica, "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Cascadia Code", Consolas, monospace',
  },
  components: {
    Button: {
      // 按钮恒 4px sharp（规格 Don't：按钮不得超过 rounded.md 4px）
      borderRadius: 4,
      borderRadiusSM: 4,
      borderRadiusLG: 4,
      // button-md：14px/600/大写 +0.7px 字距（大写与字距由 css 片段承担）
      fontWeight: 600,
      // 按钮不带投影（Soft Lift 只归产品卡/定价列）
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = Cloud 微高亮 + Ink 墨（规格灰带节奏）
      itemSelectedBg: '#f7f7f7',
      itemSelectedColor: '#1a1a1a',
    },
    Table: {
      // 扁平表头：Cloud 灰带 + Charcoal 标签 + 透明分隔缝；行 hover = Cloud
      headerBg: '#f7f7f7',
      headerColor: '#3d3d3d',
      headerSplitColor: 'transparent',
      rowHoverBg: '#f7f7f7',
      rowSelectedBg: '#e8e8e8',
      rowSelectedHoverBg: '#e8e8e8',
      borderColor: '#e8e8e8',
    },
    Tooltip: { colorBgSpotlight: '#1a1a1a' }, // Ink 暗板反转（promo-strip-dark 语汇）
    Select: {
      // 选项选中 = Soft Blue #c9e0fc（规格「selection chips」原文用色）
      optionSelectedBg: '#c9e0fc',
      optionSelectedColor: '#1a1a1a',
    },
    Input: {
      // 规格 text-input：默认 1px Steel 边，hover 转 Charcoal，聚焦换 1px Ink 边、无 halo
      hoverBorderColor: '#3d3d3d',
      activeBorderColor: '#1a1a1a',
      activeShadow: 'none',
    },
    Segmented: {
      // category-tab-active：Ink 实底白字（规格 pill 子导航激活态原文）
      trackBg: '#f7f7f7',
      itemSelectedBg: '#1a1a1a',
      itemSelectedColor: '#ffffff',
    },
    Tag: { borderRadiusSM: 999 }, // 迷你徽标/过滤 chip = pill（rounded.pill）
    Pagination: { itemActiveBg: 'transparent' },
  },
}
