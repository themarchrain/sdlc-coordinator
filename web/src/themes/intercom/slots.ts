import type { ThemeSlots } from '../types'

/**
 * intercom 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：Intercom 的分析图表色 =
 * in-product report 色板（report-blue/green/pink/lime/cyan）——趋势线、
 * 占比卡本就是「产品内分析表面」，故四条趋势/四张占比直接取该色板；
 * 语义态取 semantic 字面：error #C41C1C / success #0BDF50。
 * Fin Orange #FF5600 不入图表（规格 Do：「Fin Orange ONLY on Fin CTAs/badges」）。
 * 品牌蓝 #0007CB 承担默认/头部数据位；warning 无规格 → 克制琥珀 #A16207（派生）。
 * report-lime #B3E01C 在白卡上对比不足（≈1.9:1），弃用并注明。
 * 键结构与 tesla/slots.ts 保持一致，只改值。
 */
export const intercomSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#0007CB',      // 每日创建 MR = Brand Blue（头部指标独占品牌蓝）
      crDone: '#0BDF50',  // 每日完成 CR = report green（= semantic-success）
      crStart: '#65B5FF', // 每日发起 CR = report blue
      avgDuration: '#03B2CB', // 每日成功运行平均耗时 = report cyan（lime 过浅弃用）
    },
    ratios: {
      success: '#0BDF50',      // 当前任务成功占比 = semantic-success（规格 字面）
      firstRun: '#0007CB',     // 首次执行成功率 = Brand Blue
      intervention: '#C41C1C', // 触发人工介入率 = semantic-error（规格 字面）
      recovery: '#03B2CB',     // 受阻后恢复成功率 = report cyan（推导：正语义且与红错开）
    },
  },
  timeline: {
    error: '#C41C1C',   // failed / blocked / unavailable = semantic-error
    warning: '#A16207', // human_required / retry_pending / waiting_upstream（派生琥珀）
    success: '#0BDF50', // succeeded / approved = semantic-success
    info: '#0007CB',    // 其余默认 = Brand Blue
  },
  eventType: {
    error: '#C41C1C',  // codex_error / claude_error = semantic-error
    usage: '#0BDF50',  // codex_usage = report green（用量正语义）
    info: '#0007CB',   // 其余默认 = Brand Blue
  },
  sparkline: {
    default: '#111111',   // Sparkline 默认描线 = 炭墨（系统主色，编辑感单墨描线）
    hoverGrid: '#D3CEC6', // hover 竖向网格虚线 = Hairline
  },
}
