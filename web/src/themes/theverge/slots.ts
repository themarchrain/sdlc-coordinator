import type { ThemeSlots } from '../types'

/**
 * theverge 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格只钉了两条语义规则：error/alert 强调 = Verge Ultraviolet #5200FF
 * （Inputs「ultraviolet — used as error/alert accent here, not the usual red」），
 * 正反馈无独立色 → 落在品牌 Jelly Mint #3CFFD0。其余槽位从「饱和色块瓦片调色板」
 * （mint/purple/yellow/pink/orange/blue/white）派生可读定值：
 *   yellow #FFE500（Verge 黄的酸饱和阶）/ pink #FF64B8 / orange #FF6A2B
 * 时间线/事件默认节点 = Hazard White #FFFFFF（白点是暗画布上的中性默认）。
 * 趋势四线 = mint / yellow / pink / ultraviolet，暗画布上亮度与色相双区分。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const thevergeSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#3CFFD0',        // 每日创建 MR = Jelly Mint（hero 指标独占签名色）
      crDone: '#FFE500',    // 每日完成 CR = Verge 黄瓦片（派生定值）
      crStart: '#FF64B8',   // 每日发起 CR = 粉瓦片（派生定值）
      avgDuration: '#5200FF', // 每日成功运行平均耗时 = Verge Ultraviolet（第二 hazard）
    },
    ratios: {
      success: '#3CFFD0',      // 当前任务成功占比 = 薄荷
      firstRun: '#FFE500',     // 首次执行成功率 = Verge 黄
      intervention: '#5200FF', // 触发人工介入率 = ultraviolet（警示 hazard）
      recovery: '#FF6A2B',     // 受阻后恢复成功率 = 橙瓦片（派生定值）
    },
  },
  timeline: {
    error: '#5200FF',   // failed / blocked / unavailable = error accent
    warning: '#FFE500', // human_required / retry_pending / waiting_upstream = Verge 黄
    success: '#3CFFD0', // succeeded / approved = 薄荷
    info: '#FFFFFF',    // 其余默认节点 = Hazard White
  },
  eventType: {
    error: '#5200FF',  // codex_error / claude_error
    usage: '#3CFFD0',  // codex_usage = 活动数据 = 薄荷
    info: '#FFFFFF',   // 其余默认
  },
  sparkline: {
    default: '#3CFFD0',  // Sparkline 默认描线 = 签名薄荷
    hoverGrid: '#949494', // hover 竖向网格虚线 = Secondary Text 元数据灰
  },
}
