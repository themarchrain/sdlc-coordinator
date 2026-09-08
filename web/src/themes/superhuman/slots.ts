import type { ThemeSlots } from '../types'

/**
 * superhuman 主题的语义色槽位 —— 自设计语言 派生。
 *
 * 规格色卡只有靛蓝 / 紫 / 深青 / 暖灰四族，没有语义色段；全部按
 * 「黑白 stark 正文 + 靛蓝交互 + 深青信息」推导（偏离均行内注明）：
 *   - 语义三色 = 深青提亮 #0F7660（success，品牌 teal 家族）/
 *     暖赭 #A16207（warning）/ 暖红 #B23E3E（error），均校准到暖墨白画布；
 *   - info 一律回落品牌深青 #155555（surface-teal-mid），
 *     紫 violet-soft 对比度不足（白底 ~1.9:1）且规格明言紫只落 hero 装饰位；
 *   - 趋势线走「靛蓝 + 暖墨阶」四级（#1B1938 + #292827/#73706D/#9A9794），
 *     呼应正文黑白 stark、彩色不进数据面的纪律。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const superhumanSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#1b1938',         // 每日创建 MR = 靛蓝近黑（头部指标独占交互色）
      crDone: '#292827',     // 每日完成 CR = 暖墨 ink
      crStart: '#73706d',    // 每日发起 CR = ink-mute
      avgDuration: '#9a9794', // 每日成功运行平均耗时 = ink-faint
    },
    ratios: {
      success: '#0f7660',      // 当前任务成功占比 = 深青提亮（teal 家族成功绿）
      firstRun: '#1b1938',     // 首次执行成功率 = 靛蓝
      intervention: '#b23e3e', // 触发人工介入率 = 暖红
      recovery: '#a16207',     // 受阻后恢复成功率 = 暖赭
    },
  },
  timeline: {
    error: '#b23e3e',   // failed / blocked / unavailable
    warning: '#a16207', // human_required / retry_pending / waiting_upstream
    success: '#0f7660', // succeeded / approved
    info: '#155555',    // 其余默认 = 品牌深青（三画布第三 voice）
  },
  eventType: {
    error: '#b23e3e', // codex_error / claude_error
    usage: '#0f7660', // codex_usage
    info: '#155555',  // 其余默认 = 品牌深青
  },
  sparkline: {
    default: '#1b1938',  // Sparkline 默认描线 = 靛蓝近黑
    hoverGrid: '#e8e4dd', // hover 竖向网格虚线 = 暖灰 hairline
  },
}
