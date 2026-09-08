import type { ThemeSlots } from '../types'

/**
 * mobbin 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格（「Semantic」）：系统不设专用 success/warning/error 色板，
 * 状态沟通留在单色阶内、Electric Blue 是唯一正向强调信号（原文
 * 「accent as the sole positive-emphasis signal」）。迁移取法（先例 vercel/notion）：
 *   正向/成功 = Electric Blue #0066FF（规格唯一彩色的合法承担）
 *   警示/失败 = 派生克制语义色（琥珀 #A16207 / 红 #C8102E，保证时间线可辨）
 * 趋势线走「蓝 + 墨阶」四级（Ink/Muted/Faint），呼应画廊单色纪律；
 * 默认节点（info）回落墨阶而非蓝 —— 蓝只留给「需要决策/正向」的信号，
 * 避免默认节点与 success 节点同色（与 tesla/notion 的 info=蓝惯例的偏离，特此注记）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const mobbinSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#0066FF',      // 每日创建 MR = Electric Blue（头部指标独占唯一彩色）
      crDone: '#141414',  // 每日完成 CR = Ink
      crStart: '#707070', // 每日发起 CR = Muted
      avgDuration: '#ADADAD', // 每日成功运行平均耗时 = Faint
    },
    ratios: {
      success: '#0066FF',  // 当前任务成功占比 = accent（唯一正向信号）
      firstRun: '#141414', // 首次执行成功率 = Ink（中性指标走墨阶）
      intervention: '#C8102E', // 触发人工介入率 = 派生语义红
      recovery: '#A16207', // 受阻后恢复成功率 = 派生语义琥珀
    },
  },
  timeline: {
    error: '#C8102E',   // failed / blocked / unavailable
    warning: '#A16207', // human_required / retry_pending / waiting_upstream
    success: '#0066FF', // succeeded / approved —— accent 承担正向
    info: '#141414',    // 其余默认 = Ink（单色 chrome；偏离 info=蓝惯例）
  },
  eventType: {
    error: '#C8102E', // codex_error / claude_error
    usage: '#0066FF', // codex_usage —— 正向计量 = accent
    info: '#141414',  // 其余默认 = Ink
  },
  sparkline: {
    default: '#141414',  // Sparkline 默认描线 = 数据墨（先例 vercel 单色数据墨）
    hoverGrid: '#E0E0E0', // hover 竖向网格虚线 = Hairline 发丝线
  },
}
