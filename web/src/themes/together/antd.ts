import type { ThemeConfig } from 'antd'

/**
 * together 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Together AI 设计语言 = AI 原生云的黑白带律：近黑 #010120 hero 带与纯白
 * 产品带交替，无中间灰（唯一软面 #EBEBEB = rgba(0,0,0,.08) 只做表头轨）；
 * 唯一黑 #000000 CTA（4px 微圆角矩形，规格 Don't「never a full pill」）承担
 * 全部转化；三色橙-品红-长春花渐变丝带是全部品牌装饰（只进 hero 尺度，
 * 不进 antd 控件）；几何 display sans（The Future → Inter 替代）500 字重、
 * 负字距；大写 mono（PP Neue Montreal Mono → JetBrains/Geist Mono 替代）
 * 只承载眉题/按钮标签/表头。规格明言不维护语义色板、「采用框架默认值是
 * 合适的」——语义三色直接取 antd 默认。algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/together.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#000000，规格黑 CTA pill/矩形)
 *   colorPrimaryActive      = --primary-active  (#010120 canvas-dark 藏青黑按压，推导)
 *   colorPrimaryHover       = #333333           (规格 no-hover 未提取；黑色抬升一档的中性推导)
 *   colorInfo/colorLink     = --info/--link     (info = 框架默认蓝；link = 墨，
 *                                                规格 inline link 属 display sans 墨色)
 *   colorSuccess/Warning/Error = antd 框架默认  (#52C41A / #FAAD14 / #FF4D4F，
 *                                                「adopting framework defaults」)
 *   colorText 四阶           = Ink #000000 / Body #959494 两律 + 禁用档 #B3B2B2（推导）
 *   colorBgLayout/Container/Elevated = --canvas (#FFFFFF)
 *   colorBorder 双阶 + colorSplit = #EBEBEB / #F2F2F2（rgba(0,0,0,.08/.05) 预混）
 *   borderRadius 4 / LG 8    = rounded.sm 规格正典 / rounded.md feature-tab 容器
 *   fontWeightStrong 500     = 规格「face never appears in bold」，全站无 700
 *   boxShadow                = Level 3 藏青软投影 rgba(1,1,32,.1) 0 4px 10px 原文
 *                              （规格 Don't：浅面卡不落软影，阴影只属浮层）
 *   fontFamily               = The Future → Inter 替代栈
 */
export const togetherAntdTheme: ThemeConfig = {
  token: {
    // 唯一黑 CTA：一个视口一颗黑钮（「One black pill per visible viewport」）
    colorPrimary: '#000000',
    // hover 为中性抬升推导（规格 no-hover 未提取）；按压沉入 canvas-dark 藏青黑
    colorPrimaryHover: '#333333',
    colorPrimaryActive: '#010120',
    // info = 框架默认蓝（无品牌语义色板，采用框架默认）
    colorInfo: '#1677FF',
    colorInfoBg: '#E6F4FF',
    colorInfoBorder: '#91CAFF',
    // link = 墨：规格 inline link 属 display sans（墨色 + 激活下划线）
    colorLink: '#000000',
    colorLinkHover: '#333333',
    colorLinkActive: '#010120',
    // 语义三色 = antd 框架默认（规格明示采用）
    colorSuccess: '#52C41A',
    colorWarning: '#FAAD14',
    colorError: '#FF4D4F',
    // 文字两律：Ink #000000 / Body #959494（规格：无独立 mute 档）+ 禁用档推导
    colorText: '#000000',
    colorTextSecondary: '#959494',
    colorTextTertiary: '#959494',
    colorTextQuaternary: '#B3B2B2',
    // 白带：页面、容器、浮层全白，发丝线承担海拔
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线：rgba(0,0,0,.08) 预混 = #EBEBEB（规格 badge/input/outline borderColor 原值）
    colorBorder: '#EBEBEB',
    colorBorderSecondary: '#F2F2F2',
    colorSplit: '#EBEBEB',
    // 圆角：4 = rounded.sm 正典（卡/钮/徽章/表行）；8 = rounded.md feature-tab 容器
    borderRadius: 4,
    borderRadiusSM: 4,
    borderRadiusLG: 8,
    // 字重两律 400/500：全站无 bold（「never in bold (700)」）
    fontWeightStrong: 500,
    // 海拔：藏青染色软投影只给浮层（规格 Level 3 原文 rgba(1,1,32,.1) 0 4px 10px）
    boxShadow: '0 4px 10px 0 rgba(1, 1, 32, .1)',
    boxShadowSecondary: '0 4px 10px 0 rgba(1, 1, 32, .1)',
    // The Future 专有 → 规格 documented 替代 Inter（ss01 字符变体见 css 片段）
    fontFamily:
      '"The Future", Inter, "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮 = 大写 mono 标签（mono-caps-button 16px/500；字族/大写见 css 片段）
      fontWeight: 500,
      // 三态阴影全关：浅面 CTA 平面（规格 Don't「soft shadows belong only to the orb」）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = 软面 #EBEBEB + 墨字（badge-neutral / toggle 轨同面）
      itemSelectedBg: '#EBEBEB',
      itemSelectedColor: '#000000',
    },
    Table: {
      // 表头 = 软面轨 #EBEBEB + Body 灰大写 mono（data-table-header 原文；字态见片段）
      headerBg: '#EBEBEB',
      headerColor: '#959494',
      headerSplitColor: 'transparent',
      rowHoverBg: 'rgba(1, 1, 32, .03)',
      borderColor: '#EBEBEB',
    },
    Tooltip: {
      colorBgSpotlight: '#010120',
    },
  },
}
