import type { ThemeSlots } from '../types'

/**
 * wise 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格纪律（规格 Do）：Wise 绿 #9fe870 是 CTA 专色，
 * 「never repurpose Wise green as success indicator since it IS the brand CTA」
 * → 绿分两族：品牌绿只做趋势主色，且 lime 压深到描线级 #53a626（推导：
 *   #9fe870 在白卡上对比仅 1.5:1，压深到 ≥3:1 的同 hue 橄榄绿）；
 * 成功语义一律 positive #2ead4b。墨阶 #0e0f0c/#454745/#868685 承担其余曲线。
 * warning 取 warning-deep #b86700（#ffd11a 黄只作 badge 面，白底描线/节点不可读，推导取舍）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const wiseSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#53a626',          // 每日创建 MR = 描线级 Wise 绿（lime 压深推导值；头部指标独占品牌绿）
      crDone: '#0e0f0c',      // 每日完成 CR = ink（暖黑主曲线）
      crStart: '#454745',     // 每日发起 CR = body
      avgDuration: '#868685', // 每日成功运行平均耗时 = mute
    },
    ratios: {
      success: '#2ead4b',      // 当前任务成功占比 = positive（规格：品牌绿不兼成功色）
      firstRun: '#0e0f0c',     // 首次执行成功率 = ink
      intervention: '#d03238', // 触发人工介入率 = negative
      recovery: '#b86700',     // 受阻后恢复成功率 = warning-deep（推导：黄提亮态压深到描线级）
    },
  },
  timeline: {
    error: '#d03238',   // failed / blocked / unavailable = negative
    warning: '#b86700', // human_required / retry_pending / waiting_upstream = warning-deep
    success: '#2ead4b', // succeeded / approved = positive
    info: '#454745',    // 其余默认 = body 墨
  },
  eventType: {
    error: '#d03238',  // codex_error / claude_error
    usage: '#53a626',  // codex_usage = 描线级 Wise 绿（用量 = 品牌绿的合法功能位）
    info: '#454745',   // 其余默认
  },
  sparkline: {
    default: '#53a626',   // Sparkline 默认描线 = 描线级 Wise 绿
    hoverGrid: '#b9beb4', // hover 竖向网格虚线 = sage 强线（与 --hairline-strong 同源）
  },
}
