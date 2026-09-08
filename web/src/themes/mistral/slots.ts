import type { ThemeSlots } from '../types'

/**
 * mistral 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格（Semantic）：Mistral 全站只给了 link（= 主橙 #FA520F），
 * 语义态无字面色值；又规定「不引入日落调色板之外的强调色」。
 * 迁移策略：语义色按功能豁免（状态 ≠ 装饰），但取文字级、暖调偏置的克制值：
 *   绿 #1E7B45（暖调绿，奶油底上可读）/ 琥珀 #A16207（芥末Mustard 同族）/
 *   红 #C53030（落日深红，与主橙 #FA520F 保持可分辨的色相差）
 * 默认节点（timeline.info / eventType.info）回落编辑墨 #1F1F1F —— badge-dark
 * 语汇，比把 info 也染橙更符合「橙只属于 CTA/激活态」的纪律。
 * 趋势线走「主橙 + 墨阶 + Orange Deep」四级（Ink/Steel），呼应
 * sunset-plus-editorial-ink 的极端调色纪律。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const mistralSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#fa520f',      // 每日创建 MR = Mistral Orange（头部指标独占 CTA 橙）
      crDone: '#1f1f1f',  // 每日完成 CR = Ink
      crStart: '#6a6a6a', // 每日发起 CR = Steel
      avgDuration: '#cc3a05', // 每日成功运行平均耗时 = Orange Deep（日落三色收尾）
    },
    ratios: {
      success: '#1e7b45',  // 当前任务成功占比 = 派生暖调绿
      firstRun: '#fa520f', // 首次执行成功率 = 主橙
      intervention: '#c53030', // 触发人工介入率 = 派生落日深红
      recovery: '#a16207', // 受阻后恢复成功率 = 派生芥末琥珀
    },
  },
  timeline: {
    error: '#c53030',   // failed / blocked / unavailable
    warning: '#a16207', // human_required / retry_pending / waiting_upstream
    success: '#1e7b45', // succeeded / approved
    info: '#1f1f1f',    // 其余默认 = 编辑墨（badge-dark 语汇）
  },
  eventType: {
    error: '#c53030', // codex_error / claude_error
    usage: '#1e7b45', // codex_usage
    info: '#1f1f1f',  // 其余默认
  },
  sparkline: {
    default: '#fa520f',  // Sparkline 默认描线 = 主橙
    hoverGrid: '#e6d5a8', // hover 竖向网格虚线 = Beige Deep 暖发丝线
  },
}
