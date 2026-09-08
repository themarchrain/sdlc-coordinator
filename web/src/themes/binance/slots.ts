import type { ThemeSlots } from '../types'

/**
 * binance 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格色板的暗底可用子集：Binance Yellow #fcd535（主 CTA/品牌电压）+
 * 涨跌对 trading-up #0ecb81 / trading-down #f6465d + info #3b82f6。
 * 取舍（偏离设计语言直译处）：
 *  - 规格 Don't 明言「涨跌色不得挪作通用 success/error」，但系统内无第二组
 *    绿/红，且任务态成功/失败与价格方向同构（主线定调「涨绿跌红贯穿」）——
 *    success/error 直译涨跌对，不发明第二组语义色，守住「单一彩 + 涨跌对」
 *    的调色板纪律；
 *  - warning 规格缺位 → #f19d27（推导：从品牌黄 #fcd535 橙移约 17°、同亮度，
 *    避免 warning 与 CTA 黄混淆；暗底 ≥4:1）；
 *  - mr/firstRun/usage 走黄（黄 = 头部指标与价值声明色，规格 stat-callout 角色），
 *    耗时 avgDuration 走跌红（耗时即成本信号）。
 * 时间线/事件一律字面色值（非 preset），与 CSS 语义令牌严格同源。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const binanceSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#fcd535',       // 每日创建 MR = Binance Yellow（头部指标独占品牌电压）
      crDone: '#0ecb81',   // 每日完成 CR = 涨绿（完成即正向）
      crStart: '#3b82f6',  // 每日发起 CR = info 蓝
      avgDuration: '#f6465d', // 每日成功运行平均耗时 = 跌红（耗时即成本信号）
    },
    ratios: {
      success: '#0ecb81',     // 当前任务成功占比 = 涨绿
      firstRun: '#fcd535',    // 首次执行成功率 = 黄（价值声明色）
      intervention: '#f6465d', // 触发人工介入率 = 跌红
      recovery: '#3b82f6',    // 受阻后恢复成功率 = info 蓝（避开已用的绿/黄/红）
    },
  },
  timeline: {
    error: '#f6465d',   // failed / blocked / unavailable（跌红）
    warning: '#f19d27', // human_required / retry_pending / waiting_upstream（推导琥珀）
    success: '#0ecb81', // succeeded / approved（涨绿）
    info: '#3b82f6',    // 其余默认 = info 蓝
  },
  eventType: {
    error: '#f6465d',  // codex_error / claude_error（跌红）
    usage: '#fcd535',  // codex_usage（用量 = 黄色价值指标）
    info: '#3b82f6',   // 其余默认
  },
  sparkline: {
    default: '#fcd535', // Sparkline 默认描线 = Binance Yellow（黑底上的黄电压）
    hoverGrid: '#454c57', // hover 竖向网格虚线 = 强线阶（推导：hairline 提亮一档保可见）
  },
}
