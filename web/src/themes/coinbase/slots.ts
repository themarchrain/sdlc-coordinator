import type { ThemeSlots } from '../types'

/**
 * coinbase 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：系统只有两个交易语义色——
 * semantic-up #05B169 与 semantic-down #CF202F，且「text color only，
 * never background fills」；蓝色 #0052FF 是唯一动作色。
 * warning 规格未给：由 accent-yellow #F4B000 深化为文字级琥珀 #9A6F00
 * （黄仅插画用途，此处只作文字描线，不违反「不作动作底色」）。
 * 趋势线走「蓝 + 墨阶」四级（ink/body/muted-soft），呼应该系统
 * 「近单色 + 唯一蓝」的纪律，同时保证白画布上的可分辨性。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const coinbaseSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#0052FF',      // 每日创建 MR = Coinbase Blue（头部指标独占品牌电压）
      crDone: '#0A0B0D',  // 每日完成 CR = ink
      crStart: '#5B616E', // 每日发起 CR = body 灰
      avgDuration: '#A8ACB3', // 每日成功运行平均耗时 = muted-soft
    },
    ratios: {
      success: '#05B169',      // 当前任务成功占比 = semantic-up
      firstRun: '#0052FF',     // 首次执行成功率 = Coinbase Blue
      intervention: '#CF202F', // 触发人工介入率 = semantic-down
      recovery: '#9A6F00',     // 受阻后恢复成功率 = 派生琥珀（规格无 warning）
    },
  },
  timeline: {
    error: '#CF202F',   // failed / blocked / unavailable
    warning: '#9A6F00', // human_required / retry_pending / waiting_upstream（派生琥珀）
    success: '#05B169', // succeeded / approved
    info: '#0052FF',    // 其余默认 = 唯一动作蓝
  },
  eventType: {
    error: '#CF202F',  // codex_error / claude_error
    usage: '#05B169',  // codex_usage（正向计量 = semantic-up）
    info: '#0052FF',   // 其余默认
  },
  sparkline: {
    default: '#0052FF',  // Sparkline 默认描线 = 唯一动作蓝
    hoverGrid: '#DEE1E6', // hover 竖向网格虚线 = hairline 发丝线
  },
}
