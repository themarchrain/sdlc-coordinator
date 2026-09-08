import type { ThemeSlots } from '../types'

/**
 * revolut 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格产品宽色盘的暗底可用子集：cobalt-violet #494FDF（品牌主线）+
 * teal #00A87E / light-blue #007BC2 / warning orange #EC7E00 / danger red #E23B4A。
 * 规格约定宽色盘「只活在产品图形与插画里，绝不做按钮面」——图表描线与时间线
 * 节点正是管理台里的「产品图形」语汇，因此整套槽位按此纪律取色。
 * 暗面 (#0A0A0A well / #16181A 卡) 上的可分辨性已逐色核对（≥3:1 描线对比）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const revolutSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#494fdf',        // 每日创建 MR = cobalt-violet（头部指标独占品牌色）
      crDone: '#00a87e',    // 每日完成 CR = accent-teal
      crStart: '#007bc2',   // 每日发起 CR = accent-light-blue
      avgDuration: '#ec7e00', // 成功运行平均耗时 = warning orange（耗时即警示）
    },
    ratios: {
      success: '#00a87e',     // 当前任务成功占比 = teal
      firstRun: '#494fdf',    // 首次执行成功率 = cobalt-violet（与趋势主色同族）
      intervention: '#e23b4a', // 触发人工介入率 = accent-danger
      recovery: '#ec7e00',    // 受阻后恢复成功率 = warning orange
    },
  },
  timeline: {
    error: '#e23b4a',   // failed / blocked / unavailable = accent-danger
    warning: '#ec7e00', // human_required / retry_pending / waiting_upstream = accent-warning
    success: '#00a87e', // succeeded / approved = accent-teal（暗面成功色）
    info: '#007bc2',    // 其余默认 = accent-light-blue
  },
  eventType: {
    error: '#e23b4a',  // codex_error / claude_error
    usage: '#00a87e',  // codex_usage
    info: '#007bc2',   // 其余默认
  },
  sparkline: {
    default: '#494fdf', // Sparkline 默认描线 = cobalt-violet（#0A0A0A well 上 3.3:1）
    hoverGrid: '#3a3d40', // hover 竖向网格虚线 = charcoal（规格中性阶，暗底可见）
  },
}
