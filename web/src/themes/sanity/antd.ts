import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * sanity 主题的 antd ThemeConfig —— 规格驱动，
 * 无参考实现，色值全部来自规格正文（字面 hex，禁 var()）。
 *
 * Sanity 设计语言（暗色优先的编辑部风，dark-first 文档实证 canvas #0b0b0b）：
 * - 画布阶 #0b0b0b → 卡/面板 #212121（canvas-soft 单步提升，深色卡 1px #353535 边）；
 * - 主 CTA = 白色药丸 + 黑字（规格 button-primary：canvas-light 底 ink 字）——
 *   黑是系统里用量最大的动作色，coral-red #f36458 是稀缺强调（每屏至多一次），
 *   因此 colorPrimary 取白而非 coral；
 * - 文字阶梯：标题白 #ffffff / 正文 ash #b9b9b9 / mute #797979（mono 眉题灰）；
 * - 链接：暗底行内链 = link-blue-soft #55beff，聚焦环同源（规格 text-input-focused 蓝环）；
 * - 语义：success #37cd84 / error #dd0000（brand-deep）；规格无 warning，
 *   按暗底可读的中明度琥珀 #e6a23c 推导；
 * - 双圆角语言：营销 CTA/徽章 = 药丸 999（Button/Tag），应用件 = 3–6px
 *   （borderRadius 5 / SM 4 / LG 12 营销卡档）；
 * - 排版：标题一律常规字重（fontWeightStrong 400，「让字距承担轮廓」），
 *   waldenburgNormal 的系统栈近似（规格替代建议 Inter 兜底）；
 * - 海拔 = 极少投影：深色卡坐平无影（规格「most cards sit flush」），两级 boxShadow 全关。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色，照 spotify 暗色范例模式）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/sanity.css 的同名令牌手工保持同源：
 *   colorPrimary       = --primary        (#ffffff 白药丸 CTA)
 *   colorPrimaryHover/Active = --login-primary-active 族（#ededed/#d9d9d9 白按压加深推导）
 *   colorInfo          = --info           (#55beff link-blue-soft)
 *   colorSuccess       = --success        (#37cd84)
 *   colorWarning       = --warning        (#e6a23c 推导琥珀)
 *   colorError         = --error          (#dd0000 brand-deep)
 *   colorLink          = --link           (#55beff 暗底链接蓝)
 *   colorText 四阶      = #ffffff / #b9b9b9 / #797979 / #505b6c（ash/mute/slate-soft）
 *   colorBgLayout      = --canvas         (#0b0b0b)
 *   colorBgContainer   = --canvas-card    (#212121)
 *   colorBgElevated    = --tooltip-bg     (#212121 深色卡靠 1px 边分层)
 *   colorBorder 双阶 + colorSplit = #353535 / #212121（hairline-soft / ink-soft）
 *   borderRadius 5/4/12  = 应用件 app-md / app-sm / 营销卡 marketing
 *   boxShadow 双阶全关   = 规格「深色卡坐平」
 *   fontFamily          = --font-sans；fontFamilyCode = --font-mono（IBM Plex Mono）
 */
export const sanityAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // 主 CTA = 白色药丸：hover/按压为白的加深推导（规格未定义 hover，按明度阶梯推导）
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#ededed',
    colorPrimaryActive: '#d9d9d9',
    // 语义色（success / error=brand-deep；warning 规格缺失，推导中明度琥珀）
    colorInfo: '#55beff',
    colorSuccess: '#37cd84',
    colorWarning: '#e6a23c',
    colorError: '#dd0000',
    // 暗底行内链 = link-blue-soft，hover 提亮到 surface-blue-bg 同族的浅蓝
    colorLink: '#55beff',
    colorLinkHover: '#afe3ff',
    colorLinkActive: '#afe3ff',
    // 文字阶梯：标题白 / 正文 ash / 眉题 mute / 占位 slate-soft（规格 Text 五级）
    colorText: '#ffffff',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#b9b9b9',
    colorTextTertiary: '#797979',
    colorTextQuaternary: '#505b6c',
    // 签名：白底（药丸/反白填充）上的文字一律黑
    colorTextLightSolid: '#0b0b0b',
    // 表面阶梯：画布 #0b0b0b / 容器·卡 #212121 / 浮层 #212121（靠 1px #353535 边分层）
    colorBgBase: '#0b0b0b',
    colorBgLayout: '#0b0b0b',
    colorBgContainer: '#212121',
    colorBgElevated: '#212121',
    colorBgSpotlight: '#212121',
    // 边线双轨：hairline-soft #353535（深色卡边）/ ink-soft #212121（输入内嵌线）
    colorBorder: '#353535',
    colorBorderSecondary: '#212121',
    colorSplit: '#353535',
    // 聚焦环 = 蓝（规格 text-input-focused：2px link-blue 环，暗底用 soft 变体）
    controlOutline: 'rgba(85, 191, 255, .3)',
    // 双圆角语言：应用件 5 / 小件 4 / 营销卡 12（药丸在 Button/Tag 组件上单独钉 999）
    borderRadius: 5,
    borderRadiusSM: 4,
    borderRadiusLG: 12,
    // 海拔 = 极少投影：深色卡坐平（规格「most cards sit flush」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // waldenburgNormal 的系统栈近似（规格替代建议：Inter 兜底 + CJK 兜底，禁外部加载）
    fontFamily:
      'waldenburgNormal, Inter, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"IBM Plex Mono", ui-monospace, Consolas, monospace',
    // 编辑部签名：标题一律常规字重（规格「Every heading is regular weight (400)」）
    fontWeightStrong: 400,
  },
  components: {
    Button: {
      // 营销 CTA 药丸：全部按钮 999px（规格 button-primary/ghost-dark = rounded.full）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 按钮字重 500（规格 button-lg 16px/500）
      fontWeight: 500,
      primaryColor: '#0b0b0b',
      // 默认按钮 = 暗面次级钮（规格 button-secondary-dark：canvas-soft 底 + ash 字 + 1px 边）
      defaultBg: '#212121',
      defaultColor: '#b9b9b9',
      defaultBorderColor: '#353535',
      // hover 提亮一阶（#2a2a2a 为 canvas-soft 的单步提亮推导；边转 slate-soft）
      defaultHoverBg: '#2a2a2a',
      defaultHoverColor: '#ffffff',
      defaultHoverBorderColor: '#505b6c',
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    // 白底/反白填充上的选中标记一律黑
    Checkbox: { colorWhite: '#0b0b0b' },
    Steps: { colorWhite: '#0b0b0b' },
    Table: {
      // 扁平表头：与容器同底 #212121 + mute 眉题灰（mono-caps 语汇）+ 透明分隔缝
      headerBg: '#212121',
      headerColor: '#797979',
      headerSplitColor: 'transparent',
      // 行 hover/选中 = canvas-soft 单步提亮推导 #2a2a2a
      rowHoverBg: '#2a2a2a',
      rowSelectedBg: '#2a2a2a',
      rowSelectedHoverBg: '#303030',
      fixedHeaderSortActiveBg: '#2a2a2a',
      borderColor: '#353535',
    },
    Modal: { contentBg: '#212121', headerBg: '#212121', footerBg: '#212121' },
    Drawer: { colorBgElevated: '#212121' },
    Tooltip: { colorBgSpotlight: '#212121' },
    Popover: { colorBgElevated: '#212121' },
    Select: { colorBgContainer: '#0b0b0b', optionSelectedBg: '#2a2a2a' },
    Input: { colorBgContainer: '#0b0b0b' },
    InputNumber: { colorBgContainer: '#0b0b0b' },
    Form: { labelColor: '#ffffff' },
    Segmented: { trackBg: '#0b0b0b', itemSelectedBg: '#2a2a2a' },
    // 徽章 = 全圆药丸（规格 badge-neutral/badge-filled 均为 rounded.full）
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
