import type { ThemeSlots } from '../types'

/**
 * mastercard 主题的语义色槽位 —— 自设计语言 正文角色推导。
 *
 * 规格的可彩位极克制：Signal Orange #cf4500 是「the page's single aggressive
 * color and must be used sparingly」，Light Signal Orange #f37338 只做
 * 「attention cue, never body color」，Clay Brown #9a3a0a 是链接式次按钮的
 * 深锈色，Link Blue #3860be 是内链/信息色。品牌红黄「never as a UI color」。
 * 因此：
 *   - warning = Signal Orange（克制的功能位：需人工/等待）；
 *   - info / 默认节点 = Link Blue（信息色字面）；
 *   - 趋势线走 Ink / Blue / Orange / Clay 四色（暖石画布上的轨道谱系）；
 *   - success 规格无绿（支付品牌回避绿）→ 推导低饱和暖绿 #3a7d50；
 *   - error 禁用品牌红 #EB001B（「never as a UI color」）→ 推导暖红 #c03a2a。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const mastercardSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#141413',     // 每日创建 MR = Ink（头部指标用主墨）
      crDone: '#3860be', // 每日完成 CR = Link Blue
      crStart: '#cf4500', // 每日发起 CR = Signal Orange（单一进攻色的功能位）
      avgDuration: '#9a3a0a', // 每日成功运行平均耗时 = Clay Brown 深锈
    },
    ratios: {
      success: '#3a7d50',     // 当前任务成功占比 = 推导暖绿
      firstRun: '#3860be',    // 首次执行成功率 = Link Blue
      intervention: '#cf4500', // 触发人工介入率 = Signal Orange（需注意）
      recovery: '#9a3a0a',    // 受阻后恢复成功率 = Clay Brown
    },
  },
  timeline: {
    error: '#c03a2a',   // failed / blocked / unavailable（推导暖红，品牌红禁作 UI 色）
    warning: '#cf4500', // human_required / retry_pending / waiting_upstream = Signal Orange
    success: '#3a7d50', // succeeded / approved
    info: '#3860be',    // 其余默认 = Link Blue
  },
  eventType: {
    error: '#c03a2a', // codex_error / claude_error
    usage: '#3a7d50', // codex_usage
    info: '#3860be',  // 其余默认
  },
  sparkline: {
    default: '#141413', // Sparkline 默认描线 = Ink 数据墨水
    hoverGrid: '#d1cdc7', // hover 竖向网格虚线 = Dust Taupe 暖灰（whisper 线）
  },
}
