import type { ThemeConfig } from 'antd'

/**
 * airbnb 主题的 antd ThemeConfig —— 自设计语言 派生。
 *
 * Airbnb 设计语言 = 慷慨的消费市场：纯白画布 + 近黑墨 #222222（从不纯黑）+
 * 单一品牌电压 Rausch #FF385C 承担全部主 CTA/激活时刻；摄影与留白扛视觉重量，
 * 字重克制（展示位 500–600，正文 400，rating 64px/700 是唯一最响时刻）。
 * 全软角（按钮 8 / 卡 14 / 搜索条·徽章药丸）、全系统只有一档阴影。
 * algorithm 未指定 = antd 默认算法。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/airbnb.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#FF385C Rausch「single brand voltage」)
 *   colorPrimaryActive      = --primary-active  (#E00B41，规格「Rausch Active」原文)
 *   colorPrimaryHover       = 派生 #F0204E（向按压态的偏移中点；规格只记按压更深）
 *   colorInfo               = --info            (#428BFF，规格 legal-link 蓝复用为 info)
 *   colorLink               = --link            (#222222 墨字链；hover 转 Rausch = 心动保存态语汇)
 *   colorInfoBg             = --info-soft       (#ECF4FF，蓝 10% 薄纱)
 *   colorSuccess            = #008A70（派生：规格无成功绿 → 旅行语境青绿）
 *   colorWarning            = #B7791F（派生：规格无警示色 → 暖金）
 *   colorError              = --error           (#C13515，规格「Error」原文，与 Rausch 区分)
 *   colorText 四阶          = Ink/Body/Muted/Muted-Soft (#222222/#3F3F3F/#6A6A6A/#929292，规格 文字四级原文)
 *   colorBgLayout/Container/Elevated = #FFFFFF（纯白画布，Airbnb 公网页无暗模式）
 *   colorBorder 双阶        = #DDDDDD/#EBEBEB（hairline/hairline-soft 规格字面值）
 *   borderRadius 8          = 规格 rounded.sm（primary/secondary 按钮 8px）
 *   borderRadiusLG 14       = 规格 rounded.md（property/host/reservation 卡 ~14px）
 *   boxShadow / Secondary   = 全系统唯一阴影档字面值（无递进海拔）
 *   fontFamily              = --font-sans（Cereal VF→Circular→系统的规格回退链近似，禁外部加载）
 */
export const airbnbAntdTheme: ThemeConfig = {
  token: {
    // Rausch：唯一品牌色（「Brand & Accent」）
    colorPrimary: '#FF385C',
    colorPrimaryHover: '#F0204E',
    colorPrimaryActive: '#E00B41',
    // info = 规格 legal-link 蓝复用（规格语义段唯一蓝）；链接 = 墨字（Airbnb 行内链接不染色），
    // hover 转 Rausch（交互显色 = 心动保存态语汇，与 css --link/--link-hover 同源）
    colorInfo: '#428BFF',
    colorLink: '#222222',
    colorLinkHover: '#FF385C',
    colorLinkActive: '#E00B41',
    // info Alert = 蓝 10% 薄纱 + hairline
    colorInfoBg: '#ECF4FF',
    colorInfoBorder: '#DDDDDD',
    // 语义三色：error = 规格 #C13515（比 Rausch 更暗更饱和的独立红）；
    // success/warning 规格未给 → 旅行语境派生（青绿 / 暖金）
    colorSuccess: '#008A70',
    colorWarning: '#B7791F',
    colorError: '#C13515',
    // 文字阶梯：Ink / Body / Muted / Muted-Soft（「Text」四级原文，从不纯黑）
    colorText: '#222222',
    colorTextSecondary: '#3F3F3F',
    colorTextTertiary: '#6A6A6A',
    colorTextQuaternary: '#929292',
    // 纯白画布：页面/容器/浮层全白，靠 hairline 与一档阴影分层
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：hairline #DDDDDD（搜索条/表分隔/卡 1px）/ hairline-soft #EBEBEB（长文分隔）
    colorBorder: '#DDDDDD',
    colorBorderSecondary: '#EBEBEB',
    colorSplit: '#EBEBEB',
    // 圆角：8px 按钮（rounded.sm）；14px 大卡（rounded.md）；全系统无硬角
    borderRadius: 8,
    borderRadiusSM: 4,
    borderRadiusLG: 14,
    // 字重克制：strong = 600（展示位 500–600，重 700 只留给 rating 大数字，见 css 片段）
    fontWeightStrong: 600,
    // 海拔：全系统唯一阴影档（字面值），无递进层
    boxShadow: 'rgba(0, 0, 0, 0.02) 0 0 0 1px, rgba(0, 0, 0, 0.04) 0 2px 6px, rgba(0, 0, 0, 0.1) 0 4px 8px',
    boxShadowSecondary: 'rgba(0, 0, 0, 0.02) 0 0 0 1px, rgba(0, 0, 0, 0.04) 0 2px 6px, rgba(0, 0, 0, 0.1) 0 4px 8px',
    // Cereal VF（授权定制族，禁外部加载）→ Circular → 系统栈（规格回退链）；附 CJK
    fontFamily:
      'Circular, -apple-system, system-ui, Roboto, "Helvetica Neue", Helvetica, Arial, "PingFang SC", "Microsoft YaHei", sans-serif',
  },
  components: {
    Button: {
      // 按钮字重 500（规格 button-md 16px/500）；三态阴影全关（阴影只留给浮层/卡）
      fontWeight: 500,
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
    },
    Menu: {
      // 下拉当前项 = Surface Soft 微高亮 + 墨（sub-nav hover 面规格）
      itemSelectedBg: '#F7F7F7',
      itemSelectedColor: '#222222',
    },
    Table: {
      // 白底表头 + Muted 标签 + hairline 分隔（编辑型栏段语汇：无卡面、无投影）
      headerBg: '#FFFFFF',
      headerColor: '#6A6A6A',
      headerSplitColor: '#DDDDDD',
      rowHoverBg: '#F7F7F7',
      borderColor: '#EBEBEB',
    },
    Tooltip: {
      // 浮层提示 = 墨面倒置岛（date-picker-day-selected「Ink fill, white text」同语汇）
      colorBgSpotlight: '#222222',
    },
  },
}
