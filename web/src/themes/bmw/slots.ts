import type { ThemeSlots } from '../types'

/**
 * bmw 主题的语义色槽位 —— 自设计语言 色彩组派生（亮轨白画布）。
 *
 * 取值纪律（Don't）：BMW corporate 只有一个品牌操作色蓝，
 * 「Don't add a brand color other than blue」且 M 三色条纹
 * 「never in the corporate site's main language」→ 与 bmw-m 分工：
 * 本主题数据墨水 = 蓝 #1C69D4 + 中性灰阶（ink/muted/muted-soft），
 * 不引入 M 红与条纹蓝之外的任何色；语义四色用规格 Semantic 组原值：
 *   success #22C55E / warning #F59E0B / error #DC2626。
 * 时间线/事件节点直接写 hex（preset 名会渲染成 antd 标准蓝绿，与品牌不符），
 * 节点为 10px 色点，白底上以色相区分优先。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const bmwSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#1C69D4',      // 每日创建 MR = BMW blue（头部指标独占唯一彩色）
      crDone: '#262626',  // 每日完成 CR = ink（完成态落在墨上）
      crStart: '#6B6B6B', // 每日发起 CR = muted
      avgDuration: '#9A9A9A', // 每日成功运行平均耗时 = muted-soft（灰阶收尾）
    },
    ratios: {
      success: '#22C55E',     // 当前任务成功占比 = 规格 success
      firstRun: '#1C69D4',    // 首次执行成功率 = BMW blue
      intervention: '#DC2626', // 触发人工介入率 = 规格 error 红（介入即警示）
      recovery: '#F59E0B',    // 受阻后恢复成功率 = 规格 warning 琥珀
    },
  },
  timeline: {
    error: '#DC2626',   // failed / blocked / unavailable = 规格 error
    warning: '#F59E0B', // human_required / retry_pending / waiting_upstream = 规格 warning
    success: '#22C55E', // succeeded / approved = 规格 success
    info: '#1C69D4',    // 其余默认 = BMW blue（唯一交互色兼默认节点）
  },
  eventType: {
    error: '#DC2626',  // codex_error / claude_error
    usage: '#22C55E',  // codex_usage
    info: '#1C69D4',   // 其余默认
  },
  sparkline: {
    default: '#1C69D4',  // Sparkline 默认描线 = BMW blue（数据墨水）
    hoverGrid: '#CCCCCC', // hover 竖向网格虚线 = hairline-strong 控件边
  },
}
