import type { ThemeSlots } from '../types'

/**
 * slack 主题的语义色槽位 —— 规格驱动，无参考实现。
 *
 * 规格的取色纪律：茄紫 #4A154B 是唯一品牌色但
 * 「不用作正文字色」→ 图表主色仍取茄紫（描线非正文，规格 统计卡里
 * 50px 茄紫巨型数值即同源用法）；四线可分辨性由规格自带的彩色家族承担：
 * link-blue #1264A3（唯一合法的彩色点缀）/ success #007A5A / error
 * #CC4117（均字面值）；规格无 warning → 推导暖琥珀 #975A16（与 error
 * 同族暖度，白底描线对比 ≥3:1）。「不加第三强调色」——灰阶只作弱曲线。
 * 白/奶油/薰衣草面上的可分辨性已逐色核对。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const slackSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#4A154B',        // 每日创建 MR = aubergine（统计卡巨型数值同源）
      crDone: '#007A5A',    // 每日完成 CR = semantic-success 字面值
      crStart: '#1264A3',   // 每日发起 CR = link-blue（唯一彩色点缀）
      avgDuration: '#696969', // 每日成功运行平均耗时 = ink-mute（弱曲线）
    },
    ratios: {
      success: '#007A5A',   // 当前任务成功占比 = semantic-success
      firstRun: '#4A154B',  // 首次执行成功率 = aubergine
      intervention: '#CC4117', // 触发人工介入率 = semantic-error
      recovery: '#975A16',  // 受阻后恢复成功率 = 暖琥珀（推导，规格无 warning）
    },
  },
  timeline: {
    error: '#CC4117',   // failed / blocked / unavailable
    warning: '#975A16', // human_required / retry_pending / waiting_upstream（推导）
    success: '#007A5A', // succeeded / approved
    info: '#1264A3',    // 其余默认 = link-blue
  },
  eventType: {
    error: '#CC4117',  // codex_error / claude_error
    usage: '#007A5A',  // codex_usage
    info: '#1264A3',   // 其余默认
  },
  sparkline: {
    default: '#4A154B',  // Sparkline 默认描线 = aubergine
    hoverGrid: '#D4CFC7', // hover 竖向网格虚线 = cream 族暖灰（推导，足见度）
  },
}
