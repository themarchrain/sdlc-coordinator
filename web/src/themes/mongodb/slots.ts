import type { ThemeSlots } from '../types'

/**
 * mongodb 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格只给了 warning 一对（#946F3F / #FFF8E0）；
 * success/error/info 未给，按设计语言推导：
 *   - success 取 brand-green-dark #00684A（文字级绿；规格 Don'ts「亮绿
 *     不做正文/描线」——亮绿只留给主 CTA）；
 *   - error 派生文字级红 #D94141（规格无红；类目四色 purple/orange/pink/blue
 *     按 规格 约束只作课程标签，不挪用）；
 *   - info 借类目 accent-blue #3D4F9F 作冷色默认节点（类目色中唯一的
 *     中性冷色，与 deep-teal 同谱系）。
 * 图表描线走「绿族 + 墨阶」：green-mid #00A35C（比亮绿 #00ED64 稳）、
 * ink #001E2B、brand-green-dark #00684A、stone #7C8C9A；
 * 亮绿 #00ED64 对比度不足以作白底 2px 描线，故全部压深一档（偏离注明于行内）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const mongodbSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#00A35C',      // 每日创建 MR = green-mid（亮绿压深一档，保证白底可辨）
      crDone: '#001E2B',  // 每日完成 CR = ink 深青墨（display 强调色）
      crStart: '#00684A', // 每日发起 CR = brand-green-dark
      avgDuration: '#7C8C9A', // 每日成功运行平均耗时 = stone 中性阶
    },
    ratios: {
      success: '#00A35C',  // 当前任务成功占比 = green-mid
      firstRun: '#00684A', // 首次执行成功率 = brand-green-dark
      intervention: '#D94141', // 触发人工介入率 = 派生 error 红
      recovery: '#946F3F', // 受阻后恢复成功率 = 规格 warning-text
    },
  },
  timeline: {
    error: '#D94141',   // failed / blocked / unavailable（派生红）
    warning: '#946F3F', // human_required / retry_pending / waiting_upstream（规格）
    success: '#00A35C', // succeeded / approved（green-mid 描线稳于亮绿）
    info: '#3D4F9F',    // 其余默认 = accent-blue（类目冷色）
  },
  eventType: {
    error: '#D94141',  // codex_error / claude_error
    usage: '#00A35C',  // codex_usage（绿 = 用量数据墨水）
    info: '#3D4F9F',   // 其余默认
  },
  sparkline: {
    default: '#00A35C',  // Sparkline 默认描线 = green-mid（亮绿压深一档）
    hoverGrid: '#C1CCD6', // hover 竖向网格虚线 = hairline-strong
  },
}
