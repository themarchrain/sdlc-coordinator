import type { ThemeSlots } from '../types'

/**
 * bugatti 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 规格色板为全单色 + 唯一冰蓝链接 #c3d9f3（「essentially the entire
 * chromatic vocabulary outside black-and-white」），语义只有两枚：
 * warning 金 #d4a017 / success 绿 #5fa657（Semantic 原文）。
 * 取舍：
 *  - 趋势线走「白 + 冰蓝 + 灰阶」四线（mr=白=头部指标独占品牌主色，
 *    crDone=冰蓝，crStart/avgDuration=规格文字阶 #cccccc/#999999 反演），
 *    呼应「total monochrome + photography」纪律，纯黑卡面上全部 ≥3:1；
 *  - error 规格缺位 → #c0584f（推导：与 success #5fa657 同明度的去饱和
 *    红砖，照 tesla「语义缺位→标准语义克制化」先例校准到规格的 muted
 *    色温，#0d0d0d 面上 ≈4.4:1）；
 *  - info/默认节点 = 白（白即 primary；冰蓝只留链接，不装饰节点）。
 * 时间线/事件一律字面色值（非 preset），与 CSS 语义令牌严格同源。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const bugattiSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#ffffff',         // 每日创建 MR = 白（品牌主色独占头部指标）
      crDone: '#c3d9f3',     // 每日完成 CR = 冰蓝（唯一非单色）
      crStart: '#cccccc',    // 每日发起 CR = body 灰白（规格文字阶反演）
      avgDuration: '#999999', // 每日成功运行平均耗时 = muted 灰（四线最沉一档）
    },
    ratios: {
      success: '#5fa657',      // 当前任务成功占比 = 规格 success 绿
      firstRun: '#c3d9f3',     // 首次执行成功率 = 冰蓝
      intervention: '#c0584f', // 触发人工介入率 = 推导红砖（规格 error 缺位）
      recovery: '#d4a017',     // 受阻后恢复成功率 = 规格 warning 金
    },
  },
  timeline: {
    error: '#c0584f',   // failed / blocked / unavailable（推导红砖）
    warning: '#d4a017', // human_required / retry_pending / waiting_upstream（规格金）
    success: '#5fa657', // succeeded / approved（规格绿）
    info: '#ffffff',    // 其余默认 = 白（白即 primary）
  },
  eventType: {
    error: '#c0584f',  // codex_error / claude_error
    usage: '#5fa657',  // codex_usage（用量 = 正向信号走规格绿）
    info: '#ffffff',   // 其余默认
  },
  sparkline: {
    default: '#ffffff',   // Sparkline 默认描线 = 白（黑画布上的单一白色事件）
    hoverGrid: '#3a3a3a', // hover 竖向网格虚线 = hairline-strong（规格强分隔线）
  },
}
