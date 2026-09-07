import type { ThemeSlots } from '../types'

/**
 * vercel 主题的语义色槽位 —— 自 design/vercel 各页面/组件的硬编码誊抄。
 * 黑白体系：OverviewPage 8 处 trendColor 全部是单色数据墨 #171717；
 * timeline 源四色并三色 —— 交互蓝 #0070f3 承担成功，其余默认节点同样
 * 回落蓝（源 timelineColor 的 fallback 即 #0070f3）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const vercelSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#171717',          // OverviewPage 每日创建 MR
      crDone: '#171717',      // OverviewPage 每日完成 CR
      crStart: '#171717',     // OverviewPage 每日发起 CR
      avgDuration: '#171717', // OverviewPage 成功运行平均耗时
    },
    ratios: {
      success: '#171717',      // 当前任务成功占比
      firstRun: '#171717',     // 首次执行成功率
      intervention: '#171717', // 触发人工介入率
      recovery: '#171717',     // 受阻后恢复成功率
    },
  },
  timeline: {
    error: '#ee0000',   // failed / blocked / unavailable
    warning: '#f5a623', // human_required / retry_pending / waiting_upstream
    success: '#0070f3', // succeeded / approved —— 蓝承担成功
    info: '#0070f3',    // 其余默认节点（源 fallback 同为蓝）
  },
  eventType: {
    error: '#ee0000', // codex_error / claude_error
    usage: '#0070f3', // codex_usage
    info: '#0070f3',  // 其余默认（源非 error 一律蓝）
  },
  sparkline: {
    default: '#171717',  // Sparkline 默认描线（数据墨）
    hoverGrid: '#8f8f8f', // hover 竖向网格虚线（源 var(--mute) 的解析值）
  },
}
