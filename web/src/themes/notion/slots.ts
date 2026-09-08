import type { ThemeSlots } from '../types'

/**
 * notion 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格（「Semantic」）：Notion 营销面不设专用 error/success 色板，
 * 状态由贴纸彩盘承担（原文示例：accent-green 作 affirmative ticks）：
 *   成功 = Sticker Green #1AAE39（规格指定）
 *   警示 = Sticker Orange #DD5B00（派生：贴纸盘中最接近警示的暖色）
 *   失败 = #E03E3A（派生：Notion 产品内危险红，贴纸级饱和度）
 * 蓝 #0075DE = 唯一结构性交互色，info/默认节点一律回落此蓝。
 * 趋势线走「蓝 + 墨阶」（Ink/Stone/Ash），呼应 monochrome-plus-one 纪律，
 * 同时保证白卡上的可分辨性（规格 Do：贴纸色不得刷结构，数据墨水保持克制）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const notionSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#0075DE',      // 每日创建 MR = Notion Blue（头部指标独占唯一结构色）
      crDone: '#31302E',  // 每日完成 CR = Warm Charcoal
      crStart: '#615D59', // 每日发起 CR = Stone
      avgDuration: '#A39E98', // 每日成功运行平均耗时 = Ash
    },
    ratios: {
      success: '#1AAE39',      // 当前任务成功占比 = 贴纸绿（规格 affirmative tick）
      firstRun: '#0075DE',     // 首次执行成功率 = Notion Blue
      intervention: '#E03E3A', // 触发人工介入率 = 派生危险红
      recovery: '#DD5B00',     // 受阻后恢复成功率 = 贴纸橙
    },
  },
  timeline: {
    error: '#E03E3A',   // failed / blocked / unavailable
    warning: '#DD5B00', // human_required / retry_pending / waiting_upstream
    success: '#1AAE39', // succeeded / approved（贴纸绿）
    info: '#0075DE',    // 其余默认 = 唯一结构蓝
  },
  eventType: {
    error: '#E03E3A',  // codex_error / claude_error
    usage: '#1AAE39',  // codex_usage（贴纸绿）
    info: '#0075DE',   // 其余默认
  },
  sparkline: {
    default: '#0075DE',  // Sparkline 默认描线 = Notion Blue
    hoverGrid: '#E6E6E6', // hover 竖向网格虚线 = 规格发丝线
  },
}
