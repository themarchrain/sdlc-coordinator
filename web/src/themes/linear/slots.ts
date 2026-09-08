import type { ThemeSlots } from '../types'

/**
 * linear 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取色池 = 规格色板 + 两条推导线：
 *  1. 单彩纪律（规格 Don't「no second chromatic accent」）：趋势线走
 *     「淡薰衣草 + 浅灰墨阶」——最亮家族成员 #828fff 领衔 MR 主线，
 *     ink #f7f8f8 / brand-secure #7a7fad / ink-tertiary #62666d 阶梯承接
 *     （tesla「monochrome-plus-one」的暗色倒影）；
 *  2. 语义色仅用于功能位（时间线/占比/事件）：success #27a644 为规格直给，
 *     error #eb5757 / warning #f2994a / info #4ea7fc 取规格 Known Gaps 声明的
 *     Linear 产品色标签（管理台是产品面，营销面无语义彩）。
 * 近黑画布 (#010102/#0f1011) 上的可分辨性已逐色核对：全部 ≥3:1 描线对比
 * （#62666d 3.3:1 为第 4 辅助线，与 tesla Silver Fog 同档）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const linearSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#828fff',        // 每日创建 MR：提亮淡薰衣草（家族最亮，头号线，7.2:1）
      crDone: '#f7f8f8',    // 每日完成 CR：ink 浅灰墨（单彩纪律的墨阶第一级）
      crStart: '#7a7fad',   // 每日发起 CR：brand-secure 薰衣草灰（墨阶中的品牌呼应）
      avgDuration: '#62666d', // 每日成功运行平均耗时：ink-tertiary 深灰（第 4 辅助线）
    },
    ratios: {
      success: '#27a644',     // 当前任务成功占比：规格 success
      firstRun: '#828fff',    // 首次执行成功率：淡薰衣草（品牌质量线）
      intervention: '#eb5757', // 触发人工介入率：产品红
      recovery: '#f2994a',    // 受阻后恢复成功率：产品橙
    },
  },
  timeline: {
    error: '#eb5757',   // failed / blocked / unavailable（产品红，推导）
    warning: '#f2994a', // human_required / retry_pending / waiting_upstream（产品橙，推导）
    success: '#27a644', // succeeded / approved（规格 success）
    info: '#5e6ad2',    // 其余默认 = 薰衣草（默认节点走品牌强调色而非 info 蓝）
  },
  eventType: {
    error: '#eb5757',  // codex_error / claude_error
    usage: '#27a644',  // codex_usage（规格 success：用量事件为正向信号）
    info: '#5e6ad2',   // 其余默认 = 薰衣草
  },
  sparkline: {
    default: '#828fff', // Sparkline 默认描线：提亮淡薰衣草（近黑画布上的唯一彩色事件）
    hoverGrid: '#34343a', // hover 竖向网格虚线：hairline-strong（暗底可见）
  },
}
