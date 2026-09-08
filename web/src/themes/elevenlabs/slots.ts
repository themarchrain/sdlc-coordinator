import type { ThemeSlots } from '../types'

/**
 * elevenlabs 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：品牌电压是「摄影感的」，不是色度的 ——
 * 柔和渐变球（mint/peach/lavender/sky/rose）只做氛围装饰，禁止做
 * 按钮填充 / 文字色 / 组件底色；语义色只有 success #16a34a 与
 * error #dc2626 在规格内，warning 取暖琥珀 #b45309、info 取墨族
 * 中性 #44403c（均推导：编辑感纪律 = 不引入饱和彩相）。
 * 趋势线走「墨 + 暖灰」四级的暖单色阶梯（ink/body/muted/muted-soft），
 * 呼应「无霓虹强调色」的安静编辑声线，同时保证离白画布上的可分辨性。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const elevenlabsSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#0c0a09',     // 每日创建 MR = ink（头部指标独占最重墨）
      crDone: '#4e4e4e', // 每日完成 CR = body（暖灰次级）
      crStart: '#777169', // 每日发起 CR = muted（暖灰三级）
      avgDuration: '#a8a29e', // 每日平均耗时 = muted-soft（最轻一档）
    },
    ratios: {
      success: '#16a34a',  // 成功占比 = 规格确认绿
      firstRun: '#292524', // 首次执行成功率 = 墨（primary 同源，无彩 CTA 原则）
      intervention: '#dc2626', // 人工介入率 = 规格 error 红
      recovery: '#b45309', // 恢复成功率 = 暖琥珀（推导，与 warning 同源）
    },
  },
  timeline: {
    error: '#dc2626',   // failed / blocked / unavailable = 规格红
    warning: '#b45309', // human_required / retry_pending / waiting_upstream = 暖琥珀（推导）
    success: '#16a34a', // succeeded / approved = 规格绿
    info: '#44403c',    // 其余默认 = 墨族中性（推导：无饱和 info 彩相）
  },
  eventType: {
    error: '#dc2626',  // codex_error / claude_error
    usage: '#16a34a',  // codex_usage = 规格绿
    info: '#44403c',   // 其余默认
  },
  sparkline: {
    default: '#292524',  // Sparkline 默认描线 = 墨（ink 药丸同源）
    hoverGrid: '#d6d3d1', // hover 竖向网格虚线 = hairline-strong
  },
}
