import type { ThemeConfig } from 'antd'

/**
 * nvidia 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * NVIDIA 设计语言 = 工程图纸式单强调色系统：NVIDIA Green #76B900 独占全部
 * CTA/激活态/角标方块，其余只有黑白灰；绿底黑字（on-primary #000000）是
 * 签名反转；全部交互件 2px 圆角（「never 0, never past 4」）；字重只有
 * 400/700，层级靠字重与字号差；卡片零投影、hairline #CCCCCC 描边分隔；
 * link-blue #0046A4 是全系统唯一蓝，只许做内联链接/数据墨水。
 * algorithm 未指定 = antd 默认（亮色）算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/nvidia.css 的同名令牌手工保持同源：
 *   colorPrimary             = --primary          (#76B900，规格 colors.primary)
 *   colorPrimaryHover        = 压深一阶            (#6DA400，派生；规格「hover 不文档化」)
 *   colorPrimaryActive       = primary-dark       (#5A8D00，规格 pressed 原文)
 *   colorTextLightSolid      = on-primary         (#000000，签名：绿底黑字)
 *   colorInfo/colorLink      = --info/--link      (#0046A4 link-blue，唯一蓝)
 *   colorSuccess             = success-deep       (#3F8500，绿饱和撞色时的正向色)
 *   colorWarning             = warning            (#DF6500，规格原文)
 *   colorError               = error              (#E52020，规格原文)
 *   colorText 四阶           = Ink/Body/Mute/Ash   (#000000/#1A1A1A/#757575/#A7A7A7)
 *   colorBgLayout/Container/Elevated = --canvas   (#FFFFFF 纸白正文轨)
 *   colorBorder 双阶 + colorSplit = Hairline/派生浅一阶 (#CCCCCC/#E0E0E0)
 *   borderRadius 2/2/2       = 规格 sm：所有交互件 2px
 *   fontWeightStrong 700     = 字重两极制（400/700），strong 即 700
 *   boxShadow                = 规格 Level 3 唯一合法 5px ambient（留给浮层/sticky chrome）
 *   fontFamily               = NVIDIA-EMEA → Inter（规格推荐替身）→ Arial（官方回退）
 */
export const nvidiaAntdTheme: ThemeConfig = {
  token: {
    // NVIDIA Green：唯一强调色（规格「carries every CTA, active tab, corner square」）
    colorPrimary: '#76B900',
    colorPrimaryHover: '#6DA400', // 派生：压深一阶（规格不文档化 hover）
    colorPrimaryActive: '#5A8D00', // 规格 button-primary-active = primary-dark
    // 签名反转：绿底黑字（规格 on-primary #000000）
    colorTextLightSolid: '#000000',
    // info / link = link-blue：全系统唯一蓝（规格「reserved for inline links」）
    colorInfo: '#0046A4',
    colorLink: '#0046A4',
    colorLinkHover: '#003A8C', // 派生：压深一阶
    // info Alert 钉成「link-blue 淡染 + hairline 边」（info-soft 与 CSS 同源派生）
    colorInfoBg: '#E8EEF7',
    colorInfoBorder: '#CCCCCC',
    // 语义三色（规格 Semantic 原文；success-deep 专为「绿饱和撞色」场景而设）
    colorSuccess: '#3F8500',
    colorWarning: '#DF6500',
    colorError: '#E52020',
    // 文字阶梯：Ink / Body / Mute / Ash（原文）
    colorText: '#000000',
    colorTextSecondary: '#1A1A1A',
    colorTextTertiary: '#757575',
    colorTextQuaternary: '#A7A7A7',
    // 纸白画布：页面、容器、浮层全白，靠 hairline 与密度分隔
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：Hairline（卡/表规则线）/ 派生浅一阶（次级分隔）
    colorBorder: '#CCCCCC',
    colorBorderSecondary: '#E0E0E0',
    colorSplit: '#CCCCCC',
    // 超锐利几何：全部交互件 2px（规格「never go to 0, never go past 4」）
    borderRadius: 2,
    borderRadiusSM: 2,
    borderRadiusLG: 2,
    // 字重两极制：只有 400/700（规格「weight contrast, not size jumps」）
    fontWeightStrong: 700,
    // 唯一合法阴影：Level 3 的 5px ambient（sticky chrome/浮层）；卡片一律平面
    boxShadow: '0 0 5px 0 rgba(0, 0, 0, .3)',
    boxShadowSecondary: 'none',
    // NVIDIA-EMEA 的推荐替身 Inter + 官方回退 Arial
    fontFamily:
      'Inter, Arial, Helvetica, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 700（规格 button-md 16px/700）
      fontWeight: 700,
      // button-outline：绿描边次级 CTA（「second-tier actions still earn the brand color」）
      defaultBorderColor: '#76B900',
      defaultColor: '#000000',
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // pill-tab-active：激活即黑底反转 + 白字（规格原文 ink bg / on-dark text）
      itemSelectedBg: '#000000',
      itemSelectedColor: '#FFFFFF',
    },
    Table: {
      // 白底表头 + hairline 规则线；行 hover = Soft Surface（规格交替行背景）
      headerBg: '#FFFFFF',
      headerColor: '#000000',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F7F7F7',
      borderColor: '#CCCCCC',
    },
    Tooltip: {
      colorBgSpotlight: '#000000', // surface-dark 反转面
    },
  },
}
