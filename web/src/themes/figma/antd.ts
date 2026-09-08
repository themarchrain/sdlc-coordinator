import type { ThemeConfig } from 'antd'

/**
 * figma 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Figma 设计语言 = 黑白编辑框架 + 巨型粉彩便利贴色块：chrome 全单色
 * （primary #000000 / canvas #FFFFFF），唯一强调色是 promo 单发 magenta #FF3D8B
 * 与 7 色 block 家族（lime/lilac/cream/mint/pink/coral/navy）。
 * CTA 只有药丸一种形状（pill 50px，由 themes/css/figma.css 专属片段施加于
 * .ant-btn——token 层 8px 是输入件 rounded.md，药丸专属按钮）；
 * 卡片 = 发丝线描边而非阴影；字重走 figmaSans 细粒度可变轴
 * （320–340 正文 / 480 按钮 / 540 标题 / 700 卡题）。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/figma.css 的同名令牌手工保持同源：
 *   colorPrimary        = --primary       (#000000，规格「system primary」)
 *   colorPrimaryActive  = #000000         (规格 button-primary-pressed：按压同面，
 *                                          靠微缩放而非压深)
 *   colorPrimaryHover   = #262626         (派生：规格未给 hover，按「黑不压深为灰」
 *                                          取最小可见一档)
 *   colorInfo           = --info          (#1F1D3D block-navy，唯一在调色板内的深色)
 *   colorInfoBg/colorInfoBorder = lilac 薄纱 (#F3EFFD + rgba(197,176,244,.55))
 *   colorLink           = --link          (#000000，规格 tertiary text-link = ink)
 *   colorSuccess        = #1EA64A         (规格 semantic-success 比对表对勾)
 *   colorWarning/Error  = #B45309/#DC2626 (派生：规格无 warning/error 色，取标准
 *                                          语义的克制饱和度并在 slots 注明)
 *   colorText 两阶      = #000000/#000000 (规格「无中灰正文，层级靠字重」；
 *                                          字重分层由 figma.css 片段承担)
 *   colorText 三四阶    = 黑系 alpha      (占位/禁用角色用透明度而非灰——
 *                                          「黑只在透明度维度让步」)
 *   colorBgLayout/Container/Elevated = --canvas (#FFFFFF 纯白画布)
 *   colorBorder 双阶 + colorSplit = hairline/hairline-soft (#E6E6E6/#F1F1F1)
 *   borderRadius 8      = 规格 rounded.md（表单输入/列表项）
 *   borderRadiusLG 24   = 规格 rounded.lg（pricing 卡 / 色块 section）
 *   fontWeightStrong 540 = 规格 headline 540（可变字重轴）
 *   boxShadow           = 0 4px 16px rgba(0,0,0,.06)（规格 Level 2 浮层字面值）
 *   fontFamily          = Inter 栈（figmaSans 定制族禁外部加载，规格 明示
 *                          Inter/Geist 为替身）
 */
export const figmaAntdTheme: ThemeConfig = {
  token: {
    // 主 CTA = 纯黑药丸（规格「the primary action across the entire site」）
    colorPrimary: '#000000',
    colorPrimaryHover: '#262626',
    colorPrimaryActive: '#000000',
    // info 家族挂 block-navy（调色板内唯一深色，深靛蓝）
    colorInfo: '#1F1D3D',
    colorInfoBg: '#F3EFFD',
    colorInfoBorder: 'rgba(197, 176, 244, .55)',
    // 链接 = 墨（规格 button-tertiary-text / link 均为 ink）
    colorLink: '#000000',
    colorLinkHover: '#262626',
    // 语义（success 原文；warning/error 派生见 JSDoc）
    colorSuccess: '#1EA64A',
    colorWarning: '#B45309',
    colorError: '#DC2626',
    // 文字阶梯：正文两级全黑（层级靠字重）；占位/禁用 = 黑系透明度
    colorText: '#000000',
    colorTextSecondary: '#000000',
    colorTextTertiary: 'rgba(0, 0, 0, .56)',
    colorTextQuaternary: 'rgba(0, 0, 0, .42)',
    // 纯白画布：页面/容器/浮层全白，靠发丝线与色块分层
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline（输入/卡描边）/ hairline-soft（表行分隔）
    colorBorder: '#E6E6E6',
    colorBorderSecondary: '#F1F1F1',
    colorSplit: '#F1F1F1',
    // 圆角：8 为输入/列表默认（rounded.md）；24 给大面（rounded.lg）；
    // 按钮药丸 50px 由 figma.css 片段施加（token 无法只对按钮生效）
    borderRadius: 8,
    borderRadiusSM: 6,
    borderRadiusLG: 24,
    // figmaSans 可变轴：标题 540（规格 headline 字重）
    fontWeightStrong: 540,
    // 阴影极少：Level 2 字面值只给浮层（色块替代传统海拔）
    boxShadow: '0 4px 16px rgba(0, 0, 0, .06)',
    boxShadowSecondary: '0 8px 24px rgba(0, 0, 0, .08)',
    // figmaSans 为 Figma 定制族（禁外部加载）→ 规格 明示 Inter 为替身
    fontFamily:
      'Inter, "SF Pro Display", system-ui, Helvetica, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 480（规格 typography.button 20px/480）；药丸无投影
      fontWeight: 480,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = 黑面白字（规格 pricing-tab-selected「selected = primary surface」）
      itemSelectedBg: '#000000',
      itemSelectedColor: '#FFFFFF',
    },
    Table: {
      // 白底表头 + 黑标签 + hairline-soft 分隔；行 hover = surface-soft
      headerBg: '#FFFFFF',
      headerColor: '#000000',
      headerSplitColor: '#F1F1F1',
      rowHoverBg: '#F7F7F5',
      borderColor: '#F1F1F1',
    },
    Tooltip: {
      // 反转面 = 纯黑（marquee strip 同面）
      colorBgSpotlight: '#000000',
    },
  },
}
