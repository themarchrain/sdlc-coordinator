import type { ThemeConfig } from 'antd'

/**
 * playstation 主题的 antd ThemeConfig —— 自设计语言 派生
 * （frontmatter 令牌表 + 正文角色句）。
 *
 * PlayStation 三面营销系统（黑/白/PS Blue 章节交替），管理台默认取白亮轨
 * （canvas-light #FFFFFF）：白画布 + 冷蓝染次级卡 #F5F7FA（surface-card）+
 * 纯黑墨 #000000（ink）；PS Blue #0070D1 全圆药丸 CTA（rounded.full 9999px，
 * 规格 Don't「Don't soften pill geometry」）；PlayStation SST 展示字 weight 300
 * 的 airy 编辑感（css 片段承担）；8px 次级卡系统（rounded.md）。系统平坦无影
 * （「cards lift only on press」），浮层唯一允许 Level 2 0 4px 12px rgba(0,0,0,.16)。
 * 黑章节面只保留给 json 深卡与登录 showcase（css 令牌层）。algorithm 未指定
 * = antd 默认算法（亮色）。
 *
 * 迁移取舍（规格未给、按设计语言推导，均在行内注明）：
 *   - 规格无 hover 态（系统政策）→ pressed #0064B7 映射为 hover，
 *     active #004D8D（primary-active 字面值）映射为 active；
 *   - 规格「warning」#C81B3A 实为校验/破坏性用色（红相）→ 映射 colorError；
 *     colorWarning 派生标准琥珀 #97650D（规格无真琥珀警示）；
 *   - colorSuccess 派生 #1F7A3D（规格无 success 字面值）；
 *   - controlHeight 44 = 规格 Touch Targets WCAG AAA 下沿（按钮/输入字面 48px
 *     对管理台密度偏高，取 ≥44 达标值）；
 *   - commerce 橙 #D53B00 规格保留给商店 CTA，不进管理台交互位（仅出现在
 *     主题卡 swatches 示意条 accent 位）。
 *
 * 约束：antd 会对 colorPrimary 做数值派生（hover/active/透明度变体），
 * CSS 变量无法参与计算，因此这里必须写死字面 hex，禁止 var()。
 * 与 themes/css/playstation.css 的同名令牌手工保持同源：
 *   colorPrimary            = --primary         (#0070D1 PS Blue，universal primary CTA)
 *   colorPrimaryHover       = #0064B7           (primary-pressed 字面值作 hover)
 *   colorPrimaryActive      = --primary-active  (#004D8D primary-active 字面值)
 *   colorInfo               = --info            (#0070D1 蓝 = badge-info / info 族)
 *   colorLink               = --link            (#0064B7 link-light 字面值，同 pressed)
 *   colorInfoBg/colorInfoBorder = --info-soft/--hairline-strong
 *                                                (#E8F2FB 蓝 9% 展开实底 + #CCCCCC ash-light 控件边)
 *   colorSuccess/Warning/Error = 语义三色（派生绿 / 派生琥珀 / 规格 warning 红相字面值）
 *   colorText 四阶           = ink #000000 / body-light rgba(0,0,0,.6) 字面值
 *                             / mute-light #6B6B6B / 墨 40% 合成 #999999（alpha 阶梯下探一档）
 *   colorBgLayout/Container/Elevated = --canvas/--canvas-card (#FFFFFF 白亮轨三面全白)
 *   colorBorder 双阶 + colorSplit = ash-light #CCCCCC（输入边）/ hairline-light #F3F3F3（分隔）
 *   borderRadius 8          = rounded.md（8px 次级卡系统）
 *   borderRadiusLG 16       = rounded.lg（dialog 卡稀有档）
 *   Button borderRadius 9999 = rounded.full（每颗 CTA 都是全圆药丸）
 *   fontWeightStrong 600    = heading-md 18px/600（规格唯六字重里的强标签档）
 *   boxShadowSecondary      = Level 2 0 4px 12px rgba(0,0,0,.16) 字面值（浮层专属）
 *   fontFamily              = --font-sans（SST 专有 → 规格回退 sst/Arial/Helvetica，
 *                             替代 Inter/Roboto 近似栈）
 */
