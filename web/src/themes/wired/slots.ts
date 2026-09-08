import type { ThemeSlots } from '../types'

/**
 * wired 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：WIRED 只运营黑白二重奏 + 唯一内文链接蓝 #057DBC
 * （「no chromatic accent」「link blue is reserved for inline body links only」）。
 * 规格无 error/success/warning 谱系（「validation cues use the ink + body grey
 * hierarchy」）——时间线/事件线需要可分辨语义，按系统先例派生为标准语义克制化：
 *   绿 #1E7B34 / 琥珀 #9A6700 / 红 #B3261E
 * info / 默认节点一律回落链接蓝 #057DBC（唯一彩色的功能化复用）。
 * 趋势线走「黑 + 蓝 + 灰阶」：ink 独占头部指标、蓝承载数据对比、灰阶铺底，
 * 呼应印刷杂志信息图的黑白主调。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const wiredSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',      // 每日创建 MR = ink（黑即品牌，头部指标独占墨色）
      crDone: '#057DBC',  // 每日完成 CR = 链接蓝（唯一彩色的数据化使用）
      crStart: '#757575', // 每日发起 CR = body 灰
      avgDuration: '#9E9E9E', // 每日成功运行平均耗时 = 派生浅灰（body 提亮）
    },
    ratios: {
      success: '#1E7B34',     // 当前任务成功占比 = 语义绿（派生）
      firstRun: '#057DBC',    // 首次执行成功率 = 链接蓝
      intervention: '#B3261E', // 触发人工介入率 = 语义红（派生）
      recovery: '#9A6700',    // 受阻后恢复成功率 = 语义琥珀（派生）
    },
  },
  timeline: {
    error: '#B3261E',   // failed / blocked / unavailable
    warning: '#9A6700', // human_required / retry_pending / waiting_upstream
    success: '#1E7B34', // succeeded / approved
    info: '#057DBC',    // 其余默认 = 链接蓝（唯一彩色承担 info）
  },
  eventType: {
    error: '#B3261E',  // codex_error / claude_error
    usage: '#1E7B34',  // codex_usage
    info: '#057DBC',   // 其余默认
  },
  sparkline: {
    default: '#000000',  // Sparkline 默认描线 = ink（印刷黑数据线）
    hoverGrid: '#E0E0E0', // hover 竖向网格虚线 = hairline
  },
}
