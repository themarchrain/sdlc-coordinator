import type { ThemeSlots } from '../types'

/**
 * uber 主题的语义色槽位 —— 自设计语言 色彩组在白画布上派生。
 *
 * 规格明言品牌无语义色盘、无第二强调色（规格 Semantic：「验证线索来自
 * 主黑与插画」；Don't：「Don't introduce a second brand accent colour」）→
 * 数据墨水 = 黑白灰阶 + 系统唯一彩 #0000EE（浏览器默认链接蓝，规格 link-blue）：
 *   趋势线走「黑 / hairline-mid / 蓝 / mute」四步（vercel 单色数据的灰阶先例
 *   + 一根蓝承担可分辨性）；
 *   语义三色按「浏览器默认语义的克制化」推导（与 #0000EE 的系统 chrome
 *   语声同理，先例：tesla/slots.ts）：绿 #1F7A3D / 琥珀 #A16207 / 红 #C8102E。
 * 时间线/事件节点直接写 hex（preset 名会渲染成 antd 标准蓝绿，与品牌不符），
 * 节点为 10px 色点，白底上以色相区分优先。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const uberSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',      // 每日创建 MR = ink 黑（头部指标独占纯黑）
      crDone: '#4B4B4B',  // 每日完成 CR = hairline-mid 深灰
      crStart: '#0000EE', // 每日发起 CR = link 蓝（系统唯一彩）
      avgDuration: '#8C8C8C', // 每日成功运行平均耗时 = 中灰（#5E5E5E 与 #AFAFAF 之间派生一档）
    },
    ratios: {
      success: '#1F7A3D',     // 当前任务成功占比 = 语义绿（推导）
      firstRun: '#0000EE',    // 首次执行成功率 = link 蓝
      intervention: '#C8102E', // 触发人工介入率 = 语义红（推导）
      recovery: '#A16207',    // 受阻后恢复成功率 = 语义琥珀（推导）
    },
  },
  timeline: {
    error: '#C8102E',   // failed / blocked / unavailable = 语义红（推导）
    warning: '#A16207', // human_required / retry_pending / waiting_upstream = 语义琥珀（推导）
    success: '#1F7A3D', // succeeded / approved = 语义绿（推导）
    info: '#0000EE',    // 其余默认 = link 蓝（唯一彩兼默认节点）
  },
  eventType: {
    error: '#C8102E',  // codex_error / claude_error
    usage: '#1F7A3D',  // codex_usage
    info: '#0000EE',   // 其余默认
  },
  sparkline: {
    default: '#000000',  // Sparkline 默认描线 = 纯黑数据墨（vercel #171717 的纯黑版）
    hoverGrid: '#AFAFAF', // hover 竖向网格虚线 = mute 灰
  },
}