export const playstationAntdTheme: ThemeConfig = {
  token: {
    // PlayStation Blue：universal primary CTA（「precise #0070d1」）
    colorPrimary: '#0070D1',
    colorPrimaryHover: '#0064B7', // primary-pressed 字面值作 hover（规格无 hover 态）
    colorPrimaryActive: '#004D8D', // primary-active 字面值
    // info = PS Blue（badge-info 填充同源）；link = link-light #0064B7 字面值
    colorInfo: '#0070D1',
    colorLink: '#0064B7',
    colorLinkHover: '#004D8D',
    colorLinkActive: '#004D8D',
    // info Alert = 蓝 9% 薄纱展开实底 + ash-light 控件边（软面 + 发丝线的克制 info 形态）
    colorInfoBg: '#E8F2FB',
    colorInfoBorder: '#CCCCCC',
    // 语义三色：规格 warning #C81B3A 为校验/破坏性红相 → error；warning/success 派生
    colorSuccess: '#1F7A3D', // 派生：规格无 success，标准语义绿克制化
    colorWarning: '#97650D', // 派生：规格无真琥珀警示（警示横幅未 captured）
    colorError: '#C81B3A', // 规格 warning 字面值（validation errors / destructive）
    // 文字阶梯：ink 纯黑 / body-light 60% 墨（字面 rgba）/ mute-light / 墨 40% 合成
    colorText: '#000000',
    colorTextSecondary: 'rgba(0, 0, 0, 0.6)', // body-light 字面值（系统默认段落色）
    colorTextTertiary: '#6B6B6B', // mute-light：metadata 档
    colorTextQuaternary: '#999999', // 派生：rgba(0,0,0,.4) 合成（alpha 阶梯下探一档）
    // 白亮轨：画布、容器、浮层全白（canvas-light）
    colorBgLayout: '#FFFFFF',
    colorBgContainer: '#FFFFFF',
    colorBgElevated: '#FFFFFF',
    // 边线双阶：ash-light（text-input 1px 边字面值）/ hairline-light（1px 分隔线字面值）
    colorBorder: '#CCCCCC',
    colorBorderSecondary: '#F3F3F3',
    colorSplit: '#F3F3F3',
    // 焦点环 = 蓝 12% 薄纱（推导：text-input-focused 以 2px 蓝边为信号、无 halo）
    controlOutline: 'rgba(0, 112, 209, 0.12)',
    // 圆角：rounded.md 8 次级卡系统为默认；rounded.sm 4 给小件；rounded.lg 16 给 dialog 稀有档
    borderRadius: 8,
    borderRadiusSM: 4,
    borderRadiusLG: 16,
    // 触达高：WCAG AAA 下沿 44（规格按钮/输入字面 48，管理台密度取达标值）
    controlHeight: 44,
    // heading-md 600 = 强标签档（display 300 的反差由 css 片段承担）
    fontWeightStrong: 600,
    // 平坦系统：静止卡无影（规格 Don't「no drop shadows on resting cards」）
    boxShadow: 'none',
    // 浮层唯一 Level 2 字面值（Active/pressed CTA、lifted card 档）
    boxShadowSecondary: '0 4px 12px rgba(0, 0, 0, 0.16)',
    // SST 专有（禁外部加载）→ 规格回退 sst/Arial/Helvetica + 替代栈 Inter（body 切）
    fontFamily:
      "'PlayStation SST', SST, Inter, Arial, Helvetica, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Button: {
      // 每颗 CTA 都是全圆药丸（rounded.full 9999px；规格 Don't「CTAs are always full」）
      borderRadius: 9999,
      borderRadiusSM: 9999,
      borderRadiusLG: 9999,
      // button-md 700：与 display 300 形成「编辑部游戏杂志」式字重反差
      fontWeight: 700,
      // 平坦系统：三态阴影全关（lift 只在 press，由 active 态色变承担）
      primaryShadow: 'none',
      defaultShadow: 'none',
      dangerShadow: 'none',
      // 默认钮 = button-secondary-light：透明底 + 墨字 + ash-light 边
      defaultBg: 'transparent',
      defaultColor: '#000000',
      defaultBorderColor: '#CCCCCC',
      defaultHoverBg: '#F5F7FA', // hover = surface-card 冷蓝染微沉
      defaultHoverColor: '#000000',
      defaultHoverBorderColor: '#6B6B6B',
    },
    Menu: {
      // 下拉当前项 = surface-card 冷蓝染 + 纯黑墨
      itemSelectedBg: '#F5F7FA',
      itemSelectedColor: '#000000',
    },
    Table: {
      // 白底表头 + mute-light 标签 + hairline-light 分隔缝；行 hover = 冷蓝染
      headerBg: '#FFFFFF',
      headerColor: '#6B6B6B',
      headerSplitColor: 'transparent',
      rowHoverBg: '#F5F7FA',
      borderColor: '#F3F3F3',
    },
    Tooltip: {
      // ink-deep 近黑反转面（黑章节面的浮层化身）
      colorBgSpotlight: '#121314',
    },
    Segmented: {
      // filter-pill：默认 surface-filter 半透明冷蓝染 / 激活翻不透明白（字面值）
      trackBg: 'rgba(245, 247, 250, 0.3)',
      itemSelectedBg: '#FFFFFF',
    },
    Select: {
      optionSelectedBg: '#F5F7FA',
    },
    Tag: {
      // badge-info 全圆药丸（rounded.full 字面用法）
      borderRadiusSM: 9999,
    },
  },
}
