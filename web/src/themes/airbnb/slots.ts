import type { ThemeSlots } from '../types'

/**
 * airbnb 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格（「Semantic」）：只定义了 error #C13515（比 Rausch 更暗更饱和的
 * 独立红）与 legal-link #428BFF；无 success/warning 色板 → 派生旅行语境色：
 *   成功 = #008A70 青绿（黄星在旅行语境显廉价，评分一律墨色 → 绿走青调）
 *   警示 = #B7791F 暖金（等待/重试态）
 * Rausch #FF385C = 品牌电压，只落在头部指标与默认节点（one or two Rausch moments）；
 * 其余趋势线走「Rausch + 墨阶」（Ink/Muted/Muted-Soft），呼应 90% 白 + 墨的克制。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const airbnbSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#FF385C',      // 每日创建 MR = Rausch（头部指标独占品牌电压）
      crDone: '#222222',  // 每日完成 CR = Ink
      crStart: '#6A6A6A', // 每日发起 CR = Muted
      avgDuration: '#929292', // 每日成功运行平均耗时 = Muted-Soft
    },
    ratios: {
      success: '#008A70',      // 当前任务成功占比 = 派生青绿
      firstRun: '#FF385C',     // 首次执行成功率 = Rausch
      intervention: '#C13515', // 触发人工介入率 = 规格 error 红
      recovery: '#B7791F',     // 受阻后恢复成功率 = 派生暖金
    },
  },
  timeline: {
    error: '#C13515',   // failed / blocked / unavailable（规格 error）
    warning: '#B7791F', // human_required / retry_pending / waiting_upstream
    success: '#008A70', // succeeded / approved
    info: '#428BFF',    // 其余默认 = 规格 legal-link 蓝复用
  },
  eventType: {
    error: '#C13515',  // codex_error / claude_error
    usage: '#008A70',  // codex_usage
    info: '#428BFF',   // 其余默认
  },
  sparkline: {
    default: '#FF385C',  // Sparkline 默认描线 = Rausch
    hoverGrid: '#DDDDDD', // hover 竖向网格虚线 = hairline 规格字面值
  },
}
