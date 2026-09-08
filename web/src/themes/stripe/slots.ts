import type { ThemeSlots } from '../types'

/**
 * stripe 主题的语义色槽位 —— 自设计语言 调色板派生。
 *
 * 规格明言营销系统没有独立语义色盘（「error/success states live in
 * dashboard-product UI」），且禁令要求不引入渐变停靠点之外的新点缀色
 * （cream/orange/lavender/indigo/ruby/magenta）。管理台的图表/时间线是
 * dashboard 产品位，按「与调色板同明度带的克制语义」推导：
 *   绿 #0e7d4f（success 推导：调色板无绿，取与 ink #0d253d 同深度的克制绿）
 *   金 #9b6829（warning = 规格 lemon 暖金原值）
 *   红 #ea2261（error = 规格 ruby 原值，文档定位「chart highlight; never a button」）
 *   靛 #533afd / #665efd（info/正向 = 品牌靛蓝一族）
 * 趋势线头牌（每日创建 MR）用品牌靛蓝，次要线 ruby/金保持四色可分辨。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const stripeSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#533afd',     // 每日创建 MR = 品牌靛蓝（头牌指标独占 CTA 色）
      crDone: '#0e7d4f', // 每日完成 CR = 推导克制绿（success 语义）
      crStart: '#ea2261', // 每日发起 CR = ruby（规格「chart highlight」合法位）
      avgDuration: '#9b6829', // 每日成功运行平均耗时 = lemon 暖金
    },
    ratios: {
      success: '#0e7d4f',      // 当前任务成功占比 = 推导克制绿
      firstRun: '#533afd',     // 首次执行成功率 = 品牌靛蓝
      intervention: '#ea2261', // 触发人工介入率 = ruby
      recovery: '#9b6829',     // 受阻后恢复成功率 = lemon 暖金
    },
  },
  timeline: {
    error: '#ea2261',   // failed / blocked / unavailable = ruby
    warning: '#9b6829', // human_required / retry_pending / waiting_upstream = lemon
    success: '#0e7d4f', // succeeded / approved = 推导克制绿
    info: '#533afd',    // 其余默认 = 品牌靛蓝
  },
  eventType: {
    error: '#ea2261',  // codex_error / claude_error = ruby
    usage: '#0e7d4f',  // codex_usage = 推导克制绿（正向信号）
    info: '#533afd',   // 其余默认 = 品牌靛蓝
  },
  sparkline: {
    default: '#533afd',  // Sparkline 默认描线 = 品牌靛蓝（主交互色）
    hoverGrid: '#a8c3de', // hover 竖向网格虚线 = hairline-input 冷调线
  },
}
