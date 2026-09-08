import type { ThemeSlots } from '../types'

/**
 * playstation 主题的语义色槽位 —— 自设计语言 colors 表 + 语义段派生。
 *
 * 规格语义段只给了一个 warning #c81b3a（校验错误/破坏性确认，红相）→
 * 迁移为 error 语义；真琥珀 warning 与 success 绿均未 captured，派生为
 * 标准语义的克制化：绿 #1f7a3d / 琥珀 #97650d。
 * PS Blue #0070d1 = universal primary，info/默认节点一律回落此蓝。
 * 趋势线走「蓝 + 墨阶」四级（ink / mute-light / 墨 40% 合成），呼应
 * 「chrome is unusually quiet」的减法纪律；commerce 橙与 PS Plus 金、
 * marathon 黄按规格保留条款不进图表（橙 = 商店 CTA 专属，金 = PS Plus
 * banner 专属，黄 = marathon 页专属）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const playstationSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#0070d1',      // 每日创建 MR = PS Blue（头部指标独占品牌蓝）
      crDone: '#121314',  // 每日完成 CR = ink-deep 近黑
      crStart: '#6b6b6b', // 每日发起 CR = mute-light
      avgDuration: '#999999', // 每日成功运行平均耗时 = 墨 40% 合成（body-light alpha 阶梯下探一档，推导）
    },
    ratios: {
      success: '#1f7a3d',     // 当前任务成功占比 = 语义绿（派生，规格无 success）
      firstRun: '#0070d1',    // 首次执行成功率 = PS Blue
      intervention: '#c81b3a', // 触发人工介入率 = 规格 warning 字面值（校验/破坏性红相）
      recovery: '#97650d',    // 受阻后恢复成功率 = 语义琥珀（派生）
    },
  },
  timeline: {
    error: '#c81b3a',   // failed / blocked / unavailable（规格字面值）
    warning: '#97650d', // human_required / retry_pending / waiting_upstream（派生琥珀）
    success: '#1f7a3d', // succeeded / approved（派生绿）
    info: '#0070d1',    // 其余默认 = PS Blue
  },
  eventType: {
    error: '#c81b3a',  // codex_error / claude_error
    usage: '#1f7a3d',  // codex_usage
    info: '#0070d1',   // 其余默认
  },
  sparkline: {
    default: '#0070d1',  // Sparkline 默认描线 = PS Blue
    hoverGrid: '#cccccc', // hover 竖向网格虚线 = ash-light（控件边一档）
  },
}
