import type { ThemeSlots } from '../types'

/**
 * ibm 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：Carbon 语义四色 = green-50 #24A148 /
 * yellow-30 #F1C21B / red-60 #DA1E28 / info 与 primary 同为 #0F62FE。
 * 两处推导偏离（均在行内注明）：
 *  - yellow-30 #F1C21B 只在「图形位」（时间线节点圆点）直用；线描/文字位
 *    （sparkline 描线、antd 文字色）压深为 #96780E —— Carbon 自身也只把
 *    yellow-30 当图标/图形色，正文不用它。
 *  - 趋势线走「IBM Blue + 炭灰阶」四级（#0F62FE + ink/ink-muted/ink-subtle），
 *    呼应「One accent color」纪律，同时保证白画布上四条线的可分辨性。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const ibmSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#0F62FE',          // 每日创建 MR = IBM Blue（头部指标独占唯一彩色）
      crDone: '#161616',      // 每日完成 CR = ink 炭墨
      crStart: '#525252',     // 每日发起 CR = ink-muted
      avgDuration: '#8C8C8C', // 每日成功运行平均耗时 = ink-subtle
    },
    ratios: {
      success: '#24A148',      // 当前任务成功占比 = green-50
      firstRun: '#0F62FE',     // 首次执行成功率 = IBM Blue
      intervention: '#DA1E28', // 触发人工介入率 = red-60
      recovery: '#96780E',     // 受阻后恢复成功率 = yellow-30 压深（线描可读）
    },
  },
  timeline: {
    error: '#DA1E28',   // failed / blocked / unavailable = red-60
    warning: '#F1C21B', // human_required / retry_pending / waiting_upstream = yellow-30（图形位直用）
    success: '#24A148', // succeeded / approved = green-50
    info: '#0F62FE',    // 其余默认 = IBM Blue
  },
  eventType: {
    error: '#DA1E28', // codex_error / claude_error
    usage: '#24A148', // codex_usage
    info: '#0F62FE',  // 其余默认
  },
  sparkline: {
    default: '#0F62FE',   // Sparkline 默认描线 = IBM Blue
    hoverGrid: '#E0E0E0', // hover 竖向网格虚线 = hairline 发丝线
  },
}
