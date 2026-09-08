import type { ThemeSlots } from '../types'

/**
 * starbucks 主题的语义色槽位 —— 自设计语言 正文语义段派生。
 *
 * 规格（Semantic & Accent）：Red #c82014 = error/destructive、
 * Yellow #fbbc05 = warning（legacy brand touch）、Green Light #d4e9e2 = valid 淡洗。
 * 迁移取舍：
 *   - 黄 #fbbc05 在奶油底上作节点/描线对比不足 → 深化 #97690f（同 hue 文字级）；
 *   - 成功一律走四绿系统（Accent #00754a / Starbucks #006241），不引入第五绿；
 *   - info/默认节点 = Starbucks Green（标题绿），与 CTA 绿分轨；
 *   - 趋势线四色 = Accent 绿 / House 深 / Uplift 中深 + 金 #cba258 —— 偏离点：
 *     规格金仅 Rewards 仪式（「Gold reserved for Rewards-status moments only」），
 *     但四档绿在描线尺度不可分辨，按「premium-feeling accents」角色将金引入为
 *     第四色相，是本主题唯一一处金落在非 Rewards 装饰位（图表数据墨）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const starbucksSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#00754a',      // 每日创建 MR = Green Accent（最亮一档绿，头部指标）
      crDone: '#1e3932',  // 每日完成 CR = House Green（最深 bookend 绿）
      crStart: '#2b5148', // 每日发起 CR = Green Uplift（中深装饰绿）
      avgDuration: '#cba258', // 每日成功运行平均耗时 = Gold（偏离见上：第四色相保可分辨）
    },
    ratios: {
      success: '#00754a',  // 当前任务成功占比 = CTA 绿
      firstRun: '#006241', // 首次执行成功率 = Starbucks Green
      intervention: '#c82014', // 触发人工介入率 = Red（规格 error 色）
      recovery: '#33433d', // 受阻后恢复成功率 = Rewards Green（微尘读色，与红/绿均可分辨）
    },
  },
  timeline: {
    error: '#c82014',   // failed / blocked / unavailable
    warning: '#97690f', // human_required / retry_pending / waiting_upstream（黄深化，推导）
    success: '#00754a', // succeeded / approved
    info: '#006241',    // 其余默认 = 标题绿
  },
  eventType: {
    error: '#c82014', // codex_error / claude_error
    usage: '#00754a', // codex_usage
    info: '#006241',  // 其余默认
  },
  sparkline: {
    default: '#00754a',  // Sparkline 默认描线 = Green Accent
    hoverGrid: '#d6dbde', // hover 竖向网格虚线 = Input Border #d6dbde（规格字面值）
  },
}
