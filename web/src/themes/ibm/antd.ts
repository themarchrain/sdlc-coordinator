import type { ThemeConfig } from 'antd'

/**
 * ibm 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * IBM 设计语言 = Carbon Design System：白画布 + 炭墨 #161616 + 唯一强调色
 * IBM Blue #0F62FE，方正几何（圆角一律 0，连 4px 都会破坏 Carbon 味），
 * 层级靠 1px 发丝线与灰面切换而非阴影（两级阴影全关），字重只有
 * 300（展示）/ 400（正文）/ 600（强调），交互节奏取 Carbon productive
 * 时长（fast-01 70ms / moderate 150ms / slow-01 240ms）。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/ibm.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#0F62FE，规格「single brand accent」)
 *   colorPrimaryHover/Active = blue-hover/blue-80 (#0050E6 / #002D9C，规格按钮三态原文)
 *   colorInfo/colorLink     = --info/--link     (#0F62FE，规格「Info Blue 与 primary 相同」)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline (#EDF5FF blue-10 薄纱 + #E0E0E0 发丝线)
 *   colorSuccess/Warning/Error = Carbon green-50 #24A148 / yellow-30 压深 #96780E /
 *                             red-60 #DA1E28（原文；yellow-30 太亮，
 *                             antd 文字位压深到可读档，图形位仍直用 #F1C21B）
 *   colorText 四阶           = #161616 / #393939 / #525252 / #8C8C8C
 *                             (规格 ink/ink-muted/ink-subtle 三级 + Carbon gray-80 插阶)
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 白画布)
 *   colorBorder 双阶 + colorSplit = #E0E0E0 / #F4F4F4（规格 hairline + surface-1）
 *   borderRadius 0          = 「flat 0px corners」
 *   fontFamily              = --font-sans（IBM Plex Sans + 规格 Helvetica Neue 回退）
 */
export const ibmAntdTheme: ThemeConfig = {
  token: {
    // IBM Blue：唯一品牌色
    colorPrimary: '#0F62FE',
    colorPrimaryHover: '#0050E6', // blue-hover（规格：主按钮 hover 态）
    colorPrimaryActive: '#002D9C', // blue-80（规格：主按钮按压态）
    // info / link 与 primary 同蓝：蓝承载全部链接、信息徽标与 CTA（规格「no second brand color」）
    colorInfo: '#0F62FE',
    colorLink: '#0F62FE',
    colorLinkHover: '#0043CE', // blue-60（规格：hovered link state）
    // info 形态 = blue-10 薄纱 + gray-20 发丝线（规格：IBM Blue 不做卡片底）
    colorInfoBg: '#EDF5FF',
    colorInfoBorder: '#E0E0E0',
    // 语义三色（原文）；warning 为 yellow-30 #F1C21B 压深到文字可读档
    colorSuccess: '#24A148',
    colorWarning: '#96780E',
    colorError: '#DA1E28',
    // 文字阶梯：ink #161616 / gray-80 插阶 / ink-muted / ink-subtle（原文）
    colorText: '#161616',
    colorTextSecondary: '#393939',
    colorTextTertiary: '#525252',
    colorTextQuaternary: '#8C8C8C',
    // 白画布：页面、容器、浮层全白，靠发丝线与灰面分层
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线：hairline #E0E0E0 为主，弱线退到 surface-1 #F4F4F4
    colorBorder: '#E0E0E0',
    colorBorderSecondary: '#F4F4F4',
    colorSplit: '#E0E0E0',
    // 方正几何：默认/小件/大面全部 0 圆角（「commits to flat 0px corners」）
    borderRadius: 0,
    borderRadiusSM: 0,
    borderRadiusLG: 0,
    // Carbon 强调 = 600（body-emphasis）；展示位 300 轻字重由 css 片段处理
    fontWeightStrong: 600,
    // 阴影全关：层级靠 surface change + 1px 发丝线（「resists drop shadows」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // Carbon productive 动效：fast-01 70ms / moderate 150ms / slow-01 240ms
    motionDurationFast: '0.07s',
    motionDurationMid: '0.15s',
    motionDurationSlow: '0.24s',
    // IBM Plex Sans（SIL OFL 开源；本仓不外载字体，本地已装即渲染），回退按规格 Helvetica Neue/Arial
    fontFamily:
      '"IBM Plex Sans", "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮标签 14px/400：Carbon 按钮不加重（规格 typography.button）；三态阴影全关
      fontWeight: 400,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = surface-1 灰面提升 + 炭墨（规格 Elevation 2「surface lift」）
      itemSelectedBg: '#F4F4F4',
      itemSelectedColor: '#161616',
    },
    Table: {
      // surface-1 表头 + ink-muted 标签 + gray-20 行线；行 hover = surface lift
      headerBg: '#F4F4F4',
      headerColor: '#525252',
      headerSplitColor: '#E0E0E0',
      rowHoverBg: '#F4F4F4',
      borderColor: '#E0E0E0',
    },
    Tooltip: {
      // Carbon tooltip = 炭墨反转面白字（规格 inverse 语汇）
      colorBgSpotlight: '#161616',
    },
  },
}
