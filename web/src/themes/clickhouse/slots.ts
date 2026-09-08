import type { ThemeSlots } from '../types'

/**
 * clickhouse 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格色板的暗底可用子集：品牌电压黄 #faff69 + 强调三色
 * （accent-emerald #22c55e / accent-rose #ef4444 / accent-blue #3b82f6，
 * 亦是规格 SQL 语法高亮的蓝黄灰一族）+ 语义 warning #f59e0b。
 * 近纯黑画布 (#0a0a0a/#1a1a1a) 上的可分辨性已逐色核对：四色全为中高明度，
 * 在 #1a1a1a 面板上有 ≥3:1 的描线对比。
 * 黄是趋势头牌（MR 曲线 / Sparkline 默认描线 / 用量轨）——呼应规格
 * 「stat numbers ALWAYS yellow」的数字级稀缺用法；其余曲线走强调三色，
 * 不引入第二品牌色。键结构与 default/slots.ts 保持一致，只改值。
 */
export const clickhouseSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#faff69',        // 每日创建 MR：电光黄 = 头牌曲线（品牌电压的数字级用法）
      crDone: '#3b82f6',    // 每日完成 CR：accent blue（语法高亮蓝）
      crStart: '#22c55e',   // 每日发起 CR：accent emerald
      avgDuration: '#ef4444', // 成功运行平均耗时：accent rose（耗时即警示）
    },
    ratios: {
      success: '#22c55e',     // 当前任务成功占比：语义绿
      firstRun: '#3b82f6',    // 首次执行成功率：accent blue
      intervention: '#ef4444', // 触发人工介入率：accent rose
      recovery: '#f59e0b',    // 受阻后恢复成功率：语义琥珀（恢复带警示余韵）
    },
  },
  timeline: {
    error: '#ef4444',   // failed / blocked / unavailable
    warning: '#f59e0b', // human_required / retry_pending / waiting_upstream
    success: '#22c55e', // succeeded / approved
    info: '#3b82f6',    // 其余默认 = accent blue（黄不进时间线节点，保持稀缺）
  },
  eventType: {
    error: '#ef4444',  // codex_error / claude_error
    usage: '#22c55e',  // codex_usage
    info: '#3b82f6',   // 其余默认
  },
  sparkline: {
    default: '#faff69',  // Sparkline 默认描线：电光黄（近纯黑画布上的单一黄色事件）
    hoverGrid: '#3a3a3a', // hover 竖向网格虚线：hairline-strong（暗底可见）
  },
}
