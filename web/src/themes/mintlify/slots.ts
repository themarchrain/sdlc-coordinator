import type { ThemeSlots } from '../types'

/**
 * mintlify 主题的语义色槽位 —— 自设计语言 色板派生。
 *
 * /Don't：Mint 绿只做强调信号（accent CTA / 激活指示 / feature 对勾），
 * 且「不得引入 mint、tag-blue、error-red、testimonial-orange 之外的强调色」。
 * 因此图表族收敛为「薄荷 + 墨阶 + 引用蓝」：
 *   深薄荷 #00B48A（brand-green-deep，头部指标独占）
 *   墨 #0A0A0A / Steel #5A5A5C（规格 文字阶梯）
 *   引用蓝 #3772CF（brand-tag，文档引用色的图表位延伸）
 * 语义态直接取规格原生三色：annotate 绿 #1BA673 / warn 琥珀 #C37D0D /
 * error 红 #D45656。键结构与 default/slots.ts 保持一致，只改值。
 */
export const mintlifySlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#00B48A',      // 每日创建 MR = 深薄荷（hero「Get started」绿一族的独占位）
      crDone: '#0A0A0A',  // 每日完成 CR = Ink（黑药丸同源墨）
      crStart: '#3772CF', // 每日发起 CR = brand-tag 引用蓝
      avgDuration: '#5A5A5C', // 每日成功运行平均耗时 = Steel
    },
    ratios: {
      success: '#1BA673',  // 当前任务成功占比 = brand-annotate 绿（对勾绿族）
      firstRun: '#3772CF', // 首次执行成功率 = 引用蓝（与 success 绿拉开色相）
      intervention: '#D45656', // 触发人工介入率 = brand-error 红
      recovery: '#C37D0D', // 受阻后恢复成功率 = brand-warn 琥珀
    },
  },
  timeline: {
    error: '#D45656',   // failed / blocked / unavailable = brand-error
    warning: '#C37D0D', // human_required / retry_pending / waiting_upstream = brand-warn
    success: '#1BA673', // succeeded / approved = brand-annotate 绿
    info: '#3772CF',    // 其余默认 = 引用蓝（避免与 success 绿混淆，推导位）
  },
  eventType: {
    error: '#D45656',  // codex_error / claude_error = brand-error
    usage: '#1BA673',  // codex_usage = 对勾绿（feature list 绿对勾同族）
    info: '#3772CF',   // 其余默认 = 引用蓝
  },
  sparkline: {
    default: '#00B48A',  // Sparkline 默认描线 = 深薄荷（激活指示绿）
    hoverGrid: '#E5E5E5', // hover 竖向网格虚线 = Hairline 主分隔线
  },
}
