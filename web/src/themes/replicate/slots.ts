import type { ThemeSlots } from '../types'

/**
 * replicate 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格语义面：badge-success #2B9A66（running 状态绿）是唯一记名语义色，
 * link 与 primary 同橙；聚焦蓝 rgba(59,130,246,.5) 是功能性色。
 * 橙的稀缺法（「one orange element per viewport」）迁移为图表纪律：
 * 橙只出现在头部 MR 趋势与 firstRun 两条线（数据墨水印戳），
 * 默认 Sparkline 描线 = 墨 #202020（单色 + 一枚橙戳 = 规格「stamp」气质）；
 * 默认时间线节点用中性 Charcoal，蓝只留给 focus/链接职。
 * warning/error 规格未给：按暖调推导 ochre #8A6A1E / 红 #CF222E
 * （均与主橙 #EA2804 拉开明度与色相，避免语义撞色）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const replicateSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#EA2804',      // 每日创建 MR = 橙戳记（头部指标的唯一品牌印）
      crDone: '#202020',  // 每日完成 CR = Ink
      crStart: '#575757', // 每日发起 CR = Charcoal
      avgDuration: '#8D8D8D', // 每日成功运行平均耗时 = Ash
    },
    ratios: {
      success: '#2B9A66',   // 当前任务成功占比 = 规格 badge-success
      firstRun: '#EA2804',  // 首次执行成功率 = 橙（与 MR 线同职的数据印）
      intervention: '#CF222E', // 触发人工介入率 = 派生红
      recovery: '#8A6A1E',  // 受阻后恢复成功率 = 派生 ochre
    },
  },
  timeline: {
    error: '#CF222E',   // failed / blocked / unavailable（派生红）
    warning: '#8A6A1E', // human_required / retry_pending / waiting_upstream
    success: '#2B9A66', // succeeded / approved（规格 badge-success）
    info: '#575757',    // 其余默认 = 中性 Charcoal（橙稀缺 + 蓝只留 focus/链接）
  },
  eventType: {
    error: '#CF222E', // codex_error / claude_error
    usage: '#2B9A66', // codex_usage = running 状态绿（规格语义）
    info: '#575757',  // 其余默认 = 中性 Charcoal
  },
  sparkline: {
    default: '#202020',  // Sparkline 默认描线 = Ink（单色印刷感，橙只做戳记）
    hoverGrid: '#BBBBBB', // hover 竖向网格虚线 = Stone（规格 disabled/neutral 档）
  },
}
