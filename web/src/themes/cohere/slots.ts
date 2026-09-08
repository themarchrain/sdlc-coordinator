import type { ThemeSlots } from '../types'

/**
 * cohere 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：Cohere 的 UI 本体近乎黑白（near-black CTA + 白画布），
 * 彩色只以「数据墨水」身份出现：action-blue #1863DC（编辑链接/分页）、
 * coral #FF7759（分类小记号）、deep-green #003C33（暗色产品带）。
 * 规格只给了 error #B30000；success/warning 按品牌旁支色派生：
 *   success = pale-green #EDFCE9 黄绿家族压到文字级 #4D7C0F
 *   warning = coral 家族压深到警示文字级 #C2410C
 * info/默认节点一律回落 action-blue（次级强调的合法色）。
 * 趋势线走「蓝 + 深绿 + 珊瑚 + 石板灰」，白画布上均可分辨。
 * 键结构与 tesla/slots.ts 保持一致，只改值。
 */
export const cohereSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#1863DC',      // 每日创建 MR = action-blue（数据墨水头部）
      crDone: '#003C33',  // 每日完成 CR = deep-green（暗色产品带同源）
      crStart: '#FF7759', // 每日发起 CR = coral（编辑部暖记号）
      avgDuration: '#75758A', // 每日成功运行平均耗时 = slate（中性第三阶）
    },
    ratios: {
      success: '#4D7C0F',      // 当前任务成功占比 = 派生黄绿（pale-green 文字级）
      firstRun: '#1863DC',     // 首次执行成功率 = action-blue
      intervention: '#B30000', // 触发人工介入率 = 规格 error 原文
      recovery: '#C2410C',     // 受阻后恢复成功率 = 派生深珊瑚
    },
  },
  timeline: {
    error: '#B30000',   // failed / blocked / unavailable（规格 error）
    warning: '#C2410C', // human_required / retry_pending / waiting_upstream（派生深珊瑚）
    success: '#4D7C0F', // succeeded / approved（派生黄绿）
    info: '#1863DC',    // 其余默认 = action-blue
  },
  eventType: {
    error: '#B30000',  // codex_error / claude_error
    usage: '#4D7C0F',  // codex_usage（用量 = 正向黄绿）
    info: '#1863DC',   // 其余默认
  },
  sparkline: {
    default: '#1863DC',  // Sparkline 默认描线 = action-blue 数据墨水
    hoverGrid: '#D9D9DD', // hover 竖向网格虚线 = Hairline（规格标准规则线）
  },
}
