import type { ThemeSlots } from '../types'

/**
 * raycast 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格把四个饱和点缀定义为官方语义位：
 *   绿 #59d499（success state）/ 黄 #ffc533（「warning」semantic）/
 *   红 #ff6161（destructive/error indicator）/ 蓝 #57c1ff（info）
 * 管理台的图表/时间线属于数据墨水与状态信号，是这四色的合法消费位
 * （规格禁令针对的是「chrome 按钮与文字」，不含状态语义本身）。
 * 白 #ffffff 是唯一主交互色：趋势线头牌（每日创建 MR）与 Sparkline
 * 默认描线都落白，呼应「white CTA 锚定每个主操作」的单色纪律。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const raycastSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#ffffff',         // 每日创建 MR = 白（头牌指标独占主交互色）
      crDone: '#59d499',     // 每日完成 CR = accent-green（success 语义位）
      crStart: '#57c1ff',    // 每日发起 CR = accent-blue（info 语义位）
      avgDuration: '#ffc533', // 每日成功运行平均耗时 = accent-yellow（时长警示语义位）
    },
    ratios: {
      success: '#59d499',      // 当前任务成功占比 = accent-green
      firstRun: '#57c1ff',     // 首次执行成功率 = accent-blue
      intervention: '#ff6161', // 触发人工介入率 = accent-red
      recovery: '#ffc533',     // 受阻后恢复成功率 = accent-yellow
    },
  },
  timeline: {
    error: '#ff6161',   // failed / blocked / unavailable = accent-red
    warning: '#ffc533', // human_required / retry_pending / waiting_upstream = accent-yellow
    success: '#59d499', // succeeded / approved = accent-green
    info: '#57c1ff',    // 其余默认 = accent-blue（info 语义位）
  },
  eventType: {
    error: '#ff6161',  // codex_error / claude_error = accent-red
    usage: '#59d499',  // codex_usage = accent-green（用量=正向信号，避免与 error 同相）
    info: '#57c1ff',   // 其余默认 = accent-blue
  },
  sparkline: {
    default: '#ffffff',                // Sparkline 默认描线 = 白（主交互色）
    hoverGrid: 'rgba(255, 255, 255, 0.16)', // hover 竖向网格虚线 = hairline-strong 白 16%
  },
}
