import type { ThemeDefinition, ThemeSwatches } from '../types'
import { clickhouseAntdTheme } from './antd'
import { clickhouseSlots } from './slots'

/** clickhouse 主题五色示意条（外观面板展示）：近纯黑 + 白字 + 电光黄一族。 */
export const clickhouseSwatches: ThemeSwatches = {
  canvas: '#0a0a0a',
  surface: '#1a1a1a',
  ink: '#ffffff',
  primary: '#faff69',
  accent: '#22c55e',
}

/**
 * clickhouse 主题 = ClickHouse 设计语言（暗色）。
 * 近纯黑画布 #0a0a0a + 卡阶 #121212/#1a1a1a/#242424 + 唯一品牌电压电光黄 #faff69；
 * 黄底黑字是品牌动作信号，黄只落在主 CTA / 统计数值（ALWAYS yellow）/ 用量轨——
 * 元素级稀缺、色带级慷慨；Inter 700 负字距承担全部层级，码字 JetBrains Mono；
 * 无投影，深度全靠黑画布与微亮卡的明度对比 + 1px 发丝线。
 * 画布明暗勘误：站内目录对 ClickHouse 的亮色描述与文档实证相反——规格画布
 * #0a0a0a 为近纯黑（「The system has no light-mode marketing surface」），按文档定暗色。
 * CSS 令牌见 themes/css/clickhouse.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor 取 --canvas（#0a0a0a；首屏防白闪映射同值）。
 */
export const clickhouseTheme: ThemeDefinition = {
  id: 'clickhouse',
  label: 'ClickHouse',
  family: 'dark',
  isDark: true,
  chromeColor: '#0a0a0a',
  swatches: clickhouseSwatches,
  antd: clickhouseAntdTheme,
  slots: clickhouseSlots,
}
