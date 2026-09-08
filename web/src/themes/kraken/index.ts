import type { ThemeDefinition, ThemeSwatches } from '../types'
import { krakenAntdTheme } from './antd'
import { krakenSlots } from './slots'

/** kraken 主题五色示意条（外观面板展示）：白底 + 近黑墨 + 命令紫 + 成功绿。 */
export const krakenSwatches: ThemeSwatches = {
  canvas: '#ffffff',
  surface: '#f1f2f4',
  ink: '#101114',
  primary: '#7132f5',
  accent: '#149e61',
}

/**
 * kraken 主题 = Kraken 加密交易所设计语言（
 * 简化格式规格，色值自正文提取）。
 * 白底 #ffffff + Kraken Purple #7132f5 命令色（变体 #5741d8/#5b1ecf、
 * subtle rgba(133,91,251,.16) 选中薄纱），近黑 #101114 正文 + Cool Gray #686b82 /
 * Silver Blue #9497a9 / Border Gray #dedee5 冷灰中性阶；12px 按钮圆角
 * （rounded but not pill，禁药丸）；whisper 级细影 rgba(0,0,0,.03) 0 4px 24px；
 * 绿 #149e61 = success（16% 徽章底 + 深绿 #026b3f 字）；
 * Kraken-Brand（display 700 负字距）+ Kraken-Product/IBM Plex Sans（UI）双字体
 * （禁外部加载 → 系统栈近似，负字距标题由 css 片段钉住）。
 * 规格未覆盖的中性细分阶与 warning/error 语义色按「冷灰蓝阶 + 克制语义」推导（css 行内注明）。
 * CSS 令牌见 themes/css/kraken.css；antd 对象见 ./antd.ts；语义槽位见 ./slots.ts。
 * chromeColor = #ffffff（「White: Primary surface」，即本主题 --canvas）。
 */
export const krakenTheme: ThemeDefinition = {
  id: 'kraken',
  label: 'Kraken',
  family: 'brand',
  isDark: false,
  chromeColor: '#ffffff',
  swatches: krakenSwatches,
  antd: krakenAntdTheme,
  slots: krakenSlots,
}
