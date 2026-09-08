import type { ThemeSlots } from '../types'

/**
 * lovable 主题的语义色槽位 —— 自设计语言 正文角色推导。
 *
 * 规格刻意回避彩色：「Don't introduce saturated accent colors — the palette
 * is intentionally warm-neutral」。系统里唯一的功能性彩色是 Ring Blue
 * #3b82f6（Interactive「Tailwind focus ring」），因此：
 *   - info / 默认节点 / 头部指标一律回落 Ring Blue（唯一合法彩色）；
 *   - 语义三色按规格自带技术栈（末行「shadcn/ui + Radix UI + Tailwind CSS」）
 *     取 Tailwind 700 文字级克制档：绿 #15803d / 琥珀 #b45309 / 红 #b91c1c；
 *   - 其余数据线走炭墨 + Muted Gray 双阶（透明度灰阶的图表投影），
 *     呼应「monochrome warm-neutral」的极端纪律，同时保证奶油底上的可分辨性。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const lovableSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#3b82f6',         // 每日创建 MR = Ring Blue（头部指标独占唯一彩色）
      crDone: '#1c1c1c',     // 每日完成 CR = Charcoal 100%
      crStart: '#5f5f5d',    // 每日发起 CR = Muted Gray
      avgDuration: '#b45309', // 每日成功运行平均耗时 = 语义琥珀（耗时=时间隐喻）
    },
    ratios: {
      success: '#15803d',     // 当前任务成功占比 = 语义绿
      firstRun: '#3b82f6',    // 首次执行成功率 = Ring Blue
      intervention: '#b91c1c', // 触发人工介入率 = 语义红
      recovery: '#b45309',    // 受阻后恢复成功率 = 语义琥珀
    },
  },
  timeline: {
    error: '#b91c1c',   // failed / blocked / unavailable
    warning: '#b45309', // human_required / retry_pending / waiting_upstream
    success: '#15803d', // succeeded / approved
    info: '#3b82f6',    // 其余默认 = Ring Blue（唯一功能性彩色）
  },
  eventType: {
    error: '#b91c1c', // codex_error / claude_error
    usage: '#15803d', // codex_usage
    info: '#3b82f6',  // 其余默认
  },
  sparkline: {
    default: '#1c1c1c',           // Sparkline 默认描线 = 炭墨数据墨水（无彩纪律）
    hoverGrid: 'rgba(28, 28, 28, 0.4)', // hover 竖向网格虚线 = 炭墨 40% 交互边线（字面）
  },
}
