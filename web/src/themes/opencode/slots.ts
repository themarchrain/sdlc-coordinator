import type { ThemeSlots } from '../types'

/**
 * opencode 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：完整 Apple HIG ramp 随系统出厂（accent #007AFF /
 * danger #FF3B30 / warning #FF9F0A / success #30D158），营销面保持单色、
 * 语义色归 in-product TUI —— 管理台的数据面板即 TUI 语境，照单取用：
 *   成功 = #30D158 / 警示 = #FF9F0A / 失败 = #FF3B30 / 信息蓝 = #007AFF
 * 默认节点（info）回落 Mute #646262 而非蓝 —— 规格明言营销 chrome 保持
 * 单色（「marketing chrome stays monochrome」），蓝只承担正向计量与命令
 * 高亮（与 tesla/notion 的 info=蓝惯例的偏离，特此注记）。
 * 趋势线走「蓝 + 灰阶」四级（Ink/Mute/Ash），呼应 manpage 单色纪律，
 * 同时保证奶油画布上的可分辨性。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const opencodeSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#007AFF',    // 每日创建 MR = Apple Blue（TUI 命令高亮色的数据用法）
      crDone: '#201D1D', // 每日完成 CR = Ink
      crStart: '#646262', // 每日发起 CR = Mute
      avgDuration: '#9A9898', // 每日成功运行平均耗时 = Ash
    },
    ratios: {
      success: '#30D158',  // 当前任务成功占比 = HIG 绿
      firstRun: '#007AFF', // 首次执行成功率 = Apple Blue
      intervention: '#FF3B30', // 触发人工介入率 = HIG 红
      recovery: '#FF9F0A', // 受阻后恢复成功率 = HIG 橙
    },
  },
  timeline: {
    error: '#FF3B30',   // failed / blocked / unavailable
    warning: '#FF9F0A', // human_required / retry_pending / waiting_upstream
    success: '#30D158', // succeeded / approved
    info: '#646262',    // 其余默认 = Mute（chrome 单色；偏离 info=蓝惯例）
  },
  eventType: {
    error: '#FF3B30', // codex_error / claude_error
    usage: '#30D158', // codex_usage —— 正向计量 = HIG 绿
    info: '#646262',  // 其余默认 = Mute
  },
  sparkline: {
    default: '#201D1D',  // Sparkline 默认描线 = 数据墨（chart-tile 稀疏线画法）
    hoverGrid: '#9A9898', // hover 竖向网格虚线 = Ash 点线
  },
}
