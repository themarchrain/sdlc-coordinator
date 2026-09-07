import { theme } from 'antd'
import type { ThemeConfig } from 'antd'

/**
 * xai 主题的 antd ThemeConfig —— 自 design/xai main.tsx 原样搬移（字面 hex，禁 var()）。
 *
 * xAI 设计语言（暗色）：白药丸是唯一主 CTA（白底近黑字），描边按钮走半透明白；
 * 画布阶 layout/canvas #0a0a0a、container/card #191919、elevated/soft #1a1c20；
 * 文字阶 ink #ffffff / body #dadbdf / mute #7d8187；发丝线 #212327（#363a3f 强调阶）；
 * 语义色映射进文档点缀组：success=breeze、warning=sunset-soft、error=sunset、info=twilight。
 * algorithm = darkAlgorithm（暗色算法派生填充/禁用态等中间色）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/xai.css 的同名令牌手工保持同源：
 *   colorPrimary      = --primary       (#ffffff)
 *   colorPrimaryHover = --link-hover    (#fafaf7)
 *   colorPrimaryActive= --primary-active(#dadbdf)
 *   colorInfo         = --info          (#c4b5fd twilight)
 *   colorSuccess      ≈ 点缀组 breeze   (#a0c3ec)
 *   colorWarning      = --warning       (#ffc285 sunset-soft)
 *   colorError        = --error         (#ff7a17 sunset)
 *   colorText         = --body          (#dadbdf)
 *   colorTextHeading  = --ink           (#ffffff)
 *   colorBgLayout     = --canvas        (#0a0a0a)
 *   colorBgContainer  = --canvas-card   (#191919)
 *   colorBgElevated   = --canvas-soft   (#1a1c20)
 *   colorBorder       = --canvas-mid    (#363a3f)
 *   colorBorderSecondary / colorSplit = --hairline (#212327)
 *   fontFamily        = --font-sans；fontFamilyCode = --font-mono
 */
export const xaiAntdTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    colorPrimary: '#ffffff',
    colorPrimaryHover: '#fafaf7',
    colorPrimaryActive: '#dadbdf',
    colorInfo: '#c4b5fd',
    colorSuccess: '#a0c3ec',
    colorWarning: '#ffc285',
    colorError: '#ff7a17',
    colorLink: '#ffffff',
    colorLinkHover: '#fafaf7',
    colorLinkActive: '#dadbdf',
    colorText: '#dadbdf',
    colorTextHeading: '#ffffff',
    colorTextSecondary: '#dadbdf',
    colorTextTertiary: '#7d8187',
    colorTextQuaternary: '#7d8187',
    colorBgBase: '#0a0a0a',
    colorBgLayout: '#0a0a0a',
    colorBgContainer: '#191919',
    colorBgElevated: '#1a1c20',
    colorBgSpotlight: '#1a1c20',
    colorBorder: '#363a3f',
    colorBorderSecondary: '#212327',
    colorSplit: '#212327',
    controlOutline: 'rgba(255, 255, 255, 0.22)',
    borderRadius: 8,
    fontFamily: 'Inter, system-ui, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    fontFamilyCode: '"JetBrains Mono", "Cascadia Code", Consolas, monospace',
  },
  components: {
    Button: {
      // 药丸是品牌唯一的交互形状：全部按钮 999px
      borderRadius: 999,
      borderRadiusSM: 999,
      borderRadiusLG: 999,
      // button-outline-on-dark：半透明白描边 + 透明底 + 白字
      defaultBorderColor: 'rgba(255, 255, 255, 0.25)',
      defaultColor: '#ffffff',
      defaultBg: 'transparent',
      // button-primary：白底近黑字（on-primary）
      primaryColor: '#0a0a0a',
      fontWeight: 400,
      defaultShadow: 'none',
      primaryShadow: 'none',
      dangerShadow: 'none',
    },
    Checkbox: { colorWhite: '#0a0a0a' },
    Switch: { handleBg: '#0a0a0a', handleShadow: 'none' },
    Steps: { colorWhite: '#0a0a0a' },
    Table: {
      headerBg: '#1a1c20',
      headerColor: '#7d8187',
      headerSplitColor: 'transparent',
      headerBorderRadius: 0,
      rowHoverBg: '#1a1c20',
      rowSelectedBg: '#1a1c20',
      rowSelectedHoverBg: '#363a3f',
      fixedHeaderSortActiveBg: '#1a1c20',
      borderColor: '#212327',
    },
    Modal: { contentBg: '#191919', headerBg: '#191919', footerBg: '#191919' },
    Drawer: { colorBgElevated: '#191919' },
    Tooltip: { colorBgSpotlight: '#1a1c20' },
    Popover: { colorBgElevated: '#1a1c20' },
    Select: { colorBgContainer: '#1a1c20', optionSelectedBg: '#363a3f' },
    Input: { colorBgContainer: '#1a1c20' },
    InputNumber: { colorBgContainer: '#1a1c20' },
    Form: { labelColor: '#dadbdf' },
    Segmented: { trackBg: '#1a1c20', itemSelectedBg: '#363a3f' },
    Tag: { borderRadiusSM: 999 },
    Pagination: { itemActiveBg: 'transparent' },
  },
}
