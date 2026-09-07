import type { ThemeSlots } from '../types'

/**
 * ollama 主题的语义色槽位 —— 自 design/ollama 各页面/组件的硬编码誊抄。
 * Ollama 数据面 = 白纸黑墨：源 TSX 删除了全部 trendColor prop，
 * 八条趋势线与四张占比卡统一 Sparkline 默认数据墨 #000000。
 * 生命周期/事件时间线 = 终端红绿灯三色 + 中性 charcoal（无蓝）：
 *   TasksPage.timelineColor     → #ff5f56 / #ffbd2e / #27c93f / #525252
 *   CodexObservabilityPage.eventTypeColor → #ff5f56 / #27c93f / #525252
 *   Sparkline 默认 #000000，hover 参考线 #d4d4d4（hairline-strong）
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const ollamaSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',         // 每日创建 MR（源移除 trendColor，单色数据墨）
      crDone: '#000000',     // 每日完成 CR
      crStart: '#000000',    // 每日发起 CR
      avgDuration: '#000000', // 每日成功运行平均耗时
    },
    ratios: {
      success: '#000000',      // 当前任务成功占比
      firstRun: '#000000',     // 首次执行成功率
      intervention: '#000000', // 触发人工介入率
      recovery: '#000000',     // 受阻后恢复成功率
    },
  },
  timeline: {
    error: '#ff5f56',   // failed / blocked / unavailable（terminal-red）
    warning: '#ffbd2e', // human_required / retry_pending / waiting_upstream（terminal-yellow）
    success: '#27c93f', // succeeded / approved（terminal-green）
    info: '#525252',    // 其余默认（charcoal，单色体系无蓝）
  },
  eventType: {
    error: '#ff5f56', // codex_error / claude_error（terminal-red）
    usage: '#27c93f', // codex_usage（terminal-green）
    info: '#525252',  // 其余默认（含 claude_result，charcoal）
  },
  sparkline: {
    default: '#000000',  // Sparkline 默认描线（源组件默认参数）
    hoverGrid: '#d4d4d4', // hover 竖向网格虚线（源硬编码 hairline-strong）
  },
}
