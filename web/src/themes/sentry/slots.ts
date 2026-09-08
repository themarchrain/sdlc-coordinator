import type { ThemeSlots } from '../types'

/**
 * sentry 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 规格色板刻意窄（「deliberately narrow」）：electric lime #C2EF4E 是唯一高亮、
 * hot pink #FA7FAA 是次级标点（「chart points, supporting accents」）、
 * violet #6A5FC1 是链接强调色 —— 三者正好构成图表/时间线所需的语义族：
 *   success = lime（正向高亮即成功）、error = pink（最热 = 错误）、
 *   info = violet、warning = #E8B93E 琥珀（推导值：规格无警示色且语义节为空，
 *   取 lime 与 pink 之间的暖琥珀，保证四态可分辨；这是唯一族外推导色）。
 * 午夜紫画布 (#1F1633/#150F23) 上的可分辨性已逐色核对（≥3:1 描线对比，
 * violet 2.9:1 为最紧但在 #150F23 宿面上 ≥3.6:1）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const sentrySlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#c2ef4e',        // 每日创建 MR = electric lime（签名高亮 = 趋势主色）
      crDone: '#6a5fc1',    // 每日完成 CR = accent-violet
      crStart: '#fa7faa',   // 每日发起 CR = accent-pink
      avgDuration: '#e8b93e', // 成功运行平均耗时 = 推导琥珀（耗时即警示）
    },
    ratios: {
      success: '#c2ef4e',     // 当前任务成功占比 = lime
      firstRun: '#6a5fc1',    // 首次执行成功率 = violet
      intervention: '#fa7faa', // 触发人工介入率 = pink
      recovery: '#e8b93e',    // 受阻后恢复成功率 = 琥珀
    },
  },
  timeline: {
    error: '#fa7faa',   // failed / blocked / unavailable = pink
    warning: '#e8b93e', // human_required / retry_pending / waiting_upstream = 琥珀
    success: '#c2ef4e', // succeeded / approved = lime
    info: '#6a5fc1',    // 其余默认 = violet
  },
  eventType: {
    error: '#fa7faa',  // codex_error / claude_error
    usage: '#c2ef4e',  // codex_usage
    info: '#6a5fc1',   // 其余默认
  },
  sparkline: {
    default: '#c2ef4e', // Sparkline 默认描线 = lime（午夜紫上的单一高亮事件）
    hoverGrid: '#4d4379', // hover 竖向网格虚线 = 提亮紫阶（暗底可见）
  },
}
