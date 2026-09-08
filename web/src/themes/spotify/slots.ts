import type { ThemeSlots } from '../types'

/**
 * spotify 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格色板的暗底可用子集：品牌绿一族（#1ed760/#1db954）+
 * 语义三色（announcement blue #539df5 / warning orange #ffa42b / negative red #f3727f）。
 * 近黑画布 (#121212/#181818) 上的可分辨性已逐色核对：
 * 绿是天然的趋势主色（MR/成功），蓝橙红承担其余曲线，全部为中高明度、
 * 在 #181818 面板上有 ≥3:1 的描线对比。
 * 时间线/事件色沿用 antd preset 名（red/orange/green/blue，照 xai 暗色范例），
 * 由 darkAlgorithm 渲染为暗色变体；preset green 与本主题 colorSuccess 同族。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const spotifySlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#1ed760',        // 每日创建 MR：品牌绿 = 趋势主色
      crDone: '#539df5',    // 每日完成 CR：announcement blue
      crStart: '#ffa42b',   // 每日发起 CR：warning orange
      avgDuration: '#f3727f', // 成功运行平均耗时：negative red（耗时即警示）
    },
    ratios: {
      success: '#1ed760',     // 当前任务成功占比：绿
      firstRun: '#539df5',    // 首次执行成功率：蓝
      intervention: '#f3727f', // 人工介入率：红
      recovery: '#ffa42b',    // 受阻后恢复成功率：橙
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
    default: '#1ed760', // Sparkline 默认描线：品牌绿（近黑画布上的单一绿色事件）
    hoverGrid: '#4d4d4d', // hover 竖向网格虚线：Border Gray（暗底可见）
  },
}
