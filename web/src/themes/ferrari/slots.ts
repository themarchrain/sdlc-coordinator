import type { ThemeSlots } from '../types'

/**
 * ferrari 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取色纪律 = 规格「Rosso 稀缺原则」：数据视图里红只给头号指标与状态位
 * （MR 趋势 / 人工介入 / 事件错误），其余曲线走白→灰单色阶 + semantic-info
 * 蓝 #4c98b9（技术耗时语义）。语义三色中 success #03904a 直译规格；
 * warning 规格值 #f13a2c 为红且已让位 error，推导琥珀 #e8a13c（暖调、
 * #181818 画布上 ≈8:1，与 #303030 卡面 ≥3:1 描线对比）。
 * 近黑画布 (#181818/#303030) 上的可分辨性已逐色核对。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const ferrariSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#da291c',      // 每日创建 MR：Rosso 独占（race-position highlight 语义）
      crDone: '#ffffff',  // 每日完成 CR：白（number-display 的数据白）
      crStart: '#969696', // 每日发起 CR：body 灰（单色阶）
      avgDuration: '#4c98b9', // 成功运行平均耗时：semantic-info 蓝（技术量）
    },
    ratios: {
      success: '#03904a',     // 当前任务成功占比：语义绿（规格直译）
      firstRun: '#4c98b9',    // 首次执行成功率：info 蓝
      intervention: '#f13a2c', // 人工介入率：validation red（警示位）
      recovery: '#e8a13c',    // 受阻后恢复成功率：推导琥珀（规格无独立 warning 值）
    },
  },
  timeline: {
    error: '#f13a2c',   // failed / blocked / unavailable（规格 validation red）
    warning: '#e8a13c', // human_required / retry_pending / waiting_upstream（推导琥珀）
    success: '#03904a', // succeeded / approved（规格直译）
    info: '#4c98b9',    // 其余默认 = semantic-info 蓝
  },
  eventType: {
    error: '#f13a2c',  // codex_error / claude_error
    usage: '#03904a',  // codex_usage
    info: '#4c98b9',   // 其余默认
  },
  sparkline: {
    default: '#da291c', // Sparkline 默认描线 = Rosso（F1 遥测红迹的界面回声）
    hoverGrid: '#4a4a4a', // hover 竖向网格虚线 = 强线灰（暗底可见）
  },
}
