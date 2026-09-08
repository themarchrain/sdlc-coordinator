import type { ThemeConfig } from 'antd'

/**
 * wise 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Wise 设计语言 = 北欧金融杂志：sage 鼠尾草画布 #e8ebe6 与白卡靠表面对比分层
 * （Level 2 海拔「the surface contrast IS the elevation」），暖橄榄近黑墨 #0e0f0c，
 * 唯一品牌色 = 柠檬绿 #9fe870（CTA 专色）。两条签名反转：
 *  1. hover/active 是提亮（#cdffad「the lighter green」）而非常规压深；
 *  2. 绿面上永远压近黑字（colorTextLightSolid = on-primary #0e0f0c）。
 * 边线语言：输入/三级按钮 = 1px solid ink（Level 1 海拔），卡面无边靠 sage/白对比。
 * 圆角即亲和力：输入 12（rounded.md）/ 卡 24（rounded.xl 签名「never sharp corners」）。
 * algorithm 未指定 = antd 默认算法（亮色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/wise.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#9fe870 Wise Green，CTA 专色)
 *   colorPrimaryHover/Active = #cdffad          (规格字面：active = 「lighter green」，提亮反转)
 *   colorInfo               = --info            (#0b74aa，accent-cyan #38c8ff 压深到文字级，推导)
 *   colorLink               = --link            (#0e0f0c 墨链路：规格 nav-link/button-tertiary 全 ink)
 *   colorSuccess/Warning/Error = --success/--warning/--error
 *                             (#2ead4b positive / #b86700 warning-deep / #d03238 negative；
 *                              规格明言 Wise 绿绝不兼成功色，success 用 positive 族)
 *   colorText 四阶          = ink/body/mute/mute 提亮 (#0e0f0c/#454745/#868685/#a1a3a0 推导)
 *   colorBgLayout           = --canvas          (#e8ebe6 sage 页画布，品牌情绪面)
 *   colorBgContainer/Elevated = --canvas-card   (#ffffff 卡内白)
 *   colorBorder             = ink #0e0f0c       (text-input/button-tertiary 的 1px ink 边——签名)
 *   colorBorderSecondary/colorSplit = #e8ebe6   (canvas-soft 分隔缝——规格 rowBorder/item-divider)
 *   borderRadius 12 / LG 24 = rounded.md / rounded.xl（按钮 24px 由 Button 组件钉死）
 *   fontFamily              = --font-sans（Wise Sans 专有 → Inter 栈近似，禁外部字体）
 */
