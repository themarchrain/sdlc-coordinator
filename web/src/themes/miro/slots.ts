import type { ThemeSlots } from '../types'

/**
 * miro 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格语义面很薄：success-accent #00B473 是唯一记名的语义绿；红系只给了
 * 薄纱底（brand-red #FBD4D4）与边框档（brand-red-dark #E3C5C5），文字/描线级
 * 按同色相推导为 #C92A2A；警示锚定黄标签前景 yellow-dark #746019（深橄榄，
 * 「Yellow-tag text color」），soft 底用 surface-yellow #FFF8E0。
 * info/默认节点 = 动作蓝 Brand Blue #4262FF（黑不做数据墨水，蓝是规格的
 * inline link / featured 强调色）。趋势线走「蓝 + 墨阶 + 品牌青」：
 * avgDuration 用品牌青 #0FBCB0（规格 brand-teal，非 pastel 档，白底可辨）。
 * success #00B473 在白底对比度 ~2.9:1，作 2px 描线/时间线节点可辨，
 * 但低于正文标准——沿用规格原值（badge-success 即此绿底白字），此处不擅自压深。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const miroSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#4262FF',      // 每日创建 MR = 动作蓝（头部指标的彩色数据墨水）
      crDone: '#1C1C1E',  // 每日完成 CR = Ink
      crStart: '#555A6A', // 每日发起 CR = Slate
      avgDuration: '#0FBCB0', // 每日成功运行平均耗时 = 品牌 Teal（规格 brand-teal）
    },
    ratios: {
      success: '#00B473',     // 当前任务成功占比 = 规格 success-accent
      firstRun: '#4262FF',    // 首次执行成功率 = 动作蓝
      intervention: '#C92A2A', // 触发人工介入率 = 派生红（brand-red 系文字级）
      recovery: '#746019',    // 受阻后恢复成功率 = yellow-dark（「曾受阻」的黄系语义）
    },
  },
  timeline: {
    error: '#C92A2A',   // failed / blocked / unavailable（派生红）
    warning: '#746019', // human_required / retry_pending / waiting_upstream
    success: '#00B473', // succeeded / approved（规格 success-accent）
    info: '#4262FF',    // 其余默认 = 动作蓝
  },
  eventType: {
    error: '#C92A2A',  // codex_error / claude_error
    usage: '#00B473',  // codex_usage（规格 success-accent）
    info: '#4262FF',   // 其余默认
  },
  sparkline: {
    default: '#4262FF',  // Sparkline 默认描线 = 动作蓝
    hoverGrid: '#C7CAD5', // hover 竖向网格虚线 = Hairline Strong
  },
}
