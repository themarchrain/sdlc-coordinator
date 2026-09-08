import type { ThemeSlots } from '../types'

/**
 * zapier 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：Zapier 营销面「不单列语义色板，状态/校验向
 * 墨 + 橙阶层借」——因此图表/时间线以「橙 + 咖啡墨阶」为主干：
 *   #FF4F00 主橙 / #201515 咖啡墨 / #605D52 Body / #939084 Body Mid。
 * 语义三色为本报告的克制化派生（规格无字面）：暖绿 #2F7A3C /
 * 琥珀 #9A6A10 / 暖红 #B3261E（均向暖温对齐，与主橙拉开明度）。
 * 键结构与 tesla/slots.ts 保持一致，只改值。
 */
export const zapierSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#FF4F00',      // 每日创建 MR = Zapier 橙（头部指标独占唯一彩色）
      crDone: '#201515',  // 每日完成 CR = 深咖啡墨
      crStart: '#605D52', // 每日发起 CR = Body
      avgDuration: '#939084', // 每日成功运行平均耗时 = Body Mid
    },
    ratios: {
      success: '#2F7A3C',      // 当前任务成功占比 = 派生暖绿（唯一语义放宽处）
      firstRun: '#FF4F00',     // 首次执行成功率 = Zapier 橙
      intervention: '#B3261E', // 触发人工介入率 = 派生暖红
      recovery: '#9A6A10',     // 受阻后恢复成功率 = 派生琥珀（受阻→恢复的中继语义）
    },
  },
  timeline: {
    error: '#B3261E',   // failed / blocked / unavailable = 派生暖红
    warning: '#9A6A10', // human_required / retry_pending / waiting_upstream = 派生琥珀
    success: '#2F7A3C', // succeeded / approved = 派生暖绿
    info: '#FF4F00',    // 其余默认 = Zapier 橙（唯一彩色 = 唯一交互信号）
  },
  eventType: {
    error: '#B3261E',  // codex_error / claude_error = 派生暖红
    usage: '#FF4F00',  // codex_usage = Zapier 橙（用量即转化数据）
    info: '#201515',   // 其余默认 = 深咖啡墨
  },
  sparkline: {
    default: '#FF4F00',   // Sparkline 默认描线 = Zapier 橙
    hoverGrid: '#C5C0B1', // hover 竖向网格虚线 = Mute 暖线
  },
}
