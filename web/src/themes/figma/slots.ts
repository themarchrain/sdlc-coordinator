import type { ThemeSlots } from '../types'

/**
 * figma 主题的语义色槽位 —— 自设计语言 派生。
 *
 * 规格：Figma 营销系统是黑白 chrome + 粉彩色块；彩色家族只有
 *   block-lime #DCEEB1 / lilac #C5B0F4 / cream / mint / pink / coral #F3C9B6 /
 *   block-navy #1F1D3D + accent-magenta #FF3D8B（promo 单发）+
 *   semantic-success #1EA64A（比对表对勾）。
 * 数据取色原则（规格「Don't introduce mid-gray / monochrome core」）：
 *   趋势线以黑为首（数据墨水），辅以调色板内的 navy / magenta / success 绿
 *   ——四线四色互可分辨且不越出文档化色集；
 *   占比卡挂语义（成功绿 / 黑 / 派生红 / 派生琥珀）。
 * 派生说明：规格无 warning/error 语义色（Known Gaps 明示表单错误态未提取），
 *   取标准语义的克制饱和度：warning #B45309 / error #DC2626。
 * info / 默认节点一律回落 block-navy #1F1D3D（与 antd colorInfo 同源）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const figmaSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',       // 每日创建 MR = 纯黑数据墨水（monochrome core 首席）
      crDone: '#1F1D3D',   // 每日完成 CR = block-navy（调色板内深色）
      crStart: '#FF3D8B',  // 每日发起 CR = accent-magenta（图表位当单发强调）
      avgDuration: '#1EA64A', // 每日成功运行平均耗时 = semantic-success
    },
    ratios: {
      success: '#1EA64A',   // 当前任务成功占比 = 语义绿（比对表对勾同源）
      firstRun: '#000000',  // 首次执行成功率 = 黑（selected = primary surface）
      intervention: '#DC2626', // 触发人工介入率 = 派生语义红
      recovery: '#B45309',  // 受阻后恢复成功率 = 派生语义琥珀
    },
  },
  timeline: {
    error: '#DC2626',   // failed / blocked / unavailable（派生语义红）
    warning: '#B45309', // human_required / retry_pending / waiting_upstream（派生）
    success: '#1EA64A', // succeeded / approved
    info: '#1F1D3D',    // 其余默认 = block-navy（info 家族同源）
  },
  eventType: {
    error: '#DC2626',  // codex_error / claude_error
    usage: '#1EA64A',  // codex_usage
    info: '#1F1D3D',   // 其余默认
  },
  sparkline: {
    default: '#000000',  // Sparkline 默认描线 = 黑数据墨水
    hoverGrid: '#E6E6E6', // hover 竖向网格虚线 = hairline
  },
}
