import type { ThemeSlots } from '../types'

/**
 * apple 主题的语义色槽位 —— 自 design/apple 各页面/组件的硬编码誊抄。
 * 取值 = Apple 系统色：绿 #34c759 / 橙 #ff9500 / 红 #ff3b30 / 青 #30b0c7 /
 * 紫 #af52de / 靛 #5856d6，其余一律回落唯一交互色 Action Blue #0066cc。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const appleSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#34c759',       // OverviewPage 已创建 MR
      crDone: '#30b0c7',   // OverviewPage 已完成 CR
      crStart: '#af52de',  // OverviewPage 已发起 CR
      avgDuration: '#ff3b30', // OverviewPage 成功运行平均耗时
    },
    ratios: {
      success: '#34c759',      // 当前任务成功占比
      firstRun: '#5856d6',    // 首次执行成功率
      intervention: '#ff3b30', // 触发人工介入率
      recovery: '#ff9500',     // 受阻后恢复成功率
    },
  },
  timeline: {
    error: '#ff3b30',   // failed / blocked / unavailable
    warning: '#ff9500', // human_required / retry_pending / waiting_upstream
    success: '#34c759', // succeeded / approved
    info: '#0066cc',    // 其余默认
  },
  eventType: {
    error: '#ff3b30',  // codex_error / claude_error
    usage: '#34c759',  // codex_usage
    info: '#0066cc',   // 其余默认
  },
  sparkline: {
    default: '#0066cc',  // Sparkline 默认描线
    hoverGrid: '#d2d2d7', // hover 竖向网格虚线
  },
}
