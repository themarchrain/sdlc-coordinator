import type { ThemeConfig } from 'antd'

/**
 * lovable 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Lovable 设计语言 = 暖纸上的克制编辑风：
 * 羊皮纸奶油画布 #f7f4ed（「not white, not beige, a deliberate cream」）、
 * 近黑炭墨 #1c1c1c（「Not pure black — organic warmth」）同时兼任主 CTA
 * （「Primary CTA: Charcoal (#1c1c1c)」），灰阶全部由炭墨透明度派生
 * （0.03 / 0.04 / 0.4 / 0.82–0.83），边线双轨 #eceae4（被动）/ rgba(28,28,28,.4)（交互），
 * 阴影只存在于两处：暗钮签名内阴影与 0 4px 12px rgba(0,0,0,.1) 软焦点阴影，
 * 字重只有 400/600（「Don't use weight 700 — 600 is the maximum」）。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生，这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/lovable.css 的同名令牌手工保持同源：
 *   colorPrimary / colorLink = --primary / --link   (#1C1C1C，炭墨即主 CTA，链接同墨色)
 *   colorPrimaryHover/Active = 炭墨 90% / 80% 压奶油合成色 #323231 / #484746
 *                             （「Active: opacity 0.8」→ 按压=提亮而非压深）
 *   colorInfo = --info        (#3B82F6 Ring Blue，规格唯一的彩色：无障碍焦点环)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline
 *   colorText 阶 = 炭墨透明度阶梯 100%/83%/82%/Muted Gray 的压奶油合成色
 *                  #1C1C1C / #414140 / #434342 / #5F5F5D（Neutral Scale 原文）
 *   colorBgLayout/Container = --canvas (#F7F4ED cream，卡与页面同底无缝)
 *   colorBgElevated = #FCFBF8 (Off-White「subtle highlight」给浮层半档提升)
 *   colorBorder 双阶 = Light Cream #ECEAE4 / 炭墨 3% 合成 #F0EEE7
 *   borderRadius 6 = 规格 按钮/输入 Standard；LG 12 = Card；SM 4 = Micro
 *   fontWeightStrong 600 = 标题上限（无 bold）
 *   boxShadow = 规格 Focus Shadow 字面值（软焦点的唯一阴影）
 *   controlOutline = Ring Blue 50% 字面值 + 2px 环（Interactive「--tw-ring-color」）
 *   fontFamily = Camera Plain Variable + 规格声明的回退栈 ui-sans-serif, system-ui
 */
export const lovableAntdTheme: ThemeConfig = {
  token: {
    // Charcoal 炭墨：主 CTA / 链接同色（「Charcoal (#1c1c1c): dark button backgrounds」）
    colorPrimary: '#1c1c1c',
    // 派生：炭墨 90%/80% 压奶油合成色（「Active: opacity 0.8」= 按压提亮）
    colorPrimaryHover: '#323231',
    colorPrimaryActive: '#484746',
    // 链接 = 炭墨 + 下划线承载交互信号（Links「No color change on hover」）
    colorLink: '#1c1c1c',
    colorLinkHover: '#1c1c1c',
    // info = Ring Blue：规格唯一功能性彩色（无障碍焦点）
    colorInfo: '#3b82f6',
    colorInfoBg: '#eef4fd',
    colorInfoBorder: '#eceae4',
    // 语义三色：规格明令「Don't introduce saturated accent colors」→
    // 按规格自带的技术栈（「shadcn/ui + Radix + Tailwind」）取 Tailwind 700 文字级克制档
    colorSuccess: '#15803d',
    colorWarning: '#b45309',
    colorError: '#b91c1c',
    // 文字阶梯：炭墨透明度模型压奶油合成（82% 正文 / 83% 次强 / Muted Gray 说明）
    colorTextBase: '#1c1c1c',
    colorTextHeading: '#1c1c1c',
    colorText: '#434342',
    colorTextSecondary: '#414140',
    colorTextTertiary: '#5f5f5d',
    colorTextQuaternary: '#5f5f5d',
    // 奶油画布：页面/容器同底（卡与页面无缝）；浮层 = Off-White 微提亮
    colorBgLayout: '#f7f4ed',
    colorBgContainer: '#f7f4ed',
    colorBgElevated: '#fcfbf8',
    // 边线双轨：Light Cream 被动 / 炭墨 3% 更轻一档
    colorBorder: '#eceae4',
    colorBorderSecondary: '#f0eee7',
    colorSplit: '#eceae4',
    // 圆角：Standard 6（按钮/输入）/ Card 12 / Micro 4（刻度）
    borderRadius: 6,
    borderRadiusSM: 4,
    borderRadiusLG: 12,
    // 字重窄幅：400 正文 / 600 标题上限（「Two weights, clear roles」）
    fontWeightStrong: 600,
    // 阴影 = 规格 Focus Shadow 字面值（软、大而弥散）
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.1)',
    boxShadowSecondary: '0 4px 12px rgba(0, 0, 0, 0.1)',
    // 焦点环 = Ring Blue 50% + 2px（「Ring Blue (#3b82f6 at 50%)」字面）
    controlOutline: 'rgba(59, 130, 246, 0.5)',
    controlOutlineWidth: 2,
    // Camera Plain Variable（定制族）+ 规格声明回退 ui-sans-serif, system-ui
    fontFamily:
      '"Camera Plain Variable", ui-sans-serif, system-ui, -apple-system, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮标签 400（Button 16px/400）；暗钮签名内阴影字面誊抄
      // （Primary Dark「rgba(255,255,255,.2) 0px 0.5px inset, rgba(0,0,0,.2) 0px 0px 0px 0.5px inset, rgba(0,0,0,.05) 0px 1px 2px」）
      fontWeight: 400,
      primaryShadow:
        'inset 0 0.5px 0 rgba(255, 255, 255, 0.2), inset 0 0 0 0.5px rgba(0, 0, 0, 0.2), 0 1px 2px rgba(0, 0, 0, 0.05)',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = 炭墨 4% 微染 + 炭墨字（「Charcoal 4%: subtle hover backgrounds」）
      itemSelectedBg: 'rgba(28, 28, 28, 0.04)',
      itemSelectedColor: '#1c1c1c',
    },
    Table: {
      // 奶油同底表头 + Muted Gray 标签；行 hover = 炭墨 4% 微染
      headerBg: '#f7f4ed',
      headerColor: '#5f5f5d',
      headerSplitColor: '#eceae4',
      rowHoverBg: 'rgba(28, 28, 28, 0.04)',
      borderColor: '#eceae4',
    },
    Tooltip: {
      // 反转面 = 炭墨（规格 暗钮面），文字 Off-White
      colorBgSpotlight: '#1c1c1c',
    },
    Input: {
      // 输入边：被动 Light Cream，聚焦升为炭墨 40% 交互轨（双轨边线）
      hoverBorderColor: 'rgba(28, 28, 28, 0.4)',
      activeBorderColor: 'rgba(28, 28, 28, 0.4)',
      activeShadow: '0 0 0 2px rgba(59, 130, 246, 0.5)',
    },
  },
}
