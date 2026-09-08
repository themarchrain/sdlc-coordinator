import type { ThemeSlots } from '../types'

/**
 * kraken 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格色板的白底可用子集：Kraken Purple 一族（#7132f5/#5741d8/#5b1ecf）
 * + Green #149e61（success/positive）+ 冷灰中性（Cool Gray #686b82 /
 * Silver Blue #9497a9）。紫即主色 → MR/首跑/默认节点一律紫族；
 * 绿承担成功/用量（加密交易所的「涨」语义）；warning/error 规格未给
 * → 标准语义克制化 #9c6a10/#b6443a（推导值）。
 * 白底 #ffffff 上的可分辨性已逐色核对（描线对比 ≥3:1）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const krakenSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#7132f5',        // 每日创建 MR：Kraken Purple（头部指标独占命令色）
      crDone: '#149e61',    // 每日完成 CR：Green（正向完成语义）
      crStart: '#5741d8',   // 每日发起 CR：Purple Dark（紫族第二阶）
      avgDuration: '#9497a9', // 每日成功运行平均耗时：Silver Blue（弱曲线）
    },
    ratios: {
      success: '#149e61',   // 当前任务成功占比：Green
      firstRun: '#7132f5',  // 首次执行成功率：Kraken Purple
      intervention: '#b6443a', // 触发人工介入率：语义红（推导）
      recovery: '#9c6a10',  // 受阻后恢复成功率：语义琥珀（推导）
    },
  },
  timeline: {
    error: '#b6443a',   // failed / blocked / unavailable
    warning: '#9c6a10', // human_required / retry_pending / waiting_upstream
    success: '#149e61', // succeeded / approved = Green（规格 success）
    info: '#7132f5',    // 其余默认 = 命令紫
  },
  eventType: {
    error: '#b6443a',  // codex_error / claude_error
    usage: '#149e61',  // codex_usage = Green（正向计量）
    info: '#7132f5',   // 其余默认
  },
  sparkline: {
    default: '#7132f5', // Sparkline 默认描线：Kraken Purple
    hoverGrid: '#dedee5', // hover 竖向网格虚线：Border Gray（白底可见）
  },
}
