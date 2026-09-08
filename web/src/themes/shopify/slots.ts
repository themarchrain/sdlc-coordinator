import type { ThemeSlots } from '../types'

/**
 * shopify 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 规格无语义色码：事务轨的绿（aloe #c1fbd4 / pistachio #d4f9e0）被明言
 * 「signal commerce, growth, transactional success」且「不许放在字后」→
 * 图表/时间线的绿色取 aloe 同 hue 深化到描线级 #17824f（推导值）；
 * 黑 #000000 即唯一交互墨色，承担趋势主色与 info 兜底；
 * 灰阶走 shade ladder（#52525b/#71717a/#a1a1aa），
 * warning/error 取标准语义克制化 #96660a/#b3372e（推导值）。
 * 白/乳白面上的可分辨性已逐色核对（描线对比 ≥3:1）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const shopifySlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',        // 每日创建 MR = 交互墨色（黑药丸同源，头部指标独占纯黑）
      crDone: '#17824f',    // 每日完成 CR = 深化 aloe 绿（事务成功色）
      crStart: '#71717a',   // 每日发起 CR = shade-50（次级曲线）
      avgDuration: '#a1a1aa', // 每日成功运行平均耗时 = shade-40（弱曲线）
    },
    ratios: {
      success: '#17824f',   // 当前任务成功占比 = 深化 aloe 绿
      firstRun: '#000000',  // 首次执行成功率 = 交互墨色
      intervention: '#b3372e', // 触发人工介入率 = 语义红（推导）
      recovery: '#96660a',  // 受阻后恢复成功率 = 语义琥珀（推导）
    },
  },
  timeline: {
    error: '#b3372e',   // failed / blocked / unavailable
    warning: '#96660a', // human_required / retry_pending / waiting_upstream
    success: '#17824f', // succeeded / approved = 事务成功绿
    info: '#52525b',    // 其余默认 = shade-60 墨系（规格无第二色相）
  },
  eventType: {
    error: '#b3372e',  // codex_error / claude_error
    usage: '#17824f',  // codex_usage = 事务成功绿
    info: '#52525b',   // 其余默认
  },
  sparkline: {
    default: '#000000', // Sparkline 默认描线 = 交互墨色
    hoverGrid: '#d4d4d8', // hover 竖向网格虚线 = shade-30（足见度强线）
  },
}