export const wiseAntdTheme: ThemeConfig = {
  token: {
    // Wise Green：唯一品牌色，CTA 专色
    colorPrimary: '#9fe870',
    colorPrimaryHover: '#cdffad', // 规格字面：active = 「the lighter green」——签名提亮反转
    colorPrimaryActive: '#cdffad',
    // 绿面上的字 = 近黑（on-primary）：签名的「绿药丸 + 暖黑字」
    colorTextLightSolid: '#0e0f0c',
    // info = accent-cyan 压深到文字级（推导）；link = 墨（规格全 ink 链路语汇）
    colorInfo: '#0b74aa',
    colorInfoBg: '#e7f8ff',
    colorInfoBorder: '#aee5f8',
    colorLink: '#0e0f0c',
    colorLinkHover: '#163300',
    colorLinkActive: '#163300',
    // 语义三色取规格 positive / warning-deep / negative 原值（Wise 绿绝不兼成功色）
    colorSuccess: '#2ead4b',
    colorSuccessBg: '#e2f6d5', // badge-positive 的 primary-pale 底（规格字面）
    colorSuccessText: '#054d28', // badge-positive 的 positive-deep 字（规格字面；antd AliasToken 无 colorSuccessContent）
    colorWarning: '#b86700', // warning-deep（#ffd11a 黄只作面，文字级用深琥珀）
    colorWarningBg: '#fff7d6',
    colorWarningText: '#4a3b1c', // 规格字面 warning-content（antd AliasToken 无 colorWarningContent）
    colorError: '#d03238',
    // 文字阶梯：ink / body / mute / mute 提亮（#a1a3a0 推导：占位/禁用级）
    colorText: '#0e0f0c',
    colorTextHeading: '#0e0f0c',
    colorTextSecondary: '#454745',
    colorTextTertiary: '#868685',
    colorTextQuaternary: '#a1a3a0',
    // sage 页画布 + 白卡/白浮层（规格：surface contrast carries elevation）
    colorBgLayout: '#e8ebe6',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    // 1px ink 控件边 = Level 1 海拔（text-input / button-tertiary / converter card 签名）
    colorBorder: '#0e0f0c',
    colorBorderSecondary: '#e8ebe6', // canvas-soft 内分隔（规格 rowBorder/item-divider）
    colorSplit: '#e8ebe6',
    // 焦点环 = 绿纱（CTA 专色的功能位）
    controlOutline: 'rgba(159, 232, 112, .35)',
    // 圆角刻度：8 徽标小件 / 12 输入 / 24 签名大圆角
    borderRadius: 12,
    borderRadiusSM: 8,
    borderRadiusLG: 24,
    // 扁平为主 + 软影：卡面靠表面对比，浮层给一点纸感软影（ex-modal-card「elevated shadow」）
    boxShadow: '0 8px 24px rgba(14, 15, 12, 0.08)',
    boxShadowSecondary: '0 12px 32px rgba(14, 15, 12, 0.12)',
    // Inter = 品牌真实第二面孔（Wise Sans 专有，禁外部加载 → Inter 栈近似）
    fontFamily:
      "'Inter', system-ui, -apple-system, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    // strong = 600：规格「weight 600 for everything else」（900 只留给 hero display）
    fontWeightStrong: 600,
  },
  components: {
    Button: {
      // 24px「pill-rectangle」非协商（规格 Don't：CTA 永不做直角矩形）
      borderRadius: 24,
      borderRadiusSM: 16,
      borderRadiusLG: 24,
      // 按钮标签 = button-md 16/600（规格字面）
      fontWeight: 600,
      // 药丸平坦无影（海拔靠表面对比）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = button-tertiary：白底 + 1px ink 边 + 墨字（规格字面）
      defaultBg: '#ffffff',
      defaultColor: '#0e0f0c',
      defaultBorderColor: '#0e0f0c',
      defaultHoverBg: '#e8ebe6', // hover 落 sage（button-secondary 面语汇）
      defaultHoverColor: '#0e0f0c',
      defaultHoverBorderColor: '#0e0f0c',
    },
    Menu: {
      // 下拉当前项 = primary-neutral #c5edab（规格字面「neutral active fill」）+ 墨字
      itemSelectedBg: '#c5edab',
      itemSelectedColor: '#0e0f0c',
    },
    Table: {
      // 表头 = canvas-soft sage（规格 ex-data-table-cell headerBackground）+ body 墨标签
      headerBg: '#e8ebe6',
      headerColor: '#454745',
      headerSplitColor: 'transparent',
      rowHoverBg: '#f0faea', // 推导：primary-pale 提亮半档（行 hover 落绿纱）
      borderColor: '#e8ebe6', // 规格 rowBorder = canvas-soft
    },
    Tooltip: {
      // ink 反转面：墨底白字（colorTextLightSolid 全局已改墨，浮层此处拨回白）
      colorBgSpotlight: '#0e0f0c',
      colorTextLightSolid: '#ffffff',
    },
    Segmented: {
      trackBg: '#e8ebe6',
      itemSelectedBg: '#ffffff',
    },
    Select: {
      optionSelectedBg: '#e2f6d5', // primary-pale 选中项
    },
    Tag: {
      // 状态徽标 = 药丸（badge-positive/negative 的 rounded.pill）
      borderRadiusSM: 999,
    },
  },
}
