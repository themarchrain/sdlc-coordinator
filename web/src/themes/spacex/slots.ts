import type { ThemeSlots } from '../types'

/**
 * spacex 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 规格是黑白体系（黑 #000000 画布 + 白 #ffffff 墨，无 accent 色码）：
 *  - 趋势线走「白 + 冷灰」四级单色阶梯（#ffffff / #c8c8cf / #9a9aa2 / #6e6e76，
 *    后三级为规格两级白之间的派生冷灰），呼应 austere 黑白纪律；
 *  - 语义三色为派生（规格无语义色码，管理台成败/告警必须可辨）：
 *    绿 #3fb950 / 琥珀 #d29922 / 红 #f2555a —— 按 #000000 画布上 ≥4.5:1
 *    可读性选的克制暗底变体；
 *  - info/默认节点一律回落白（规格 Link on Dark = 白，白即唯一信号）。
 * 全部取值已在 #000000/#0a0a0a 面板上核对描线对比。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const spacexSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#ffffff',        // 每日创建 MR：纯白（头部指标独占最亮阶）
      crDone: '#c8c8cf',    // 每日完成 CR：冷灰一阶（派生）
      crStart: '#9a9aa2',   // 每日发起 CR：冷灰二阶（派生）
      avgDuration: '#6e6e76', // 每日成功运行平均耗时：冷灰三阶（派生）
    },
    ratios: {
      success: '#3fb950',     // 当前任务成功占比：派生语义绿
      firstRun: '#ffffff',    // 首次执行成功率：白（唯一信号）
      intervention: '#f2555a', // 触发人工介入率：派生语义红
      recovery: '#d29922',    // 受阻后恢复成功率：派生语义琥珀
    },
  },
  timeline: {
    error: '#f2555a',   // failed / blocked / unavailable
    warning: '#d29922', // human_required / retry_pending / waiting_upstream
    success: '#3fb950', // succeeded / approved
    info: '#ffffff',    // 其余默认 = 白（黑白体系的 info 信号）
  },
  eventType: {
    error: '#f2555a',  // codex_error / claude_error
    usage: '#3fb950',  // codex_usage
    info: '#ffffff',   // 其余默认
  },
  sparkline: {
    default: '#ffffff', // Sparkline 默认描线：白（黑画布上的单一白色事件）
    hoverGrid: '#3a3a3f', // hover 竖向网格虚线：hairline-on-dark（规格发丝线）
  },
}
