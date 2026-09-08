import type { ThemeSlots } from '../types'

/**
 * pinterest 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格语义段：error #9E0A0A / error-deep #CC001F / success-deep #103C25 均为
 * 字面值；warning 未给 → 派生暖金 #8C651A（与奶油中性同调）。
 * 彩色纪律：Pinterest Red #E60023 只落在头部指标（mr / firstRun），
 * 其余走暖中性（charcoal/ash）与编辑 accent（accent-purple-deep #6845AB
 * 「editorial badge」语汇）；info 一律焦点蓝 #435EE5（规格唯一 chrome 蓝）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const pinterestSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#E60023',      // 每日创建 MR = Pinterest Red（头部指标独占品牌红）
      crDone: '#262622',  // 每日完成 CR = Charcoal（暖近黑）
      crStart: '#6845AB', // 每日发起 CR = accent-purple-deep（编辑紫，规格「editorial badge」）
      avgDuration: '#91918C', // 每日成功运行平均耗时 = Ash（最轻可读中性）
    },
    ratios: {
      success: '#103C25',     // 当前任务成功占比 = success-deep（规格字面值）
      firstRun: '#E60023',    // 首次执行成功率 = Pinterest Red
      intervention: '#9E0A0A', // 触发人工介入率 = Error（规格字面值）
      recovery: '#8C651A',    // 受阻后恢复成功率 = 派生暖金
    },
  },
  timeline: {
    error: '#9E0A0A',   // failed / blocked / unavailable
    warning: '#8C651A', // human_required / retry_pending / waiting_upstream
    success: '#103C25', // succeeded / approved
    info: '#435EE5',    // 其余默认 = focus-outer 蓝（规格唯一 chrome 蓝）
  },
  eventType: {
    error: '#9E0A0A',  // codex_error / claude_error
    usage: '#103C25',  // codex_usage = success-deep
    info: '#435EE5',   // 其余默认
  },
  sparkline: {
    default: '#E60023',   // Sparkline 默认描线 = 品牌红（数据墨水是红的功能性使用）
    hoverGrid: '#E5E5E0', // hover 竖向网格虚线 = hairline-soft
  },
}
