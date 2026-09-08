import type { ThemeSlots } from '../types'

/**
 * airtable 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：Airtable 营销面只有 link/info/success 三个语义色码——
 *   链接蓝 #1B61C9（info 族同源 #254FAD）、success #006400；
 *   warning/error 未给 → 按设计语言推导：
 *   - error 取签名 coral #AA2D00（规格「dark coral / oxide red」，系统里唯一的红，
 *     作为语义错误的功能性使用，非小件装饰）；
 *   - warning 取签名 mustard #D9A441 压深到文字级 #8F6412（白画布上可辨）。
 * 趋势线走「链接蓝 + 墨阶」四级（ink #181D26 / muted #41454D / border-strong #9297A0），
 * 呼应「monochrome editorial + 唯一蓝」的版面纪律；签名卡色（coral/forest/peach 等）
 * 按 规格 约束只作整面表面，不作小件描线色，故不进图表。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const airtableSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#1B61C9',      // 每日创建 MR = 链接蓝（头部指标独占唯一彩色）
      crDone: '#181D26',  // 每日完成 CR = ink（规格：display 强调靠墨不靠彩）
      crStart: '#41454D', // 每日发起 CR = muted
      avgDuration: '#9297A0', // 每日成功运行平均耗时 = border-strong 灰阶
    },
    ratios: {
      success: '#006400',     // 当前任务成功占比 = 规格 success
      firstRun: '#1B61C9',    // 首次执行成功率 = 链接蓝
      intervention: '#AA2D00', // 触发人工介入率 = 签名 coral（氧化红，error 同源）
      recovery: '#8F6412',    // 受阻后恢复成功率 = mustard 文字级（warning 同源）
    },
  },
  timeline: {
    error: '#AA2D00',   // failed / blocked / unavailable = 签名 coral
    warning: '#8F6412', // human_required / retry_pending / waiting_upstream = mustard 文字级
    success: '#006400', // succeeded / approved = 规格 success
    info: '#1B61C9',    // 其余默认 = 链接蓝
  },
  eventType: {
    error: '#AA2D00',  // codex_error / claude_error
    usage: '#006400',  // codex_usage
    info: '#1B61C9',   // 其余默认
  },
  sparkline: {
    default: '#1B61C9',  // Sparkline 默认描线 = 链接蓝
    hoverGrid: '#DDDDDD', // hover 竖向网格虚线 = hairline 发丝线
  },
}
