import type { ThemeSlots } from '../types'

/**
 * nike 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：chrome 近单色（ink/canvas/soft-cloud 三色扛 95% 表面），
 * 色彩只留给信号 —— sale 红 #D30005（唯一红）、success 绿 #007D48、
 * info 蓝 #1151FF（member-experience callout），外加克制的品类 accent
 * （teal #0A7281 等，规格 明令不得进 chrome，但作为图表墨水是信号）。
 * 派生值（规格未给、按设计语言推导）：
 *   warning 琥珀 #9A6700 —— 规格语义段无橙/琥珀（规格 全文无 warning 色），
 *   按其「色只给信号、低饱和文字级对比」的法则从标准语义克制化（先例：tesla/slots.ts）。
 * 趋势线走「墨 + 三信号色」：mr 用 ink（数据墨水即品牌黑），
 * done/start/duration 对应 success/info/sale 的语义映射。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const nikeSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#111111',      // 每日创建 MR = Nike Black（数据墨水即品牌色）
      crDone: '#007d48',  // 每日完成 CR = success 绿（完成信号）
      crStart: '#1151ff', // 每日发起 CR = info 蓝（发起/信息信号）
      avgDuration: '#d30005', // 每日成功运行平均耗时 = sale 红（成本/代价信号，价格色语言）
    },
    ratios: {
      success: '#007d48',      // 当前任务成功占比 = success 绿
      firstRun: '#1151ff',     // 首次执行成功率 = info 蓝
      intervention: '#d30005', // 触发人工介入率 = sale 红（告警信号）
      recovery: '#0a7281',     // 受阻后恢复成功率 = accent teal（品类 accent 作图表墨水，与绿蓝红拉开色相）
    },
  },
  timeline: {
    error: '#d30005',   // failed / blocked / unavailable = sale 红
    warning: '#9a6700', // human_required / retry_pending / waiting_upstream = 派生克制琥珀
    success: '#007d48', // succeeded / approved = success 绿
    info: '#1151ff',    // 其余默认 = info 蓝（信息节点信号色）
  },
  eventType: {
    error: '#d30005',  // codex_error / claude_error
    usage: '#007d48',  // codex_usage
    info: '#1151ff',   // 其余默认
  },
  sparkline: {
    default: '#111111',  // Sparkline 默认描线 = Nike Black（单色数据墨水）
    hoverGrid: '#cacacb', // hover 竖向网格虚线 = hairline 发丝线
  },
}
