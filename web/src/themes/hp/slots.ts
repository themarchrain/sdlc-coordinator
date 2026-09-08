import type { ThemeSlots } from '../types'

/**
 * hp 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格允许色板的白底可用子集：Electric Blue #024ad8（唯一信号）、
 * Deep Navy #0e3191（按压/visited）、Storm Deep #356373（中性状态 accent）、
 * Bloom Deep #b3262b（error/sale 色）+ Graphite #636363 中性系列
 * （规格 Don't：不得引入蓝族/珊瑚/风暴族之外的饱和色 → 趋势线走「蓝 + 墨阶」，
 * 呼应「蓝 + 墨做九成工作」的目录纪律）。
 * warning 琥珀 #8a5a00 为派生（规格无 warning 色码，白底 ≥5:1 克制变体）。
 * 全部取值已在 #ffffff 卡面上核对描线对比。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const hpSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#024ad8',      // 每日创建 MR：Electric Blue（头部指标独占唯一信号）
      crDone: '#1a1a1a',  // 每日完成 CR：Ink（墨做九成工作）
      crStart: '#636363', // 每日发起 CR：Graphite
      avgDuration: '#356373', // 每日成功运行平均耗时：Storm Deep
    },
    ratios: {
      success: '#356373',    // 当前任务成功占比：Storm Deep（规格状态 accent）
      firstRun: '#024ad8',   // 首次执行成功率：Electric Blue
      intervention: '#b3262b', // 触发人工介入率：Bloom Deep（error/sale 族）
      recovery: '#8a5a00',   // 受阻后恢复成功率：派生琥珀
    },
  },
  timeline: {
    error: '#b3262b',   // failed / blocked / unavailable
    warning: '#8a5a00', // human_required / retry_pending / waiting_upstream
    success: '#356373', // succeeded / approved
    info: '#024ad8',    // 其余默认 = 唯一信号蓝
  },
  eventType: {
    error: '#b3262b',  // codex_error / claude_error
    usage: '#356373',  // codex_usage
    info: '#024ad8',   // 其余默认
  },
  sparkline: {
    default: '#024ad8', // Sparkline 默认描线：Electric Blue
    hoverGrid: '#c2c2c2', // hover 竖向网格虚线：Steel（规格强边线）
  },
}
