import type { ThemeSlots } from '../types'

/**
 * runway 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格（「Semantic」）明言：Runway 不引入信号色（红/绿/黄），
 * 校验只靠边线与文案——但后台图表/时间线必须可分辨，因此按 tesla 先例
 * 取「标准语义色的低饱和化」：
 *   绿 #2F7A4D / 琥珀 #946F12 / 红 #B3261E
 * 黑 = 唯一交互色（primary #000000），但时间线默认节点回落 ink-soft #1A1A1A
 * （规格 nav-link「one click softer than ink」——常态节点不抢黑药丸的戏）。
 * 趋势线走「黑 + 灰阶」四级（Ink/Graphite/Slate/Stone），呼应
 * monochrome-plus-photography 的极端纪律，同时保证纸白画布上的可分辨性。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const runwaySlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',      // 每日创建 MR = 黑（头部指标独占唯一行动色）
      crDone: '#404040',  // 每日完成 CR = Graphite
      crStart: '#676F7B', // 每日发起 CR = Slate（唯一带色温的中性）
      avgDuration: '#939393', // 每日成功运行平均耗时 = Stone
    },
    ratios: {
      success: '#2F7A4D',      // 当前任务成功占比 = 派生语义绿
      firstRun: '#000000',     // 首次执行成功率 = 黑（交互信号位）
      intervention: '#B3261E', // 触发人工介入率 = 派生语义红
      recovery: '#946F12',     // 受阻后恢复成功率 = 派生语义琥珀
    },
  },
  timeline: {
    error: '#B3261E',   // failed / blocked / unavailable
    warning: '#946F12', // human_required / retry_pending / waiting_upstream
    success: '#2F7A4D', // succeeded / approved
    info: '#1A1A1A',    // 其余默认 = ink-soft（黑药丸弱一档，不抢主行动色）
  },
  eventType: {
    error: '#B3261E',  // codex_error / claude_error
    usage: '#2F7A4D',  // codex_usage
    info: '#1A1A1A',   // 其余默认
  },
  sparkline: {
    default: '#030303',  // Sparkline 默认描线 = 墨（数据墨水即黑墨）
    hoverGrid: '#C9CCD1', // hover 竖向网格虚线 = hairline-soft 规线
  },
}
