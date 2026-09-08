import type { ThemeSlots } from '../types'

/**
 * vodafone 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：Vodafone 只运营「红 + 近黑墨 + 灰阶」——
 * 红 #E60000 兼任 destructive/validation 信号（「no second accent colour」），
 * 成功/警告规格未给 → 按系统先例派生为标准语义的克制化：
 *   绿 #0E8A3E / 琥珀 #9A6700
 * info / 默认节点一律回落近黑墨 #25282B（中性墨承担，不引入蓝）。
 * 趋势线走「红 + 墨阶」四级（ink/body/mute），呼应单色加一红的极端纪律，
 * 同时保证白画布上的可分辨性。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const vodafoneSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#E60000',      // 每日创建 MR = Vodafone Red（头部指标独占唯一彩色）
      crDone: '#25282B',  // 每日完成 CR = ink 近黑墨
      crStart: '#7E7E7E', // 每日发起 CR = body 灰
      avgDuration: '#BEBEBE', // 每日成功运行平均耗时 = mute 灰
    },
    ratios: {
      success: '#0E8A3E',     // 当前任务成功占比 = 语义绿（派生）
      firstRun: '#E60000',    // 首次执行成功率 = 品牌红（hero 指标）
      intervention: '#9A6700', // 触发人工介入率 = 语义琥珀（派生）
      recovery: '#25282B',    // 受阻后恢复成功率 = ink（红绿琥珀之外的品牌另一极）
    },
  },
  timeline: {
    error: '#E60000',   // failed / blocked / unavailable = 红兼任 destructive（规格原文）
    warning: '#9A6700', // human_required / retry_pending / waiting_upstream
    success: '#0E8A3E', // succeeded / approved
    info: '#25282B',    // 其余默认 = 近黑墨（禁第二强调色）
  },
  eventType: {
    error: '#E60000',  // codex_error / claude_error
    usage: '#0E8A3E',  // codex_usage
    info: '#25282B',   // 其余默认
  },
  sparkline: {
    default: '#E60000',  // Sparkline 默认描线 = 品牌红
    hoverGrid: '#BEBEBE', // hover 竖向网格虚线 = mute 灰
  },
}
