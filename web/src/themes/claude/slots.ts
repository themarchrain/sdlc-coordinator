import type { ThemeSlots } from '../types'

/**
 * claude 主题的语义色槽位 —— 值为 claude 源 TSX 硬编码原样誊抄：
 *   chart.trends    ← OverviewPage.tsx 4 处 trendColor
 *   chart.ratios    ← OverviewPage.tsx 4 处 trendColor
 *   timeline        ← TasksPage.tsx timelineColor 分支
 *   eventType       ← AgentObservabilityPage.tsx eventTypeColor 分支
 *   sparkline       ← Sparkline.tsx 默认描线色与 hover 虚线色
 * 色彩语言：success 绿 / teal 辅色 / amber 辅色 / 珊瑚 / 暖墨红。
 */
export const claudeSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#5db872',        // 已创建 MR
      crDone: '#5db8a6',    // 已完成 CR（teal）
      crStart: '#e8a55a',   // 已发起 CR（amber）
      avgDuration: '#cc785c', // 平均耗时（珊瑚）
    },
    ratios: {
      success: '#5db872',
      firstRun: '#5db8a6',
      intervention: '#c64545',
      recovery: '#e8a55a',
    },
  },
  timeline: {
    error: '#c64545',       // failed / blocked / unavailable
    warning: '#d4a017',     // human_required / retry_pending / waiting_upstream
    success: '#5db872',     // succeeded / approved
    info: '#5db8a6',        // 其余默认（teal）
  },
  eventType: {
    error: '#c64545',       // codex_error / claude_error
    usage: '#5db872',       // codex_usage
    info: '#5db8a6',        // 其余默认（teal）
  },
  sparkline: {
    default: '#cc785c',     // 珊瑚默认描线（= claude --primary）
    hoverGrid: '#d8cfc0',   // hover 虚线（= claude --hairline-strong）
  },
}
