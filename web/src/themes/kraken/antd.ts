import type { ThemeConfig } from 'antd'

/**
 * kraken 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Kraken 设计语言（简洁可信的加密交易所）：
 * - 白底 #ffffff + Kraken Purple #7132f5 命令色（CTA/链接/品牌强调）；
 *   变体 Purple Dark #5741d8（描边/外描钮）、Purple Deep #5b1ecf（最深，
 *   按压态）、Purple Subtle rgba(133,91,251,.16)（选中薄纱底）；
 * - 近黑 #101114 正文 + 冷蓝灰中性阶（#484b5e / Cool Gray #686b82 /
 *   Silver Blue #9497a9 / Border Gray #dedee5）；
 * - 12px 按钮圆角 = 「rounded but not pill」的最大值（规格 Don't：禁药丸）；
 * - whisper 级细影：Subtle rgba(0,0,0,.03) 0 4px 24px / Micro rgba(16,24,40,.04) 0 1px 4px；
 * - 绿 #149e61 = success（16% 徽章底 + 深绿 #026b3f 字）；
 * - Kraken-Brand（display 700 负字距）+ Kraken-Product/IBM Plex Sans（UI）双字体。
 * 规格未给 warning/error 语义色与部分中性细分阶 → 按冷灰阶克制推导（见行内注释）。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/kraken.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#7132f5 Kraken Purple)
 *   colorPrimaryHover       = --link-hover      (#5741d8 Purple Dark)
 *   colorPrimaryActive      = --primary-active  (#5b1ecf Purple Deep)
 *   colorInfo/colorLink     = --info/--link     (#7132f5 紫 = CTA/链接/信息同源)
 *   colorInfoBg/colorInfoBorder = --info-soft/--login-notice-border
 *                                                (Purple Subtle 16% 薄纱，规格字面值)
 *   colorSuccess            = --success         (#149e61 Green)
 *   colorWarning/colorError = --warning/--error (#9c6a10/#b6443a 推导克制语义)
 *   colorText 四阶          = #101114 / #484b5e / #686b82 / #9497a9
 *                             (Near Black + 徽章字灰 + Cool Gray + Silver Blue)
 *   colorBgLayout/Container/Elevated = #ffffff  (White Primary surface)
 *   colorBorder             = --hairline        (#dedee5 Border Gray)
 *   boxShadow               = Micro 细影        (rgba(16,24,40,.04) 0 1px 4px)
 *   boxShadowSecondary      = Subtle 细影       (rgba(0,0,0,.03) 0 4px 24px)
 *   borderRadius 12         = 按钮签名圆角（rounded but not pill）
 *   borderRadiusLG 16       = 规格圆角阶最大值（大浮层）
 *   fontFamily              = --font-sans（Kraken-Product → IBM Plex Sans 栈）
 */
export const krakenAntdTheme: ThemeConfig = {
  token: {
    // Kraken Purple 一族：基 #7132f5 → hover Purple Dark #5741d8 → 按压 Purple Deep #5b1ecf
    colorPrimary: '#7132f5',
    colorPrimaryHover: '#5741d8',
    colorPrimaryActive: '#5b1ecf',
    // 紫 = CTA/链接/信息同源（Primary：CTA、brand accent、links）
    colorInfo: '#7132f5',
    colorLink: '#7132f5',
    colorLinkHover: '#5741d8',
    colorLinkActive: '#5b1ecf',
    // info Alert = Purple Subtle 薄纱（规格字面值 rgba(133,91,251,0.16)）
    colorInfoBg: 'rgba(133, 91, 251, 0.16)',
    colorInfoBorder: 'rgba(113, 50, 245, 0.28)',
    // 语义色：success = 规格 Green #149e61；warning/error = 推导克制语义（规格未给）
    colorSuccess: '#149e61',
    colorWarning: '#9c6a10',
    colorError: '#b6443a',
    // 文字阶梯：Near Black #101114 + 冷灰阶（#484b5e 来自规格 Neutral 徽章字色）
    colorText: '#101114',
    colorTextHeading: '#101114',
    colorTextSecondary: '#484b5e',
    colorTextTertiary: '#686b82',
    colorTextQuaternary: '#9497a9',
    // 白底三面：页面/容器/浮层全白，靠 Border Gray + whisper 影分层
    colorBgLayout: '#ffffff',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 边线：Border Gray #dedee5 主边 / 软分隔提亮半档（推导）
    colorBorder: '#dedee5',
    colorBorderSecondary: '#e8e8ed',
    colorSplit: '#e8e8ed',
    // 焦点环 = 紫 18%（Kraken Purple 的焦点语汇）
    controlOutline: 'rgba(113, 50, 245, 0.18)',
    // 圆角：按钮 12（签名最大值）/ 小件 8 / 大浮层 16（规格圆角阶 3/6/8/10/12/16）
    borderRadius: 12,
    borderRadiusSM: 8,
    borderRadiusLG: 16,
    // whisper 级细影：控件 Micro / 浮层 Subtle（字面值）
    boxShadow: '0 1px 4px rgba(16, 24, 40, 0.04)',
    boxShadowSecondary: '0 4px 24px rgba(0, 0, 0, 0.03)',
    // Kraken-Product → IBM Plex Sans 近似栈（禁外部加载；fallback 链）
    fontFamily:
      "'Kraken-Product', 'IBM Plex Sans', 'Helvetica Neue', Helvetica, Arial, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    fontFamilyCode: "'IBM Plex Mono', 'Cascadia Code', Consolas, monospace",
    // Feature Title 600（600–700 带，UI 层取 600）
    fontWeightStrong: 600,
  },
  components: {
    Button: {
      // 12px 圆角贯穿全部按钮尺寸（「rounded but not pill」；规格 Don't：禁药丸）
      borderRadius: 12,
      borderRadiusSM: 12,
      borderRadiusLG: 12,
      // 按钮字重 600（规格 Button 500–600 取上沿）
      fontWeight: 600,
      // whisper 体系：按钮不带投影
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = Secondary Gray：银蓝 8% 底 + 近黑字（规格字面值）
      defaultBg: 'rgba(148, 151, 169, 0.08)',
      defaultColor: '#101114',
      defaultBorderColor: 'transparent',
      defaultHoverBg: 'rgba(148, 151, 169, 0.14)',
      defaultHoverColor: '#101114',
      defaultHoverBorderColor: 'transparent',
    },
    Menu: {
      // 下拉当前项 = Purple Subtle 薄纱（规格「subtle button backgrounds」）
      itemSelectedBg: 'rgba(133, 91, 251, 0.16)',
      itemSelectedColor: '#5b1ecf',
    },
    Table: {
      // 白底表头 + Cool Gray 标签 + Border Gray 分隔；行 hover = 银蓝 8% 洗
      headerBg: '#ffffff',
      headerColor: '#686b82',
      headerSplitColor: '#dedee5',
      rowHoverBg: 'rgba(148, 151, 169, 0.08)',
      borderColor: '#dedee5',
    },
    Tooltip: {
      // 近黑 tooltip（Near Black #101114）
      colorBgSpotlight: '#101114',
    },
    Segmented: {
      trackBg: 'rgba(148, 151, 169, 0.08)',
      itemSelectedBg: '#ffffff',
    },
    Select: {
      optionSelectedBg: 'rgba(133, 91, 251, 0.16)',
    },
    Tag: {
      // 徽章 6px（规格 Success badge radius 6px）
      borderRadiusSM: 6,
    },
  },
}
