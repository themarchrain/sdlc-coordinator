import type { ThemeSlots } from '../types'

/**
 * xai 主题的语义色槽位 —— 自 design/xai 各页面/组件的硬编码誊抄。
 * 取值 = xAI 点缀组：sunset #ff7a17 / sunset-soft #ffc285 / dusk #7c3aed /
 * twilight #c4b5fd / breeze #a0c3ec（趋势线是点缀组的合法使用场景）。
 * 时间线沿用源里的 antd preset 名（red/orange/green/blue），由
 * darkAlgorithm 渲染为暗色变体 —— 源码即如此，不改写为 hex。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const xaiSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#ff7a17',       // OverviewPage 已创建 MR（sunset）
      crDone: '#a0c3ec',   // OverviewPage 已完成 CR（breeze）
      crStart: '#7c3aed',  // OverviewPage 已发起 CR（dusk）
      avgDuration: '#ffc285', // OverviewPage 成功运行平均耗时（sunset-soft）
    },
    ratios: {
      success: '#a0c3ec',      // 当前任务成功占比（breeze）
      firstRun: '#c4b5fd',     // 首次执行成功率（twilight）
      intervention: '#ff7a17', // 触发人工介入率（sunset）
      recovery: '#ffc285',     // 受阻后恢复成功率（sunset-soft）
    },
  },
  timeline: {
    error: 'red',     // failed / blocked / unavailable（preset，darkAlgorithm 派生暗色）
    warning: 'orange', // human_required / retry_pending / waiting_upstream
    success: 'green', // succeeded / approved
    info: 'blue',     // 其余默认
  },
  eventType: {
    error: 'red',    // codex_error / claude_error
    usage: 'green',  // codex_usage
    info: 'blue',    // 其余默认
  },
  sparkline: {
    default: '#ff7a17',  // Sparkline 默认描线（sunset）
    hoverGrid: '#363a3f', // hover 竖向网格虚线（canvas-mid）
  },
}
