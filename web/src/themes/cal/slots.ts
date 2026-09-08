import type { ThemeSlots } from '../types'

/**
 * cal 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：动作层单色（ink #111111），语义三色
 * 规格直书 —— success #10B981 / warning #F59E0B / error #EF4444；
 * brand-accent #3B82F6 只允许稀缺内联/徽章时刻，不作 CTA。
 * 趋势线走「墨阶 + 一处蓝」：黑/深灰/浅灰三阶保证白画布可分辨，
 * 第四条（平均耗时）落到唯一蓝 —— 呼应「蓝只在稀有时刻出现」。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const calSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#111111',      // 每日创建 MR = ink（黑即主电压）
      crDone: '#374151',  // 每日完成 CR = body 深灰
      crStart: '#898989', // 每日发起 CR = muted-soft 浅灰
      avgDuration: '#3B82F6', // 每日成功运行平均耗时 = 唯一蓝（稀缺内联时刻）
    },
    ratios: {
      success: '#10B981',   // 当前任务成功占比 = 语义绿
      firstRun: '#3B82F6',  // 首次执行成功率 = 品牌蓝（正向但与绿区分）
      intervention: '#EF4444', // 触发人工介入率 = 语义红
      recovery: '#F59E0B',  // 受阻后恢复成功率 = 语义琥珀
    },
  },
  timeline: {
    error: '#EF4444',   // failed / blocked / unavailable
    warning: '#F59E0B', // human_required / retry_pending / waiting_upstream
    success: '#10B981', // succeeded / approved
    info: '#3B82F6',    // 其余默认 = 品牌蓝（info 即蓝的内联用法）
  },
  eventType: {
    error: '#EF4444',  // codex_error / claude_error
    usage: '#10B981',  // codex_usage（正向计量 = 语义绿）
    info: '#3B82F6',   // 其余默认
  },
  sparkline: {
    default: '#111111',  // Sparkline 默认描线 = 墨（单色动作层延伸到数据墨水）
    hoverGrid: '#E5E7EB', // hover 竖向网格虚线 = hairline
  },
}
