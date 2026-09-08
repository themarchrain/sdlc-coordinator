import type { ThemeSlots } from '../types'

/**
 * clay 主题的语义色槽位 —— 自设计语言 派生。
 *
 * 规格（迭代指南）：Clay 的彩色只活在 6 色饱和卡家族——
 *   pink #FF4D8B（outbound/sequencer）/ teal #1A3A3A（enterprise/featured）/
 *   lavender #B8A4ED（AI-agent 产品）/ peach #FFB084 / ochre #E8B94A / mint #A4D4C5
 * 迭代指南明示「pink for outbound/sequencer，lavender for AI-agent」，
 * 因此趋势线/占比卡的取色直接套用这套业务-颜色映射：
 *   mr（每日创建 MR，outbound 动作）= pink；crDone（完成）= teal（featured 终态）；
 *   avgDuration（Agent 运行耗时）= lavender（AI-agent）
 * 语义节点仍走 规格 语义三色（#22C55E/#F59E0B/#EF4444），
 * info/默认节点回落 lavender（与 antd colorInfo 同源）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const claySlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#FF4D8B',         // 每日创建 MR = brand pink（outbound/sequencer 语义色）
      crDone: '#1A3A3A',     // 每日完成 CR = brand teal（featured/终态语义色）
      crStart: '#E8B94A',    // 每日发起 CR = brand ochre（第三条饱和卡色，保证四线可分辨）
      avgDuration: '#B8A4ED', // 每日成功运行平均耗时 = brand lavender（AI-agent 语义色）
    },
    ratios: {
      success: '#22C55E',      // 当前任务成功占比 = 语义绿
      firstRun: '#FF4D8B',     // 首次执行成功率 = brand pink（首发触达 = outbound）
      intervention: '#EF4444', // 触发人工介入率 = 语义红
      recovery: '#F59E0B',     // 受阻后恢复成功率 = 语义琥珀
    },
  },
  timeline: {
    error: '#EF4444',   // failed / blocked / unavailable
    warning: '#F59E0B', // human_required / retry_pending / waiting_upstream
    success: '#22C55E', // succeeded / approved
    info: '#B8A4ED',    // 其余默认 = lavender（info 家族同源）
  },
  eventType: {
    error: '#EF4444',  // codex_error / claude_error
    usage: '#22C55E',  // codex_usage
    info: '#B8A4ED',   // 其余默认
  },
  sparkline: {
    default: '#FF4D8B',  // Sparkline 默认描线 = brand pink（品牌电压落点）
    hoverGrid: '#E5E5E5', // hover 竖向网格虚线 = hairline
  },
}
