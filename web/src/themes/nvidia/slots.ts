import type { ThemeSlots } from '../types'

/**
 * nvidia 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：全系统一个强调色 NVIDIA Green #76B900（CTA/激活/角标），
 * 语义色齐全：error #E52020 / warning #DF6500 / success-deep #3F8500
 * （专为「绿饱和撞色」场景而设）；link-blue #0046A4 是唯一蓝，只许做
 * 内联链接/数据墨水；accent-purple #952FC6 是长文编辑强调。
 * info/默认节点走 ink 黑：绿是「珍贵资源」（规格 Don't「one-color」纪律），
 * 蓝不外溢到 chrome，黑是黑白灰体系内的天然默认。
 * 趋势线走「绿 + 蓝 + 紫 + 灰」，白画布上均可分辨。
 * 键结构与 tesla/slots.ts 保持一致，只改值。
 */
export const nvidiaSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#76B900',      // 每日创建 MR = NVIDIA Green（头部数据独占品牌绿）
      crDone: '#0046A4',  // 每日完成 CR = link-blue（唯一蓝做数据墨水）
      crStart: '#952FC6', // 每日发起 CR = accent-purple（编辑强调色）
      avgDuration: '#757575', // 每日成功运行平均耗时 = Mute 中性灰
    },
    ratios: {
      success: '#3F8500',      // 当前任务成功占比 = success-deep（规格语义原文）
      firstRun: '#76B900',     // 首次执行成功率 = 品牌绿（正向主力指标）
      intervention: '#E52020', // 触发人工介入率 = error
      recovery: '#DF6500',     // 受阻后恢复成功率 = warning
    },
  },
  timeline: {
    error: '#E52020',   // failed / blocked / unavailable（规格 error）
    warning: '#DF6500', // human_required / retry_pending / waiting_upstream
    success: '#3F8500', // succeeded / approved（success-deep）
    info: '#000000',    // 其余默认 = ink 黑（绿留给 CTA，蓝不进 chrome）
  },
  eventType: {
    error: '#E52020',  // codex_error / claude_error
    usage: '#76B900',  // codex_usage（用量轨 = 品牌绿，与 --bar-usage 同源）
    info: '#000000',   // 其余默认
  },
  sparkline: {
    default: '#76B900',  // Sparkline 默认描线 = 品牌绿（callout-stat 绿数字同源）
    hoverGrid: '#CCCCCC', // hover 竖向网格虚线 = Hairline
  },
}
