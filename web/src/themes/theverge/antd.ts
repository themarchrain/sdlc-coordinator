import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * theverge 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * The Verge 2024 设计语言（暗色，无亮色对应物）：
 * - 近黑画布 #131313（newsprint negative），零阴影：海拔全靠 1px 白/薄荷/紫外发丝线；
 * - 双「hazard tape」强调：Jelly Mint #3CFFD0（主 CTA / 活动态）+ Verge Ultraviolet #5200FF
 *   （第二强调；Inputs：error 边框 = ultraviolet 而非红）；
 * - 形状语言：按钮全圆药丸（24px pill 的 999 近似）、瓦片卡 20/24px、输入 2px「typewriter tag」；
 * - hover 语言：所有链接 hover → Deep Link Blue #3860BE（Do），Focus Cyan #1EAEDB
 *   只给键盘焦点环（controlOutline），绝不静态出现。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态，无白底残留）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/theverge.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#3CFFD0，Jelly Mint「signature acid-mint accent」)
 *   colorPrimaryHover       = #63FFD9           (派生：规格 hover rgba(255,255,255,.2) 叠加薄荷的合成色)
 *   colorPrimaryActive      = --primary-active  (#309875，Console Mint Border「压深处」)
 *   colorInfo / colorLink   = --info / --link   (#3CFFD0，活动电压 = 薄荷)
 *   colorLinkHover/Active   = --link-hover      (#3860BE，Deep Link Blue「唯一 hover 色」)
 *   colorInfoBg/colorInfoBorder = #2D2D2D/#313131（钉「软面 + 发丝线」的克制 info 形态，先例 cursor/antd.ts 40-46）
 *   colorSuccess            = --success         (#3CFFD0，正反馈 = 品牌薄荷)
 *   colorWarning            = --warning         (#FFE500，派生：瓦片调色板 yellow 的可读定值)
 *   colorError              = --error           (#9D7BFF，派生：ultraviolet 可读文字阶；#5200FF 在近黑上仅 2.5:1)
 *   colorText 四阶           = #FFFFFF/#E9E9E9/#949494/#8C8C8C（Neutrals 原文值）
 *   colorBgLayout/Container = --canvas/--canvas-card (#131313，瓦片 = 画布 + 白发丝线)
 *   colorBgElevated         = --canvas-soft     (#2D2D2D，Surface Slate)
 *   colorBorder             = #FFFFFF           (Hazard White「button border / tile outline」)
 *   colorBorderSecondary/Split = #313131        (Image Frame 静线)
 *   borderRadius 2/2/20     = 输入 2px typewriter tag、瓦片 20px
 *   boxShadow 两级全关       = 「color-as-elevation，无投影」
 *   fontFamily              = PolySans 系统栈近似（fallback: Helvetica, Arial）
 *   fontFamilyCode          = PolySans Mono 系统栈近似（fallback: Courier New, Courier）
 */
export const thevergeAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    // Jelly Mint：唯一主 CTA / 活动电压（Primary / Do）
    colorPrimary: '#3CFFD0',
    colorPrimaryHover: '#63FFD9',
    colorPrimaryActive: '#309875',
    // info / processing / link 同薄荷：酸性薄荷是活动信号
    colorInfo: '#3CFFD0',
    colorInfoBg: '#2D2D2D',
    colorInfoBorder: '#313131',
    colorLink: '#3CFFD0',
    colorLinkHover: '#3860BE',
    colorLinkActive: '#3860BE',
    // 语义迁移：success=薄荷 / warning=Verge 黄(派生) / error=ultraviolet 文字阶(派生，规格钉 error 边=紫外)
    colorSuccess: '#3CFFD0',
    colorWarning: '#FFE500',
    colorError: '#9D7BFF',
    // 文字阶梯：Hazard White / Muted Text / Secondary Text / Dim Gray（原文值）
    colorText: '#FFFFFF',
    colorTextHeading: '#FFFFFF',
    colorTextSecondary: '#E9E9E9',
    colorTextTertiary: '#949494',
    colorTextQuaternary: '#8C8C8C',
    // 画布阶：layout/container = 近黑画布；elevated = slate 二级面（下拉/浮层需要浮出画布）
    colorBgBase: '#131313',
    colorBgLayout: '#131313',
    colorBgContainer: '#131313',
    colorBgElevated: '#2D2D2D',
    colorBgSpotlight: '#2D2D2D',
    // 发丝线双阶：白描边承担海拔（零阴影），Image Frame 作静线
    colorBorder: '#FFFFFF',
    colorBorderSecondary: '#313131',
    colorSplit: '#313131',
    // Focus Cyan 只出现在键盘焦点环（「Never shown outside a keyboard-focus state」）
    controlOutline: 'rgba(30, 174, 219, 0.35)',
    // 圆角：输入 2px typewriter tag / 瓦片 20px（Inputs + Border Radius Scale）
    borderRadius: 2,
    borderRadiusSM: 2,
    borderRadiusLG: 20,
    // PolySans 字重 300/500/700；UI 强调 = 700（Hierarchy）
    fontWeightStrong: 700,
    // 零海拔投影：全部层级交给 1px 发丝线（「No box-shadow for elevation」）
    boxShadow: 'none',
    boxShadowSecondary: 'none',
    // 按钮 ~180ms ease、瓦片 150ms ease
    motionDurationFast: '0.15s',
    motionDurationMid: '0.18s',
    motionDurationSlow: '0.18s',
    fontFamily: 'Helvetica, Arial, system-ui, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"Courier New", Courier, monospace',
  },
  components: {
    Button: {
      // 全圆药丸：24px pill 的 999 近似（「fully rounded pill」）
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // 主按钮 = Jelly Mint Pill：薄荷底 Absolute Black 字（Primary）
      primaryColor: '#000000',
      // 默认按钮 = Dark Slate Pill：slate 底 Muted Text 字、无边（Secondary）
      defaultBg: '#2D2D2D',
      defaultColor: '#E9E9E9',
      defaultBorderColor: '#2D2D2D',
      defaultHoverBg: '#3D3D3D',
      defaultHoverColor: '#FFFFFF',
      defaultHoverBorderColor: '#3D3D3D',
      // 按钮字重 700（「PolySans 16px / 700」）；三态阴影全关（零海拔）
      fontWeight: 700,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    // 薄荷底上的勾选/步骤数字 = Absolute Black（同主按钮黑字语言）
    Checkbox: { colorWhite: '#000000' },
    Switch: { handleBg: '#000000', handleShadow: 'none' },
    Steps: { colorWhite: '#000000' },
    Menu: {
      // 下拉当前项 = slate 面 + 薄荷字
      itemSelectedBg: '#2D2D2D',
      itemSelectedColor: '#3CFFD0',
    },
    Table: {
      // slate 表头带 + Secondary Text 标签 + Image Frame 静线
      headerBg: '#2D2D2D',
      headerColor: '#949494',
      headerSplitColor: 'transparent',
      rowHoverBg: '#2D2D2D',
      borderColor: '#313131',
    },
    Modal: { contentBg: '#131313', headerBg: '#131313', footerBg: '#131313' },
    Drawer: { colorBgElevated: '#131313' },
    // 输入族 = 近黑底 + 白描边（Inputs「#131313 background, 1px #ffffff border」）
    Input: { colorBgContainer: '#131313', activeShadow: 'none' },
    InputNumber: { colorBgContainer: '#131313', activeShadow: 'none' },
    Select: { colorBgContainer: '#131313', optionSelectedBg: '#2D2D2D' },
    Segmented: { trackBg: '#2D2D2D', itemSelectedBg: '#131313' },
    // Pill Tag：饱和底黑/白字的文字药丸（「Pill Tag (Non-interactive)」）
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
    Tooltip: { colorBgSpotlight: '#2D2D2D' },
  },
}
