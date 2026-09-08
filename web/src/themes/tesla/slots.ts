import type { ThemeSlots } from '../types'

/**
 * tesla 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格（Semantic）：Tesla 营销站回避语义色码，error/success/warning 跟随
 * 「标准浏览器默认」——迁移为标准语义色的克制化（低饱和、文字级对比度）：
 *   绿 #1F7A3D / 琥珀 #A16207 / 红 #C8102E
 * 蓝 = 唯一交互色 Electric Blue #3E6AE1，info/默认节点一律回落此蓝。
 * 趋势线走「蓝 + 墨阶」四级（Carbon/Pewter/Silver Fog），呼应
 * monochrome-plus-one 的极端纪律，同时保证白画布上的可分辨性。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const teslaSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#3E6AE1',      // 每日创建 MR = Electric Blue（头部指标独占唯一彩色）
      crDone: '#171A20',  // 每日完成 CR = Carbon Dark
      crStart: '#5C5E62', // 每日发起 CR = Pewter
      avgDuration: '#8E8E8E', // 每日成功运行平均耗时 = Silver Fog
    },
    ratios: {
      success: '#1F7A3D',      // 当前任务成功占比 = 语义绿
      firstRun: '#3E6AE1',     // 首次执行成功率 = Electric Blue
      intervention: '#C8102E', // 触发人工介入率 = 语义红
      recovery: '#A16207',     // 受阻后恢复成功率 = 语义琥珀
    },
  },
  timeline: {
    error: '#C8102E',   // failed / blocked / unavailable
    warning: '#A16207', // human_required / retry_pending / waiting_upstream
    success: '#1F7A3D', // succeeded / approved
    info: '#3E6AE1',    // 其余默认 = 唯一交互蓝
  },
  eventType: {
    error: '#C8102E',  // codex_error / claude_error
    usage: '#1F7A3D',  // codex_usage
    info: '#3E6AE1',   // 其余默认
  },
  sparkline: {
    default: '#3E6AE1',  // Sparkline 默认描线 = 唯一交互蓝
    hoverGrid: '#D0D1D2', // hover 竖向网格虚线 = Pale Silver 发丝线
  },
}
